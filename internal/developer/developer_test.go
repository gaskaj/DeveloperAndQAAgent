package developer

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitHubClient implements ghub.Client for testing.
type mockGitHubClient struct{}

func (m *mockGitHubClient) ListIssues(_ context.Context, _ []string) ([]*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) GetIssue(_ context.Context, _ int) (*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) AssignIssue(_ context.Context, _ int, _ []string) error  { return nil }
func (m *mockGitHubClient) AssignSelfIfNoAssignees(_ context.Context, _ int) error { return nil }
func (m *mockGitHubClient) AddLabels(_ context.Context, _ int, _ []string) error    { return nil }
func (m *mockGitHubClient) RemoveLabel(_ context.Context, _ int, _ string) error     { return nil }
func (m *mockGitHubClient) CreateIssue(_ context.Context, _ string, _ string, _ []string) (*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) CreateBranch(_ context.Context, _ string, _ string) error { return nil }
func (m *mockGitHubClient) CreatePR(_ context.Context, _ ghub.PROptions) (*github.PullRequest, error) {
	return nil, nil
}
func (m *mockGitHubClient) ListPRs(_ context.Context, _ string) ([]*github.PullRequest, error) {
	return nil, nil
}
func (m *mockGitHubClient) CreateComment(_ context.Context, _ int, _ string) error { return nil }
func (m *mockGitHubClient) ListComments(_ context.Context, _ int) ([]*github.IssueComment, error) {
	return nil, nil
}

func newTestDeps(t *testing.T) agent.Dependencies {
	t.Helper()
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	return agent.Dependencies{
		Config: &config.Config{
			GitHub: config.GitHubConfig{
				Token:        "test-token",
				Owner:        "testowner",
				Repo:         "testrepo",
				PollInterval: 30 * time.Second,
				WatchLabels:  []string{"agent:ready"},
			},
			Claude: config.ClaudeConfig{
				APIKey:    "test-key",
				Model:     "claude-sonnet-4-20250514",
				MaxTokens: 4096,
			},
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Enabled:       true,
					MaxConcurrent: 1,
					WorkspaceDir:  dir,
				},
			},
		},
		GitHub: &mockGitHubClient{},
		Claude: claude.NewClient("test-key", "claude-sonnet-4-20250514", 4096),
		Store:  store,
		Logger: logger,
	}
}

func TestNew(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, agent.TypeDeveloper, a.Type())
}

func TestDeveloperAgent_Status(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	status := a.Status()
	assert.Equal(t, agent.TypeDeveloper, status.Type)
	assert.Equal(t, string(state.StateIdle), status.State)
	assert.Equal(t, "waiting for issues", status.Message)
}
