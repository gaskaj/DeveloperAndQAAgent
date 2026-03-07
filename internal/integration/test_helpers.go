package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/claude"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/developer"
	"github.com/gaskaj/OpenAgentFramework/internal/errors"
	"github.com/gaskaj/OpenAgentFramework/internal/gitops"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/orchestrator"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/stretchr/testify/require"
)

// TestEnvironment provides an isolated testing environment for agent integration tests
type TestEnvironment struct {
	t                *testing.T
	tempDir          string
	config           *config.Config
	githubClient     *MockGitHubClient
	claudeClient     *SimpleClaudeClient
	claudeAPIClient  *claude.Client
	store            *MockStore
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
	errorManager     *errors.Manager
	orchestrator     *orchestrator.Orchestrator
	agents           []agent.Agent
	mockServer       *MockClaudeServer
	bareRepoDir      string
}

// NewTestEnvironment creates a fresh test environment for each test
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	tempDir := t.TempDir()

	// Create a bare git repo that acts as the "remote" for cloning
	bareRepoDir := filepath.Join(tempDir, "bare-repo.git")
	initBareRepo(t, bareRepoDir)

	// Start mock Claude API server
	mockServer := NewMockClaudeServer()

	// Create a real Claude client pointing at the mock server
	claudeAPIClient := claude.NewClient("test-api-key", "claude-3-haiku-20240307", 4096,
		mockServer.RequestOption())

	// Create test configuration
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Owner:        "test-owner",
			Repo:         "test-repo",
			Token:        "test-token",
			PollInterval: 2 * time.Second,
			WatchLabels:  []string{"agent:ready"},
		},
		Claude: config.ClaudeConfig{
			APIKey:    "test-api-key",
			Model:     "claude-3-haiku-20240307",
			MaxTokens: 4096,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: filepath.Join(tempDir, "workspaces"),
			},
		},
		State: config.StateConfig{
			Dir: filepath.Join(tempDir, "state"),
		},
		Creativity: config.CreativityConfig{
			Enabled: false, // Disable creativity to avoid nil-client panics
		},
		Decomposition: config.DecompositionConfig{
			Enabled:            true,
			MaxIterationBudget: 50,
			MaxSubtasks:        5,
		},
	}

	// Create mock clients
	githubClient := NewMockGitHubClient()
	claudeClient := NewSimpleClaudeClient()
	store := NewMockStore()

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create observability components using config types
	loggingConfig := config.LoggingConfig{
		Level:             "debug",
		Format:            "json",
		EnableCorrelation: true,
		StructuredLogging: config.StructuredLoggingConfig{
			Enabled:           true,
			Format:            "json",
			IncludeCaller:     true,
			IncludeStackTrace: false,
			Correlation: config.CorrelationConfig{
				Enabled:              true,
				AutoGenerate:         true,
				IncludeWorkflowStage: true,
				IncludeAgentMetadata: true,
			},
			WorkflowTracking: config.WorkflowTrackingConfig{
				Enabled:            true,
				TrackHandoffs:      true,
				TrackDecisions:     true,
				IncludePerformance: true,
				TrackToolUsage:     true,
			},
			Performance: config.PerformanceLoggingConfig{
				TrackDurations:  true,
				MemorySnapshots: false,
				LLMMetrics:      true,
				WorkflowTiming:  true,
			},
		},
	}

	structuredLogger := observability.NewStructuredLogger(loggingConfig)

	metrics := observability.NewMetrics(structuredLogger)

	errorHandlingConfig := &config.ErrorHandlingConfig{
		Retry: config.RetryConfig{
			Enabled: true,
			DefaultPolicy: config.RetryPolicyConfig{
				MaxAttempts:   3,
				BaseDelay:     100 * time.Millisecond,
				MaxDelay:      time.Second,
				BackoffFactor: 2.0,
				JitterFactor:  0.1,
			},
		},
		CircuitBreaker: config.CircuitBreakerGroupConfig{
			Enabled: true,
			DefaultConfig: config.CircuitBreakerConfigSpec{
				MaxFailures:  5,
				Timeout:      30 * time.Second,
				MaxRequests:  10,
				FailureRatio: 0.5,
				MinRequests:  3,
			},
		},
	}
	errorManager := errors.NewManager(errorHandlingConfig, logger)

	te := &TestEnvironment{
		t:                t,
		tempDir:          tempDir,
		config:           cfg,
		githubClient:     githubClient,
		claudeClient:     claudeClient,
		claudeAPIClient:  claudeAPIClient,
		store:            store,
		logger:           logger,
		structuredLogger: structuredLogger,
		metrics:          metrics,
		errorManager:     errorManager,
		mockServer:       mockServer,
		bareRepoDir:      bareRepoDir,
	}

	// Override gitops.CloneFn to use local bare repo
	te.installGitMock()

	return te
}

