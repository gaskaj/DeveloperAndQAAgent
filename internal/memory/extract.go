package memory

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// ExtractionPrompt is appended to the implement response to extract learnings.
const ExtractionPrompt = `

Now that implementation is complete, extract key learnings about this repository that would help future work.

Respond with a JSON array of memory entries. Each entry should capture ONE specific, reusable insight.

Categories:
- "architecture": High-level design decisions (e.g., "Uses dependency injection via agent.Dependencies struct")
- "convention": Coding conventions (e.g., "Error wrapping uses fmt.Errorf with %%w verb")
- "pattern": Recurring patterns (e.g., "All handlers follow chi middleware chain pattern")
- "file_map": Important file locations (e.g., "Config types are in internal/config/config.go")
- "gotcha": Non-obvious pitfalls (e.g., "Must rebuild binary after changing internal/ code")
- "learning": Task-specific insights (e.g., "GitHub API returns 422 when label already exists")

Rules:
- Only include learnings that would be useful across MULTIPLE future issues
- Be specific — reference actual file paths, function names, and package names
- Skip obvious things (like "Go uses goroutines")
- Maximum 5 entries per extraction
- Keep each entry under 200 characters

Format:
` + "```json" + `
[
  {"category": "architecture", "content": "..."},
  {"category": "convention", "content": "..."}
]
` + "```"

// ParseExtractedMemories parses Claude's response for memory entries.
func ParseExtractedMemories(response string, source string, logger *slog.Logger) []*Entry {
	// Find JSON array in the response
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start == -1 || end == -1 || end <= start {
		logger.Debug("no memory entries found in response")
		return nil
	}

	jsonStr := response[start : end+1]

	var raw []struct {
		Category string `json:"category"`
		Content  string `json:"content"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		logger.Debug("failed to parse memory entries", "error", err)
		return nil
	}

	var entries []*Entry
	for _, r := range raw {
		cat := Category(r.Category)
		switch cat {
		case CategoryArchitecture, CategoryConvention, CategoryPattern,
			CategoryFileMap, CategoryGotcha, CategoryLearning:
			// valid
		default:
			continue
		}

		content := strings.TrimSpace(r.Content)
		if content == "" || len(content) > 300 {
			continue
		}

		entries = append(entries, &Entry{
			ID:       fmt.Sprintf("%s-%s", cat, sanitizeID(content)),
			Category: cat,
			Content:  content,
			Source:   source,
		})
	}

	// Cap at 5 entries
	if len(entries) > 5 {
		entries = entries[:5]
	}

	return entries
}

// sanitizeID creates a short ID suffix from content.
func sanitizeID(content string) string {
	s := strings.ToLower(content)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.Trim(s, "-")
}
