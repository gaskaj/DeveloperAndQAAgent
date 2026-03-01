package creativity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectionCache_Add(t *testing.T) {
	cache := NewRejectionCache(10)
	cache.Add("Add logging framework")

	assert.True(t, cache.Contains("add logging framework"))
}

func TestRejectionCache_Contains(t *testing.T) {
	cache := NewRejectionCache(10)
	cache.Add("Improve error handling")

	assert.True(t, cache.Contains("Improve error handling"))
	assert.True(t, cache.Contains("improve error handling"))
	assert.False(t, cache.Contains("add caching"))
}

func TestRejectionCache_ContainsSubstring(t *testing.T) {
	cache := NewRejectionCache(10)
	cache.Add("Add comprehensive logging framework")

	// Cached title contains the query.
	assert.True(t, cache.Contains("logging framework"))
	// Query contains the cached title.
	assert.True(t, cache.Contains("add comprehensive logging framework to the project"))
	// No match.
	assert.False(t, cache.Contains("caching layer"))
}

func TestRejectionCache_MaxSize(t *testing.T) {
	cache := NewRejectionCache(3)
	cache.Add("first")
	cache.Add("second")
	cache.Add("third")
	cache.Add("fourth") // Should evict "first".

	assert.False(t, cache.Contains("first"), "first should have been evicted")
	assert.True(t, cache.Contains("second"))
	assert.True(t, cache.Contains("third"))
	assert.True(t, cache.Contains("fourth"))
}

func TestRejectionCache_NoDuplicates(t *testing.T) {
	cache := NewRejectionCache(10)
	cache.Add("Same Title")
	cache.Add("Same Title")
	cache.Add("SAME TITLE")

	// Should only have one entry.
	cache.mu.RLock()
	assert.Equal(t, 1, len(cache.titles))
	cache.mu.RUnlock()
}

func TestRejectionCache_EmptyTitle(t *testing.T) {
	cache := NewRejectionCache(10)
	cache.Add("")
	cache.Add("  ")

	cache.mu.RLock()
	assert.Equal(t, 0, len(cache.titles))
	cache.mu.RUnlock()

	assert.False(t, cache.Contains(""))
}