// installGitMock overrides gitops.CloneFn to clone from a local bare repo
// and makes Push a no-op by removing the remote after clone.
func (te *TestEnvironment) installGitMock() {
	bareRepoDir := te.bareRepoDir
	origCloneFn := gitops.CloneFn

	gitops.CloneFn = func(url, dir, token string) (*gitops.Repo, error) {
		// Clone from the local bare repo instead of the URL
		repo, err := gogit.PlainClone(dir, false, &gogit.CloneOptions{
			URL: bareRepoDir,
		})
		if err != nil {
			return nil, fmt.Errorf("cloning repo: %w", err)
		}

		wt, err := repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("getting worktree: %w", err)
		}

		return gitops.NewRepoFromWorktree(repo, wt, dir, token), nil
	}

	te.t.Cleanup(func() {
		gitops.CloneFn = origCloneFn
	})
}

// CreateDependencies creates agent dependencies for testing
func (te *TestEnvironment) CreateDependencies() agent.Dependencies {
	return agent.Dependencies{
		Config:           te.config,
		GitHub:           te.githubClient,
		Claude:           te.claudeAPIClient,
		Store:            te.store,
		Logger:           te.logger,
		StructuredLogger: te.structuredLogger,
		Metrics:          te.metrics,
		ErrorManager:     te.errorManager,
	}
}

// CreateDeveloperAgent creates a developer agent for testing
func (te *TestEnvironment) CreateDeveloperAgent() (agent.Agent, error) {
	deps := te.CreateDependencies()
	return developer.New(deps)
}

// CreateOrchestrator creates an orchestrator with the given agents
func (te *TestEnvironment) CreateOrchestrator(agents []agent.Agent) *orchestrator.Orchestrator {
	te.agents = agents
	te.orchestrator = orchestrator.New(agents, te.logger).
		WithObservability(te.structuredLogger, te.metrics)
	return te.orchestrator
}

// RunWithTimeout runs a function with a timeout context
func (te *TestEnvironment) RunWithTimeout(timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return fn(ctx)
}

// AssertWorkflowState asserts that an agent has the expected workflow state
func (te *TestEnvironment) AssertWorkflowState(agentType string, expectedState state.WorkflowState) {
	ctx := context.Background()
	ws, err := te.store.Load(ctx, agentType)
	require.NoError(te.t, err)
	require.NotNil(te.t, ws)
	require.Equal(te.t, expectedState, ws.State)
}

// AssertIssueLabels asserts that an issue has the expected labels
func (te *TestEnvironment) AssertIssueLabels(issueNumber int, expectedLabels []string) {
	labels := te.githubClient.GetIssueLabels(issueNumber)
	labelMap := make(map[string]bool)
	for _, label := range labels {
		labelMap[label] = true
	}

	for _, expected := range expectedLabels {
		require.True(te.t, labelMap[expected],
			fmt.Sprintf("expected label %s not found, got: %v", expected, labels))
	}
}

