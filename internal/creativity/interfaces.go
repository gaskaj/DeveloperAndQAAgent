package creativity

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/google/go-github/v68/github"
)

// Issue is a simplified issue representation for the creativity package.
type Issue struct {
	Number int
	Title  string
	Body   string
	Labels []string
	State  string
}

// Suggestion represents a generated improvement suggestion.
type Suggestion struct {
	Title string
	Body  string
}

// GitHubClient defines the GitHub operations needed by the creativity engine.
type GitHubClient interface {
	ListIssuesByLabel(ctx context.Context, label string) ([]*Issue, error)
	ListClosedIssuesByLabel(ctx context.Context, label string) ([]*Issue, error)
	ListAllClosedIssues(ctx context.Context) ([]*Issue, error)
	CreateIssue(ctx context.Context, title, body string, labels []string) (int, error)
	AddLabels(ctx context.Context, number int, labels []string) error
	RemoveLabel(ctx context.Context, number int, label string) error
}

// AIClient defines the AI operations needed by the creativity engine.
type AIClient interface {
	GenerateSuggestion(ctx context.Context, prompt string) (*Suggestion, error)
}

// GitHubAdapter wraps a ghub.Client to satisfy the GitHubClient interface.
type GitHubAdapter struct {
	client ghub.Client
}

// NewGitHubAdapter creates a new GitHubAdapter.
func NewGitHubAdapter(client ghub.Client) *GitHubAdapter {
	return &GitHubAdapter{client: client}
}

// ListIssuesByLabel returns open issues matching a single label.
func (a *GitHubAdapter) ListIssuesByLabel(ctx context.Context, label string) ([]*Issue, error) {
	ghIssues, err := a.client.ListIssues(ctx, []string{label})
	if err != nil {
		return nil, fmt.Errorf("listing issues by label %q: %w", label, err)
	}
	return convertGitHubIssues(ghIssues), nil
}

// ListClosedIssuesByLabel returns closed issues matching a single label.
func (a *GitHubAdapter) ListClosedIssuesByLabel(ctx context.Context, label string) ([]*Issue, error) {
	ghIssues, err := a.client.ListIssuesByState(ctx, []string{label}, "closed")
	if err != nil {
		return nil, fmt.Errorf("listing closed issues by label %q: %w", label, err)
	}
	return convertGitHubIssues(ghIssues), nil
}

// ListAllClosedIssues returns all closed issues (no label filter).
func (a *GitHubAdapter) ListAllClosedIssues(ctx context.Context) ([]*Issue, error) {
	ghIssues, err := a.client.ListIssuesByState(ctx, nil, "closed")
	if err != nil {
		return nil, fmt.Errorf("listing all closed issues: %w", err)
	}
	return convertGitHubIssues(ghIssues), nil
}

// convertGitHubIssues converts go-github issues to creativity Issue types.
func convertGitHubIssues(ghIssues []*github.Issue) []*Issue {
	issues := make([]*Issue, 0, len(ghIssues))
	for _, gi := range ghIssues {
		issue := &Issue{
			Number: gi.GetNumber(),
			Title:  gi.GetTitle(),
			Body:   gi.GetBody(),
			State:  gi.GetState(),
		}
		for _, l := range gi.Labels {
			issue.Labels = append(issue.Labels, l.GetName())
		}
		issues = append(issues, issue)
	}
	return issues
}

// CreateIssue creates a new GitHub issue and returns its number.
func (a *GitHubAdapter) CreateIssue(ctx context.Context, title, body string, labels []string) (int, error) {
	issue, err := a.client.CreateIssue(ctx, title, body, labels)
	if err != nil {
		return 0, fmt.Errorf("creating issue: %w", err)
	}
	return issue.GetNumber(), nil
}

// AddLabels adds labels to an issue.
func (a *GitHubAdapter) AddLabels(ctx context.Context, number int, labels []string) error {
	return a.client.AddLabels(ctx, number, labels)
}

// RemoveLabel removes a label from an issue.
func (a *GitHubAdapter) RemoveLabel(ctx context.Context, number int, label string) error {
	return a.client.RemoveLabel(ctx, number, label)
}

// ClaudeAdapter wraps a claude.Client to satisfy the AIClient interface.
type ClaudeAdapter struct {
	client *claude.Client
}

// NewClaudeAdapter creates a new ClaudeAdapter.
func NewClaudeAdapter(client *claude.Client) *ClaudeAdapter {
	return &ClaudeAdapter{client: client}
}

// GenerateSuggestion sends a prompt to Claude and parses the response into a Suggestion.
func (a *ClaudeAdapter) GenerateSuggestion(ctx context.Context, prompt string) (*Suggestion, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
	}

	system := "You are a senior software engineer performing a thorough review of a project to suggest ONE high-impact improvement. " +
		"You have been given the full project context including codebase structure, documentation, open issues, closed issues, and prior suggestions. " +
		"Respond with exactly two sections: a TITLE line and a BODY section. " +
		"The TITLE should be a concise issue title (under 80 characters). " +
		"The BODY should be a detailed markdown description of the improvement, referencing specific files and packages.\n\n" +
		"Format:\nTITLE: <issue title>\nBODY:\n<detailed description>"

	msg, err := a.client.SendMessage(ctx, system, messages)
	if err != nil {
		return nil, fmt.Errorf("generating suggestion: %w", err)
	}

	text := claude.ExtractText(msg)
	return parseSuggestion(text)
}

// parseSuggestion extracts title and body from the Claude response.
// It handles case variations (TITLE/Title/title) and markdown formatting
// (**TITLE:**, ## TITLE:, etc.) that Claude may produce.
func parseSuggestion(text string) (*Suggestion, error) {
	normalized := normalizeResponse(text)
	lower := strings.ToLower(normalized)

	titleIdx := strings.Index(lower, "title:")
	bodyIdx := strings.Index(lower, "body:")

	if titleIdx == -1 || bodyIdx == -1 {
		return nil, fmt.Errorf("unexpected response format: missing TITLE or BODY section")
	}

	// Extract content after "title:" marker from the normalized text
	titleStart := titleIdx + len("title:")
	title := normalized[titleStart:bodyIdx]
	title = trimSpace(title)

	// Extract content after "body:" marker from the normalized text
	bodyStart := bodyIdx + len("body:")
	body := normalized[bodyStart:]
	body = trimSpace(body)

	if title == "" || body == "" {
		return nil, fmt.Errorf("empty title or body in suggestion")
	}

	return &Suggestion{Title: title, Body: body}, nil
}

// normalizeResponse strips markdown formatting (**, ##, #) from lines
// containing TITLE or BODY markers so that the markers can be found
// with a simple case-insensitive search.
func normalizeResponse(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "title:") || strings.Contains(lower, "body:") {
			// Strip leading # and ## markdown headers
			stripped := strings.TrimLeft(line, "# ")
			// Strip bold markdown markers
			stripped = strings.ReplaceAll(stripped, "**", "")
			lines[i] = stripped
		}
	}
	return strings.Join(lines, "\n")
}

// trimSpace trims whitespace and newlines from a string.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Ensure adapters satisfy interfaces at compile time.
var (
	_ GitHubClient = (*GitHubAdapter)(nil)
	_ AIClient     = (*ClaudeAdapter)(nil)
)
