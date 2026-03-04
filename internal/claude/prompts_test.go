package claude

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSystemPrompt_SinglePart(t *testing.T) {
	result := FormatSystemPrompt("You are a helpful assistant.")
	assert.Equal(t, "You are a helpful assistant.", result)
}

func TestFormatSystemPrompt_MultipleParts(t *testing.T) {
	result := FormatSystemPrompt("Part one", "Part two", "Part three")
	assert.Equal(t, "Part one\n\nPart two\n\nPart three", result)
}

func TestFormatSystemPrompt_Empty(t *testing.T) {
	result := FormatSystemPrompt()
	assert.Equal(t, "", result)
}

func TestFormatSystemPrompt_SingleEmpty(t *testing.T) {
	result := FormatSystemPrompt("")
	assert.Equal(t, "", result)
}

func TestFormatIssueContext(t *testing.T) {
	result := FormatIssueContext(42, "Fix bug", "The bug is in main.go", []string{"bug", "agent:ready"})
	assert.Contains(t, result, "#42")
	assert.Contains(t, result, "Fix bug")
	assert.Contains(t, result, "The bug is in main.go")
	assert.Contains(t, result, "bug, agent:ready")
}

func TestFormatIssueContext_NoLabels(t *testing.T) {
	result := FormatIssueContext(1, "Title", "Body", nil)
	assert.Contains(t, result, "#1")
	assert.Contains(t, result, "Title")
	assert.Contains(t, result, "Body")
}

func TestFormatIssueContext_EmptyBody(t *testing.T) {
	result := FormatIssueContext(10, "No body", "", []string{"enhancement"})
	assert.Contains(t, result, "#10")
	assert.Contains(t, result, "No body")
	assert.Contains(t, result, "enhancement")
}

func TestFormatFileList_WithFiles(t *testing.T) {
	files := []string{"main.go", "utils.go", "README.md"}
	result := FormatFileList(files)
	assert.Contains(t, result, "- main.go")
	assert.Contains(t, result, "- utils.go")
	assert.Contains(t, result, "- README.md")
	// Each line ends with newline
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	assert.Len(t, lines, 3)
}

func TestFormatFileList_Empty(t *testing.T) {
	result := FormatFileList(nil)
	assert.Equal(t, "(no files)", result)
}

func TestFormatFileList_EmptySlice(t *testing.T) {
	result := FormatFileList([]string{})
	assert.Equal(t, "(no files)", result)
}

func TestFormatFileList_SingleFile(t *testing.T) {
	result := FormatFileList([]string{"only.go"})
	assert.Equal(t, "- only.go\n", result)
}
