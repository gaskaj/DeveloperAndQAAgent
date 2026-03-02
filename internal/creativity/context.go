package creativity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/gitops"
)

// maxClosedIssues is the cap on closed issues fetched for context.
const maxClosedIssues = 50

// maxDocSize is the max characters per documentation file included in the prompt.
const maxDocSize = 3000

// ProjectContext holds contextual information about the project for prompt building.
type ProjectContext struct {
	OpenIssues    []*Issue
	ClosedIssues  []*Issue
	RejectedIdeas []string
	PendingIdeas  []*Issue
	RepoStructure string
	KeyDocs       map[string]string // path → content
}

// gatherContext fetches project context from GitHub issues and the repository.
func (e *CreativityEngine) gatherContext(ctx context.Context) (*ProjectContext, error) {
	openIssues, err := e.gh.ListIssuesByLabel(ctx, labelReady)
	if err != nil {
		return nil, fmt.Errorf("gathering open issues: %w", err)
	}

	pendingIdeas, err := e.gh.ListIssuesByLabel(ctx, labelSuggestion)
	if err != nil {
		return nil, fmt.Errorf("gathering pending suggestions: %w", err)
	}

	// Fetch closed issues for awareness of completed work.
	closedIssues, err := e.gh.ListAllClosedIssues(ctx)
	if err != nil {
		e.logger.Warn("failed to fetch closed issues", "error", err)
		closedIssues = nil
	}
	if len(closedIssues) > maxClosedIssues {
		closedIssues = closedIssues[:maxClosedIssues]
	}

	// Gather repo context (file tree + key docs).
	var repoStructure string
	var keyDocs map[string]string

	if e.repoCfg.URL != "" {
		repo, err := e.ensureRepo(ctx)
		if err != nil {
			e.logger.Warn("failed to clone repo for context, continuing without codebase awareness", "error", err)
		} else {
			repoStructure = buildRepoTree(repo)
			keyDocs = readKeyDocs(repo)
		}
	}

	return &ProjectContext{
		OpenIssues:    openIssues,
		ClosedIssues:  closedIssues,
		RejectedIdeas: e.rejectionCache.titles,
		PendingIdeas:  pendingIdeas,
		RepoStructure: repoStructure,
		KeyDocs:       keyDocs,
	}, nil
}

// buildRepoTree builds a file tree string from the repository.
func buildRepoTree(repo *gitops.Repo) string {
	var sb strings.Builder
	repoDir := repo.Dir()

	_ = filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoDir, path)
		if rel == "." {
			return nil
		}
		// Skip hidden dirs (.git), workspaces, vendor, node_modules.
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "workspaces" || d.Name() == "vendor" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		indent := strings.Repeat("  ", strings.Count(rel, string(filepath.Separator)))
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		sb.WriteString(indent + name + "\n")
		return nil
	})

	return sb.String()
}

// readKeyDocs reads README.md, CLAUDE.md, and all .md files under docs/.
func readKeyDocs(repo *gitops.Repo) map[string]string {
	docs := make(map[string]string)

	// Read top-level docs.
	for _, name := range []string{"README.md", "CLAUDE.md"} {
		if content, err := repo.ReadFile(name); err == nil {
			docs[name] = truncateDoc(content)
		}
	}

	// Read docs/ directory.
	docsDir := filepath.Join(repo.Dir(), "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return docs
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join("docs", entry.Name())
		if content, err := repo.ReadFile(path); err == nil {
			docs[path] = truncateDoc(content)
		}
	}

	return docs
}

// truncateDoc truncates a document to maxDocSize characters.
func truncateDoc(content string) string {
	if len(content) > maxDocSize {
		return content[:maxDocSize] + "\n... (truncated)"
	}
	return content
}