// AssertCommentCreated asserts that a comment was created on an issue
func (te *TestEnvironment) AssertCommentCreated(issueNumber int, commentSubstring string) {
	comments := te.githubClient.GetIssueComments(issueNumber)
	found := false
	for _, comment := range comments {
		if comment.Contains(commentSubstring) {
			found = true
			break
		}
	}
	require.True(te.t, found,
		fmt.Sprintf("expected comment containing '%s' not found", commentSubstring))
}

// AssertPRCreated asserts that a PR was created
func (te *TestEnvironment) AssertPRCreated(expectedTitle string) {
	prs := te.githubClient.GetCreatedPRs()
	found := false
	for _, pr := range prs {
		if pr.Title == expectedTitle {
			found = true
			break
		}
	}
	require.True(te.t, found,
		fmt.Sprintf("expected PR with title '%s' not found", expectedTitle))
}

// AssertMetricsRecorded asserts that specific metrics were recorded
func (te *TestEnvironment) AssertMetricsRecorded(metricName string) {
	// Mock metrics implementation would need to track recorded metrics
	// This is a placeholder for metrics validation
}

// AssertHandoffLogged asserts that an agent handoff was logged
func (te *TestEnvironment) AssertHandoffLogged(fromAgent, toAgent string) {
	// Mock structured logger would need to track handoff events
	// This is a placeholder for handoff validation
}

// Cleanup cleans up test resources
func (te *TestEnvironment) Cleanup() {
	if te.mockServer != nil {
		te.mockServer.Close()
	}
}

// SimulateGitHubIssue creates a mock GitHub issue for testing
func (te *TestEnvironment) SimulateGitHubIssue(number int, title, body string, labels []string) *MockIssue {
	issue := &MockIssue{
		Number: number,
		Title:  title,
		Body:   body,
		Labels: labels,
	}
	te.githubClient.AddIssue(issue)
	return issue
}

// SimulateAgentFailure simulates an agent failure condition
func (te *TestEnvironment) SimulateAgentFailure(agentType string, err error) {
	switch agentType {
	case "github":
		te.githubClient.SimulateError(err)
	case "claude":
		te.claudeClient.SimulateError(err)
	case "store":
		te.store.SimulateError(err)
	}
}

// SimulateConcurrentAccess simulates concurrent access to shared resources
func (te *TestEnvironment) SimulateConcurrentAccess(concurrency int, fn func(int) error) error {
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			errCh <- fn(id)
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}

	return nil
}

// WaitForWorkflowTransition waits for an agent to transition to a specific state
func (te *TestEnvironment) WaitForWorkflowTransition(agentType string, targetState state.WorkflowState, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s to reach state %s", agentType, targetState)
		case <-ticker.C:
			ws, err := te.store.Load(context.Background(), agentType)
			if err != nil {
				continue
			}
			if ws != nil && ws.State == targetState {
				return nil
			}
		}
	}
}

// initBareRepo creates a bare git repository with an initial commit
// so that cloning and branching operations work properly.
func initBareRepo(t *testing.T, bareDir string) {
	t.Helper()

	// Create a temporary working repo, commit, then clone to bare
	workDir := t.TempDir()

	repo, err := gogit.PlainInit(workDir, false)
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	// Create an initial file so we have something to commit
	readmePath := filepath.Join(workDir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Test Repo\n"), 0o644))

	// Also create a go.mod so gatherRepoContext works
	gomodPath := filepath.Join(workDir, "go.mod")
	require.NoError(t, os.WriteFile(gomodPath, []byte("module github.com/test-owner/test-repo\n\ngo 1.25\n"), 0o644))

	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Add("go.mod")
	require.NoError(t, err)

	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Clone as bare repo
	_, err = gogit.PlainClone(bareDir, true, &gogit.CloneOptions{
		URL: workDir,
	})
	require.NoError(t, err)
}
