package ghub

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
	agentErrors "github.com/gaskaj/DeveloperAndQAAgent/internal/errors"
)

// ListIssues returns open issues matching the given labels.
func (c *GitHubClient) ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error) {
	return c.ListIssuesByState(ctx, labels, "open")
}

// ListIssuesByState returns issues matching the given labels and state (e.g. "open", "closed", "all").
func (c *GitHubClient) ListIssuesByState(ctx context.Context, labels []string, state string) ([]*github.Issue, error) {
	if c.errorManager != nil {
		retryer := c.errorManager.GetRetryer("github_api")
		return agentErrors.Execute(ctx, retryer, func(ctx context.Context, attempt int) ([]*github.Issue, error) {
			return c.listIssuesCore(ctx, labels, state)
		})
	}

	return c.listIssuesCore(ctx, labels, state)
}

// listIssuesCore contains the core listing logic
func (c *GitHubClient) listIssuesCore(ctx context.Context, labels []string, issueState string) ([]*github.Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State:  issueState,
		Labels: labels,
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	}

	issues, _, err := c.client.Issues.ListByRepo(ctx, c.owner, c.repo, opts)
	if err != nil {
		return nil, agentErrors.ClassifyError(fmt.Errorf("listing issues: %w", err))
	}

	// Filter out pull requests (GitHub API returns them as issues too).
	var filtered []*github.Issue
	for _, issue := range issues {
		if issue.PullRequestLinks == nil {
			filtered = append(filtered, issue)
		}
	}

	return filtered, nil
}

// GetIssue retrieves a single issue by number.
func (c *GitHubClient) GetIssue(ctx context.Context, number int) (*github.Issue, error) {
	if c.errorManager != nil {
		retryer := c.errorManager.GetRetryer("github_api")
		return agentErrors.Execute(ctx, retryer, func(ctx context.Context, attempt int) (*github.Issue, error) {
			return c.getIssueCore(ctx, number)
		})
	}
	
	return c.getIssueCore(ctx, number)
}

// getIssueCore contains the core issue retrieval logic
func (c *GitHubClient) getIssueCore(ctx context.Context, number int) (*github.Issue, error) {
	issue, _, err := c.client.Issues.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return nil, agentErrors.ClassifyError(fmt.Errorf("getting issue #%d: %w", number, err))
	}
	return issue, nil
}

// AssignIssue assigns users to an issue.
func (c *GitHubClient) AssignIssue(ctx context.Context, number int, assignees []string) error {
	_, _, err := c.client.Issues.AddAssignees(ctx, c.owner, c.repo, number, assignees)
	if err != nil {
		return fmt.Errorf("assigning issue #%d: %w", number, err)
	}
	return nil
}

// AssignSelfIfNoAssignees assigns the authenticated user to an issue if it has no assignees.
func (c *GitHubClient) AssignSelfIfNoAssignees(ctx context.Context, number int) error {
	// Get the current issue to check assignees
	issue, err := c.GetIssue(ctx, number)
	if err != nil {
		return fmt.Errorf("getting issue #%d: %w", number, err)
	}

	// If issue already has assignees, do nothing
	if len(issue.Assignees) > 0 {
		return nil
	}

	// Get the authenticated user
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("getting authenticated user: %w", err)
	}

	// Assign self to the issue
	return c.AssignIssue(ctx, number, []string{user.GetLogin()})
}

// AddLabels adds labels to an issue.
func (c *GitHubClient) AddLabels(ctx context.Context, number int, labels []string) error {
	_, _, err := c.client.Issues.AddLabelsToIssue(ctx, c.owner, c.repo, number, labels)
	if err != nil {
		return fmt.Errorf("adding labels to issue #%d: %w", number, err)
	}
	return nil
}

// CreateIssue creates a new issue with the given title, body, and labels.
func (c *GitHubClient) CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	if c.errorManager != nil {
		retryer := c.errorManager.GetRetryer("github_api")
		return agentErrors.Execute(ctx, retryer, func(ctx context.Context, attempt int) (*github.Issue, error) {
			return c.createIssueCore(ctx, title, body, labels)
		})
	}
	
	return c.createIssueCore(ctx, title, body, labels)
}

// createIssueCore contains the core issue creation logic
func (c *GitHubClient) createIssueCore(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	req := &github.IssueRequest{
		Title:  &title,
		Body:   &body,
		Labels: &labels,
	}

	issue, _, err := c.client.Issues.Create(ctx, c.owner, c.repo, req)
	if err != nil {
		return nil, agentErrors.ClassifyError(fmt.Errorf("creating issue: %w", err))
	}
	return issue, nil
}

// RemoveLabel removes a label from an issue.
func (c *GitHubClient) RemoveLabel(ctx context.Context, number int, label string) error {
	_, err := c.client.Issues.RemoveLabelForIssue(ctx, c.owner, c.repo, number, label)
	if err != nil {
		return fmt.Errorf("removing label %q from issue #%d: %w", label, number, err)
	}
	return nil
}