// buildPrompt constructs the AI prompt with full project context.
func buildPrompt(projectCtx *ProjectContext) string {
	var b strings.Builder

	b.WriteString("You are a senior software engineer performing a thorough review of a project to suggest ONE high-impact improvement.\n\n")
	b.WriteString("## Your Review Process\n\n")
	b.WriteString("Before proposing a suggestion, you MUST review all of the following:\n\n")
	b.WriteString("1. **Codebase Structure** — Understand the project layout, packages, and file organization\n")
	b.WriteString("2. **Documentation** — Read the README, CLAUDE.md, and all files in ./docs/ to understand architecture, conventions, and existing documentation\n")
	b.WriteString("3. **Open Issues** — Understand what work is currently planned or in progress\n")
	b.WriteString("4. **Closed Issues** — Understand what has already been implemented, fixed, or completed\n")
	b.WriteString("5. **Pending Suggestions** — See what improvements have already been proposed\n")
	b.WriteString("6. **Rejected Ideas** — Avoid suggesting anything that was previously rejected\n\n")

	// Repository structure.
	if projectCtx.RepoStructure != "" {
		b.WriteString("## Repository Structure\n\n```\n")
		b.WriteString(projectCtx.RepoStructure)
		b.WriteString("```\n\n")
	}

	// Key documentation.
	if len(projectCtx.KeyDocs) > 0 {
		b.WriteString("## Key Documentation\n\n")
		for path, content := range projectCtx.KeyDocs {
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", path, content)
		}
	}

	// Open issues with bodies.
	if len(projectCtx.OpenIssues) > 0 {
		b.WriteString("## Open Issues (planned/in-progress work)\n\n")
		for _, issue := range projectCtx.OpenIssues {
			fmt.Fprintf(&b, "- #%d: %s", issue.Number, issue.Title)
			if body := truncateBody(issue.Body, 200); body != "" {
				fmt.Fprintf(&b, "\n  > %s", body)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Closed issues.
	if len(projectCtx.ClosedIssues) > 0 {
		b.WriteString("## Closed Issues (completed work)\n\n")
		for _, issue := range projectCtx.ClosedIssues {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.Number, issue.Title)
		}
		b.WriteString("\n")
	}

	// Pending suggestions.
	if len(projectCtx.PendingIdeas) > 0 {
		b.WriteString("## Pending Suggestions (already proposed)\n\n")
		for _, issue := range projectCtx.PendingIdeas {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.Number, issue.Title)
		}
		b.WriteString("\n")
	}

	// Rejected ideas.
	if len(projectCtx.RejectedIdeas) > 0 {
		b.WriteString("## Previously Rejected Ideas (do NOT suggest these again)\n\n")
		for _, title := range projectCtx.RejectedIdeas {
			fmt.Fprintf(&b, "- %s\n", title)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Instructions\n\n")
	b.WriteString("Based on your thorough review of the codebase, documentation, open issues, and closed issues:\n\n")
	b.WriteString("1. Identify a gap, weakness, or opportunity that is NOT already covered by open issues, closed issues, pending suggestions, or rejected ideas\n")
	b.WriteString("2. The suggestion must be concrete and actionable — reference specific files, packages, or patterns\n")
	b.WriteString("3. Focus on: code quality, performance, security, testing, documentation, or developer experience\n")
	b.WriteString("4. Ensure the suggestion includes documentation updates in the ./docs directory\n")
	b.WriteString("5. Do NOT duplicate any existing open issue, closed issue, pending suggestion, or rejected idea\n\n")
	b.WriteString("Respond with:\nTITLE: <concise issue title under 80 characters>\nBODY:\n<detailed markdown description including specific files and areas to address>\n")

	return b.String()
}

// truncateBody truncates an issue body to the given max length for prompt inclusion.
func truncateBody(body string, maxLen int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	// Take only the first line or maxLen chars, whichever is shorter.
	if idx := strings.IndexByte(body, '\n'); idx != -1 && idx < maxLen {
		body = body[:idx]
	}
	if len(body) > maxLen {
		body = body[:maxLen] + "..."
	}
	return body
}
