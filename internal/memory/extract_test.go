package memory

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExtractedMemories(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `Here are the learnings:

` + "```json" + `
[
  {"category": "architecture", "content": "Uses dependency injection via agent.Dependencies struct"},
  {"category": "convention", "content": "Error wrapping uses fmt.Errorf with %w verb"},
  {"category": "gotcha", "content": "Must rebuild binary after code changes"}
]
` + "```"

	entries := ParseExtractedMemories(response, "issue-123", logger)
	assert.Len(t, entries, 3)
	assert.Equal(t, CategoryArchitecture, entries[0].Category)
	assert.Equal(t, CategoryConvention, entries[1].Category)
	assert.Equal(t, CategoryGotcha, entries[2].Category)
}

func TestParseExtractedMemoriesInvalid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	entries := ParseExtractedMemories("no json here", "issue-456", logger)
	assert.Nil(t, entries)
}

func TestParseExtractedMemoriesInvalidCategory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `[{"category": "invalid_cat", "content": "should be skipped"}]`
	entries := ParseExtractedMemories(response, "issue-789", logger)
	assert.Len(t, entries, 0)
}

func TestParseExtractedMemoriesCapsAt5(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `[
		{"category": "learning", "content": "a"},
		{"category": "learning", "content": "b"},
		{"category": "learning", "content": "c"},
		{"category": "learning", "content": "d"},
		{"category": "learning", "content": "e"},
		{"category": "learning", "content": "f"},
		{"category": "learning", "content": "g"}
	]`
	entries := ParseExtractedMemories(response, "issue-100", logger)
	assert.Len(t, entries, 5)
}
