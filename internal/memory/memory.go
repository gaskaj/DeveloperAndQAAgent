package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// maxMemories is the maximum number of memories stored per repository.
const maxMemories = 100

// maxPromptSize caps the total character size of memory injected into prompts.
const maxPromptSize = 8000

// Category classifies what a memory entry is about.
type Category string

const (
	CategoryArchitecture Category = "architecture"
	CategoryConvention   Category = "convention"
	CategoryPattern      Category = "pattern"
	CategoryFileMap      Category = "file_map"
	CategoryLearning     Category = "learning"
	CategoryGotcha       Category = "gotcha"
)

// Entry represents a single memory learned from working on a repository.
type Entry struct {
	ID        string   `json:"id"`
	Category  Category `json:"category"`
	Content   string   `json:"content"`
	Source    string   `json:"source,omitempty"`    // e.g. "issue-123", "creativity"
	CreatedAt time.Time `json:"created_at"`
	UsedCount int      `json:"used_count"`
	LastUsed  time.Time `json:"last_used"`
}

// Store persists and retrieves memory entries for a specific repository.
type Store struct {
	dir     string
	mu      sync.RWMutex
	entries []*Entry
}

// NewStore creates a memory store at the given directory path.
// The directory is typically workspaces/{owner}/{repo}/.memory/
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating memory dir: %w", err)
	}
	s := &Store{dir: dir}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("loading memories: %w", err)
	}
	return s, nil
}

func (s *Store) memoryFile() string {
	return filepath.Join(s.dir, "memories.json")
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.memoryFile())
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = make([]*Entry, 0)
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.memoryFile(), data, 0o644)
}

// Add stores a new memory entry, deduplicating by content similarity.
func (s *Store) Add(entry *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate content
	for _, existing := range s.entries {
		if existing.Content == entry.Content {
			return nil // exact duplicate, skip
		}
	}

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s-%d", entry.Category, time.Now().UnixNano())
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	s.entries = append(s.entries, entry)

	// Evict oldest, least-used entries if over limit
	if len(s.entries) > maxMemories {
		s.evict()
	}

	return s.save()
}

// evict removes the oldest, least-used entries to stay under maxMemories.
func (s *Store) evict() {
	// Find the entry with the lowest score (used_count / age)
	if len(s.entries) <= maxMemories {
		return
	}

	// Simple eviction: remove entries with lowest use count, oldest first
	minIdx := 0
	minScore := s.entries[0].UsedCount
	for i, e := range s.entries {
		if e.UsedCount < minScore {
			minScore = e.UsedCount
			minIdx = i
		}
	}
	s.entries = append(s.entries[:minIdx], s.entries[minIdx+1:]...)
}

// All returns all stored memories.
func (s *Store) All() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Entry, len(s.entries))
	copy(result, s.entries)
	return result
}

// ByCategory returns memories filtered by category.
func (s *Store) ByCategory(cat Category) []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Entry
	for _, e := range s.entries {
		if e.Category == cat {
			result = append(result, e)
		}
	}
	return result
}

// MarkUsed increments the use count for the given entries.
func (s *Store) MarkUsed(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	for _, e := range s.entries {
		if idSet[e.ID] {
			e.UsedCount++
			e.LastUsed = time.Now()
		}
	}
	_ = s.save()
}

// Count returns the number of stored memories.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// FormatForPrompt builds a prompt section from all stored memories,
// capped at maxPromptSize characters.
func (s *Store) FormatForPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Repository Memory\n\n")
	sb.WriteString("These are learnings from previous work on this repository. Use them to work more efficiently.\n\n")

	// Group by category for readability
	categoryOrder := []Category{
		CategoryArchitecture,
		CategoryConvention,
		CategoryPattern,
		CategoryFileMap,
		CategoryGotcha,
		CategoryLearning,
	}

	categoryNames := map[Category]string{
		CategoryArchitecture: "Architecture",
		CategoryConvention:   "Conventions",
		CategoryPattern:      "Patterns",
		CategoryFileMap:      "Key Files",
		CategoryGotcha:       "Gotchas",
		CategoryLearning:     "Learnings",
	}

	totalSize := sb.Len()
	var usedIDs []string

	for _, cat := range categoryOrder {
		var catEntries []*Entry
		for _, e := range s.entries {
			if e.Category == cat {
				catEntries = append(catEntries, e)
			}
		}
		if len(catEntries) == 0 {
			continue
		}

		header := fmt.Sprintf("### %s\n", categoryNames[cat])
		if totalSize+len(header) > maxPromptSize {
			break
		}
		sb.WriteString(header)
		totalSize += len(header)

		for _, e := range catEntries {
			line := fmt.Sprintf("- %s\n", e.Content)
			if totalSize+len(line) > maxPromptSize {
				break
			}
			sb.WriteString(line)
			totalSize += len(line)
			usedIDs = append(usedIDs, e.ID)
		}
		sb.WriteString("\n")
		totalSize++
	}

	// Mark entries as used (without lock since we're already holding it read-only,
	// we'll do this after releasing)
	go func() {
		s.MarkUsed(usedIDs)
	}()

	return sb.String()
}
