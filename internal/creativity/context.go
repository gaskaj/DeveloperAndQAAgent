package creativity

import (
	"context"
	"fmt"
	"strings"
)

// ProjectContext holds contextual information about the project for prompt building.
type ProjectContext struct {
	OpenIssues    []*Issue
	RejectedIdeas []string
	PendingIdeas  []*Issue
}

// gatherContext fetches project context from GitHub issues.
func (e *CreativityEngine) gatherContext(ctx context.Context) (*ProjectContext, error) {
	openIssues, err := e.gh.ListIssuesByLabel(ctx, labelReady)
	if err != nil {
		return nil, fmt.Errorf("gathering open issues: %w", err)
	}

	pendingIdeas, err := e.gh.ListIssuesByLabel(ctx, labelSuggestion)
	if err != nil {
		return nil, fmt.Errorf("gathering pending suggestions: %w", err)
	}

	return &ProjectContext{
		OpenIssues:    openIssues,
		RejectedIdeas: e.rejectionCache.titles,
		PendingIdeas:  pendingIdeas,
	}, nil
}

// buildPrompt constructs the AI prompt with project context.
func buildPrompt(projectCtx *ProjectContext) string {
	var b strings.Builder

	b.WriteString("You are reviewing a software project to suggest ONE high-impact improvement.\n\n")

	if len(projectCtx.OpenIssues) > 0 {
		b.WriteString("## Current Open Issues\n")
		for _, issue := range projectCtx.OpenIssues {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.Number, issue.Title)
		}
		b.WriteString("\n")
	}

	if len(projectCtx.PendingIdeas) > 0 {
		b.WriteString("## Pending Suggestions (already proposed)\n")
		for _, issue := range projectCtx.PendingIdeas {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.Number, issue.Title)
		}
		b.WriteString("\n")
	}

	if len(projectCtx.RejectedIdeas) > 0 {
		b.WriteString("## Previously Rejected Ideas (do NOT suggest these again)\n")
		for _, title := range projectCtx.RejectedIdeas {
			fmt.Fprintf(&b, "- %s\n", title)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Instructions\n")
	b.WriteString("Propose ONE specific, high-impact improvement for this project.\n")
	b.WriteString("Do NOT duplicate any existing issue or previously rejected idea.\n")
	b.WriteString("Focus on: code quality, performance, security, testing, documentation, or developer experience.\n")
	b.WriteString("Be concrete and actionable — include specific files, patterns, or areas to address.\n\n")
	b.WriteString("Respond with:\nTITLE: <concise issue title>\nBODY:\n<detailed markdown description>\n")

	return b.String()
}
