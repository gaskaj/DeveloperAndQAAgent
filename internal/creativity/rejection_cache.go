package creativity

import (
	"strings"
	"sync"
)

// RejectionCache stores rejected suggestion titles for deduplication.
// It is thread-safe and uses FIFO eviction when at capacity.
type RejectionCache struct {
	mu      sync.RWMutex
	titles  []string
	maxSize int
}

// NewRejectionCache creates a new RejectionCache with the given maximum size.
func NewRejectionCache(maxSize int) *RejectionCache {
	return &RejectionCache{
		titles:  make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a rejected title to the cache. Duplicate titles are ignored.
// If the cache is at capacity, the oldest title is evicted (FIFO).
func (c *RejectionCache) Add(title string) {
	normalized := strings.ToLower(strings.TrimSpace(title))
	if normalized == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Skip if already present.
	for _, t := range c.titles {
		if t == normalized {
			return
		}
	}

	// Evict oldest if at capacity.
	if len(c.titles) >= c.maxSize {
		c.titles = c.titles[1:]
	}

	c.titles = append(c.titles, normalized)
}

// Contains checks if a title matches any rejected title using case-insensitive
// substring matching. Returns true if the given title is a substring of any
// cached title, or any cached title is a substring of the given title.
func (c *RejectionCache) Contains(title string) bool {
	normalized := strings.ToLower(strings.TrimSpace(title))
	if normalized == "" {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, t := range c.titles {
		if strings.Contains(normalized, t) || strings.Contains(t, normalized) {
			return true
		}
	}
	return false
}
