package ghub

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
)

// CreatePR creates a new pull request.
func (c *GitHubClient) CreatePR(ctx context.Context, opts PROptions) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Create(ctx, c.owner, c.repo, &github.NewPullRequest{
		Title: github.Ptr(opts.Title),
		Body:  github.Ptr(opts.Body),
		Head:  github.Ptr(opts.Head),
		Base:  github.Ptr(opts.Base),
	})
	if err != nil {
		return nil, fmt.Errorf("creating pull request: %w", err)
	}
	return pr, nil
}

// ListPRs lists pull requests with the given state.
func (c *GitHubClient) ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error) {
	prs, _, err := c.client.PullRequests.List(ctx, c.owner, c.repo, &github.PullRequestListOptions{
		State: state,
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing pull requests: %w", err)
	}
	return prs, nil
}

// GetPR retrieves a specific pull request by number.
func (c *GitHubClient) GetPR(ctx context.Context, number int) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return nil, fmt.Errorf("getting pull request %d: %w", number, err)
	}
	return pr, nil
}

// MergePR squash-merges a pull request.
func (c *GitHubClient) MergePR(ctx context.Context, prNumber int, commitMessage string) error {
	_, _, err := c.client.PullRequests.Merge(ctx, c.owner, c.repo, prNumber, commitMessage, &github.PullRequestOptions{
		MergeMethod: "squash",
	})
	if err != nil {
		return fmt.Errorf("merging pull request %d: %w", prNumber, err)
	}
	return nil
}
