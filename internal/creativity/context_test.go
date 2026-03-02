package creativity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildPrompt_AllSections(t *testing.T) {
	ctx := &ProjectContext{
		OpenIssues: []*Issue{
			{Number: 1, Title: "Add feature X", Body: "Detailed description of feature X"},
			{Number: 2, Title: "Fix bug Y", Body: "Bug Y causes crashes"},
		},
		ClosedIssues: []*Issue{
			{Number: 10, Title: "Implemented auth", State: "closed"},
			{Number: 11, Title: "Fixed login bug", State: "closed"},
		},
		PendingIdeas: []*Issue{
			{Number: 20, Title: "Add caching"},
		},
		RejectedIdeas: []string{"Add XML support"},
		RepoStructure: "cmd/\ninternal/\n  agent/\n  developer/\n",
		KeyDocs: map[string]string{
			"README.md": "# Project\nThis is a project.",
			"CLAUDE.md": "# Instructions\nBuild conventions.",
		},
	}

	prompt := buildPrompt(ctx)

	// Verify all sections are present.
	assert.Contains(t, prompt, "Repository Structure")
	assert.Contains(t, prompt, "cmd/")
	assert.Contains(t, prompt, "internal/")

	assert.Contains(t, prompt, "Key Documentation")
	assert.Contains(t, prompt, "README.md")
	assert.Contains(t, prompt, "This is a project.")
	assert.Contains(t, prompt, "CLAUDE.md")
	assert.Contains(t, prompt, "Build conventions.")

	assert.Contains(t, prompt, "Open Issues")
	assert.Contains(t, prompt, "#1: Add feature X")
	assert.Contains(t, prompt, "#2: Fix bug Y")
	assert.Contains(t, prompt, "Detailed description of feature X")

	assert.Contains(t, prompt, "Closed Issues")
	assert.Contains(t, prompt, "#10: Implemented auth")
	assert.Contains(t, prompt, "#11: Fixed login bug")

	assert.Contains(t, prompt, "Pending Suggestions")
	assert.Contains(t, prompt, "#20: Add caching")

	assert.Contains(t, prompt, "Rejected Ideas")
	assert.Contains(t, prompt, "Add XML support")

	assert.Contains(t, prompt, "Instructions")
	assert.Contains(t, prompt, "TITLE:")
	assert.Contains(t, prompt, "BODY:")
}

func TestBuildPrompt_EmptySections(t *testing.T) {
	ctx := &ProjectContext{}

	prompt := buildPrompt(ctx)

	// Should still have the review process and instructions.
	assert.Contains(t, prompt, "Your Review Process")
	assert.Contains(t, prompt, "## Instructions")
	assert.Contains(t, prompt, "TITLE:")

	// Empty data sections should not appear as headers.
	assert.NotContains(t, prompt, "## Repository Structure")
	assert.NotContains(t, prompt, "## Key Documentation")
	assert.NotContains(t, prompt, "## Open Issues")
	assert.NotContains(t, prompt, "## Closed Issues")
	assert.NotContains(t, prompt, "## Pending Suggestions")
	assert.NotContains(t, prompt, "## Previously Rejected Ideas")
}

func TestBuildPrompt_NoDocsOrRepo(t *testing.T) {
	ctx := &ProjectContext{
		OpenIssues: []*Issue{
			{Number: 1, Title: "Some issue"},
		},
		ClosedIssues: []*Issue{
			{Number: 5, Title: "Done task"},
		},
	}

	prompt := buildPrompt(ctx)

	assert.NotContains(t, prompt, "Repository Structure")
	assert.NotContains(t, prompt, "Key Documentation")
	assert.Contains(t, prompt, "Open Issues")
	assert.Contains(t, prompt, "#1: Some issue")
	assert.Contains(t, prompt, "Closed Issues")
	assert.Contains(t, prompt, "#5: Done task")
}

func TestBuildPrompt_IssueBodyTruncation(t *testing.T) {
	longBody := "This is a very long issue body that goes on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on and on"

	ctx := &ProjectContext{
		OpenIssues: []*Issue{
			{Number: 1, Title: "Issue with long body", Body: longBody},
		},
	}

	prompt := buildPrompt(ctx)
	assert.Contains(t, prompt, "#1: Issue with long body")
	assert.Contains(t, prompt, "...")
}

func TestTruncateBody(t *testing.T) {
	assert.Equal(t, "", truncateBody("", 200))
	assert.Equal(t, "", truncateBody("   ", 200))
	assert.Equal(t, "short", truncateBody("short", 200))
	assert.Equal(t, "first line", truncateBody("first line\nsecond line", 200))

	long := "abcdefghij"
	assert.Equal(t, "abcde...", truncateBody(long, 5))
}

func TestTruncateDoc(t *testing.T) {
	short := "short content"
	assert.Equal(t, short, truncateDoc(short))

	long := make([]byte, maxDocSize+100)
	for i := range long {
		long[i] = 'x'
	}
	result := truncateDoc(string(long))
	assert.Equal(t, maxDocSize+len("\n... (truncated)"), len(result))
	assert.Contains(t, result, "... (truncated)")
}
