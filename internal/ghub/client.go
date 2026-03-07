package ghub

import (
	"context"

	agentErrors "github.com/gaskaj/OpenAgentFramework/internal/errors"
	"github.com/google/go-github/v68/github"
)

// Client defines the interface for GitHub operations.
type Client interface {
	// Issues
	ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error)
	ListIssuesByState(ctx context.Context, labels []string, state string) ([]*github.Issue, error)
	GetIssue(ctx context.Context, number int) (*github.Issue, error)
	AssignIssue(ctx context.Context, number int, assignees []string) error
	AssignSelfIfNoAssignees(ctx context.Context, number int) error
	AddLabels(ctx context.Context, number int, labels []string) error
	RemoveLabel(ctx context.Context, number int, label string) error

	// Branches
	CreateBranch(ctx context.Context, name string, fromRef string) error

	// Pull Requests
	CreatePR(ctx context.Context, opts PROptions) (*github.PullRequest, error)
	ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error)
	GetPR(ctx context.Context, number int) (*github.PullRequest, error)
	ValidatePR(ctx context.Context, prNumber int, opts PRValidationOptions) (*PRValidationResult, error)
	GetPRCheckStatus(ctx context.Context, prNumber int) (*PRValidationResult, error)
	MergePR(ctx context.Context, prNumber int, commitMessage string) error

	// Issues (create)
	CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error)

	// Comments
	CreateComment(ctx context.Context, number int, body string) error
	ListComments(ctx context.Context, number int) ([]*github.IssueComment, error)
}

// PROptions holds options for creating a pull request.
type PROptions struct {
	Title string
	Body  string
	Head  string
	Base  string
}

// GitHubClient implements Client using the go-github library.
type GitHubClient struct {
	client       *github.Client
	owner        string
	repo         string
	errorManager *agentErrors.Manager
}

// NewClient creates a new GitHubClient.
func NewClient(token, owner, repo string) *GitHubClient {
	client := github.NewClient(nil).WithAuthToken(token)
	return &GitHubClient{
		client: client,
		owner:  owner,
		repo:   repo,
	}
}

// WithErrorHandling adds error handling capabilities to the client
func (c *GitHubClient) WithErrorHandling(errorManager *agentErrors.Manager) *GitHubClient {
	c.errorManager = errorManager
	return c
}
