package creativity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/gitops"
	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGatherContext(t *testing.T) {
	t.Run("gathers context without repo URL", func(t *testing.T) {
		gh := &gatherContextMockGH{
			readyIssues:    []*Issue{{Number: 1, Title: "Ready issue"}},
			suggIssues:     []*Issue{{Number: 2, Title: "Pending suggestion"}},
			closedIssues:   []*Issue{{Number: 10, Title: "Closed issue"}},
			rejectedIssues: nil,
		}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

		ctx, err := engine.gatherContext(context.Background())
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Len(t, ctx.OpenIssues, 1)
		assert.Equal(t, "Ready issue", ctx.OpenIssues[0].Title)
		assert.Len(t, ctx.PendingIdeas, 1)
		assert.Len(t, ctx.ClosedIssues, 1)
		assert.Empty(t, ctx.RepoStructure)
		assert.Empty(t, ctx.KeyDocs)
	})

	t.Run("truncates closed issues to max", func(t *testing.T) {
		closedIssues := make([]*Issue, 100)
		for i := range closedIssues {
			closedIssues[i] = &Issue{Number: i, Title: fmt.Sprintf("Closed %d", i)}
		}

		gh := &gatherContextMockGH{
			readyIssues:  nil,
			suggIssues:   nil,
			closedIssues: closedIssues,
		}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

		ctx, err := engine.gatherContext(context.Background())
		require.NoError(t, err)
		assert.Len(t, ctx.ClosedIssues, maxClosedIssues)
	})

	t.Run("handles closed issues fetch error gracefully", func(t *testing.T) {
		gh := &gatherContextMockGH{
			readyIssues:     nil,
			suggIssues:      nil,
			closedIssuesErr: fmt.Errorf("API error"),
		}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

		ctx, err := engine.gatherContext(context.Background())
		require.NoError(t, err) // Should not fail
		assert.Nil(t, ctx.ClosedIssues)
	})
}

// gatherContextMockGH is a mock that returns specific issues based on label.
type gatherContextMockGH struct {
	readyIssues     []*Issue
	suggIssues      []*Issue
	rejectedIssues  []*Issue
	closedIssues    []*Issue
	closedIssuesErr error
}

func (m *gatherContextMockGH) ListIssuesByLabel(_ context.Context, label string) ([]*Issue, error) {
	switch label {
	case labelReady:
		return m.readyIssues, nil
	case labelSuggestion:
		return m.suggIssues, nil
	case labelSuggestionRejected:
		return m.rejectedIssues, nil
	}
	return nil, nil
}
func (m *gatherContextMockGH) ListClosedIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return m.closedIssues, m.closedIssuesErr
}
func (m *gatherContextMockGH) ListAllClosedIssues(_ context.Context) ([]*Issue, error) {
	return m.closedIssues, m.closedIssuesErr
}
func (m *gatherContextMockGH) CreateIssue(_ context.Context, _, _ string, _ []string) (int, error) {
	return 0, nil
}
func (m *gatherContextMockGH) AddLabels(_ context.Context, _ int, _ []string) error { return nil }
func (m *gatherContextMockGH) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }

// initTestRepo creates a temporary git repository with some files for testing.
func initTestRepo(t *testing.T) *gitops.Repo {
	t.Helper()
	dir := t.TempDir()

	// Initialize a bare git repo
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	// Create directory structure
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", "main"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "agent"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0755)) // already exists
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules"), 0755))

	// Create files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\nThis is a test."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Instructions\nBuild conventions."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd", "main", "main.go"), []byte("package main"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "agent", "agent.go"), []byte("package agent"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "architecture.md"), []byte("# Architecture\nDesign doc."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "config.md"), []byte("# Config\nConfig doc."), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "notes.txt"), []byte("Not a markdown file"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "dep.go"), []byte("package dep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "pkg.js"), []byte("module.exports = {}"), 0644))

	repo, err := gitops.Open(dir, "")
	require.NoError(t, err)
	return repo
}

func TestBuildRepoTree(t *testing.T) {
	repo := initTestRepo(t)
	tree := buildRepoTree(repo)

	// Should contain top-level dirs
	assert.Contains(t, tree, "cmd/")
	assert.Contains(t, tree, "internal/")
	assert.Contains(t, tree, "docs/")

	// Should contain files
	assert.Contains(t, tree, "README.md")
	assert.Contains(t, tree, "CLAUDE.md")
	assert.Contains(t, tree, "main.go")
	assert.Contains(t, tree, "agent.go")

	// Should NOT contain hidden dirs, vendor, node_modules
	assert.NotContains(t, tree, "vendor/")
	assert.NotContains(t, tree, "node_modules/")
	assert.NotContains(t, tree, "hooks")
	assert.NotContains(t, tree, "dep.go")
	assert.NotContains(t, tree, "pkg.js")
}

func TestReadKeyDocs(t *testing.T) {
	repo := initTestRepo(t)
	docs := readKeyDocs(repo)

	// Should read top-level docs
	assert.Contains(t, docs, "README.md")
	assert.Contains(t, docs["README.md"], "Test Project")

	assert.Contains(t, docs, "CLAUDE.md")
	assert.Contains(t, docs["CLAUDE.md"], "Instructions")

	// Should read markdown files from docs/
	assert.Contains(t, docs, filepath.Join("docs", "architecture.md"))
	assert.Contains(t, docs[filepath.Join("docs", "architecture.md")], "Architecture")

	assert.Contains(t, docs, filepath.Join("docs", "config.md"))
	assert.Contains(t, docs[filepath.Join("docs", "config.md")], "Config")

	// Should NOT read non-markdown files from docs/
	_, hasNotes := docs[filepath.Join("docs", "notes.txt")]
	assert.False(t, hasNotes)
}

func TestReadKeyDocs_NoDocsDir(t *testing.T) {
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	// Only create README, no docs/ directory
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Readme"), 0644))

	repo, err := gitops.Open(dir, "")
	require.NoError(t, err)

	docs := readKeyDocs(repo)
	assert.Contains(t, docs, "README.md")
	// Should not have any docs/ entries
	for k := range docs {
		assert.NotContains(t, k, "docs/")
	}
}
