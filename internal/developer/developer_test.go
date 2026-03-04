package developer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitHubClient implements ghub.Client for testing.
type mockGitHubClient struct{}

func (m *mockGitHubClient) ListIssues(_ context.Context, _ []string) ([]*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) ListIssuesByState(_ context.Context, _ []string, _ string) ([]*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) GetIssue(_ context.Context, _ int) (*github.Issue, error) {
	return nil, nil
}
func (m *mockGitHubClient) AssignIssue(_ context.Context, _ int, _ []string) error { return nil }
func (m *mockGitHubClient) AssignSelfIfNoAssignees(_ context.Context, _ int) error { return nil }
func (m *mockGitHubClient) AddLabels(_ context.Context, _ int, _ []string) error   { return nil }
func (m *mockGitHubClient) RemoveLabel(_ context.Context, _ int, _ string) error   { return nil }
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
func (m *mockGitHubClient) GetPR(_ context.Context, _ int) (*github.PullRequest, error) {
	return nil, nil
}
func (m *mockGitHubClient) ValidatePR(_ context.Context, _ int, _ ghub.PRValidationOptions) (*ghub.PRValidationResult, error) {
	// Mock successful validation
	return &ghub.PRValidationResult{
		Status:           ghub.PRCheckStatusSuccess,
		AllChecksPassing: true,
		FailedChecks:     []ghub.CheckFailure{},
		PendingChecks:    []string{},
		TotalChecks:      2,
	}, nil
}
func (m *mockGitHubClient) GetPRCheckStatus(_ context.Context, _ int) (*ghub.PRValidationResult, error) {
	return &ghub.PRValidationResult{
		Status:           ghub.PRCheckStatusSuccess,
		AllChecksPassing: true,
	}, nil
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

func TestDeveloperAgent_Type(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	assert.Equal(t, agent.TypeDeveloper, a.Type())
}

func TestDeveloperAgent_UpdateStatus(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	da := a.(*DeveloperAgent)
	da.updateStatus(state.StateImplement, 42, "implementing changes")

	status := da.Status()
	assert.Equal(t, string(state.StateImplement), status.State)
	assert.Equal(t, 42, status.IssueID)
	assert.Equal(t, "implementing changes", status.Message)
}

func TestDeveloperAgent_Logger(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	da := a.(*DeveloperAgent)
	logger := da.logger()
	assert.NotNil(t, logger)
}

func TestDeveloperAgent_New_WithDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Config with zero values to test default filling
	deps := agent.Dependencies{
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

	a, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestDeveloperAgent_StatusWithWorkspaceStats(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	da := a.(*DeveloperAgent)

	// Status should include workspace stats
	status := da.Status()
	if status.WorkspaceStats != nil {
		assert.GreaterOrEqual(t, status.WorkspaceStats.TotalWorkspaces, 0)
	}
}

func TestStartupValidationReport(t *testing.T) {
	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
		ValidatedAt:        time.Now(),
	}

	assert.True(t, report.Valid)
	assert.True(t, report.StartupSafe)
	assert.Empty(t, report.OrphanedWorkFound)
	assert.Empty(t, report.RecoveryActions)
}

func TestStartupValidator_ValidateExistingState(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockGH := newTrackingMock()
	mockGH.issues[42] = &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("Test issue"),
		State:  github.Ptr("open"),
		Labels: []*github.Label{{Name: github.Ptr("agent:claimed")}},
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: mockGH,
	}

	validator := state.NewStateValidator(store, mockGH, logger)
	sv := NewStartupValidator(deps, validator)

	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err = sv.validateExistingState(context.Background(), existingState, report)
	assert.NoError(t, err)
}

func TestStartupValidator_DetectOrphanedWork(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockGH := &mockGitHubClient{}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: mockGH,
	}

	validator := state.NewStateValidator(store, mockGH, logger)
	sv := NewStartupValidator(deps, validator)

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err = sv.detectOrphanedWork(context.Background(), report)
	assert.NoError(t, err)
}

func TestRecoveryAction_Types(t *testing.T) {
	assert.Equal(t, RecoveryActionType("cleanup_orphaned"), ActionCleanupOrphaned)
	assert.Equal(t, RecoveryActionType("resume_work"), ActionResumeWork)
	assert.Equal(t, RecoveryActionType("validate_state"), ActionValidateState)
	assert.Equal(t, RecoveryActionType("reconcile_drift"), ActionReconcileDrift)
	assert.Equal(t, RecoveryActionType("flag_for_manual"), ActionFlagForManual)
}

// --- StartupValidator tests ---

func TestNewStartupValidator(t *testing.T) {
	deps := newTestDeps(t)
	mockValidator := &state.StateValidator{}

	sv := NewStartupValidator(deps, mockValidator)
	assert.NotNil(t, sv)
	assert.NotNil(t, sv.logger)
}

func TestStartupValidator_ValidateAndRecoverStartup_NoExistingState(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mockGH := &mockGitHubClient{}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: mockGH,
	}

	// Use a real StateValidator with the mock GitHub
	validator := state.NewStateValidator(store, mockGH, logger)

	sv := NewStartupValidator(deps, validator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.Valid)
	assert.True(t, report.StartupSafe)
	assert.Empty(t, report.OrphanedWorkFound)
}

func TestStartupValidator_ValidateAndRecoverStartup_WithExistingState(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Use a mock that returns non-nil issue to prevent nil pointer dereference
	mockGH := newTrackingMock()
	mockGH.issues[42] = &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("Test issue"),
		State:  github.Ptr("open"),
		Labels: []*github.Label{{Name: github.Ptr("agent:claimed")}},
	}

	// Save existing state
	existingState := &state.AgentWorkState{
		AgentType:   string(agent.TypeDeveloper),
		IssueNumber: 42,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, store.Save(context.Background(), existingState))

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: mockGH,
	}

	validator := state.NewStateValidator(store, mockGH, logger)
	sv := NewStartupValidator(deps, validator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.NotNil(t, report.ValidatedAt)
	assert.True(t, report.ValidationDuration > 0)
}

func TestStartupValidator_ValidateExistingState_InvalidReport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockValidator := &mockValidatorForDev{
		validateResult: &state.ValidationReport{
			Valid: false,
			IssuesFound: []*state.ValidationIssue{
				{Type: state.IssueTypeInconsistentState, Severity: state.SeverityCritical, Description: "state mismatch"},
			},
			OrphanedWork: []*state.OrphanedWorkItem{
				{AgentType: "developer", IssueNumber: 42, State: state.StateImplement, RecoveryType: state.RecoveryTypeCleanup},
			},
			StateDrifts: []*state.StateDrift{
				{Type: state.DriftTypeIssueState, ExternalState: "closed"},
			},
			RecommendedActions: []*state.RecommendedAction{
				{Type: state.ActionTypeCleanup, Description: "cleanup needed"},
			},
		},
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err := sv.validateExistingState(context.Background(), existingState, report)
	assert.NoError(t, err)

	// Report should be marked invalid
	assert.False(t, report.Valid)
	assert.False(t, report.StartupSafe)
	assert.NotEmpty(t, report.ValidationIssues)
	assert.NotEmpty(t, report.OrphanedWorkFound)
	assert.NotEmpty(t, report.RecommendedActions)
}

func TestStartupValidator_ValidateExistingState_ValidatorError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockValidator := &mockValidatorForDev{
		validateError: fmt.Errorf("validation error"),
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err := sv.validateExistingState(context.Background(), existingState, report)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validating existing state")
}

func TestStartupValidator_DetectOrphanedWork_WithOrphans(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockValidator := &mockValidatorForDev{
		detectResult: []*state.OrphanedWorkItem{
			{
				AgentType:    "developer",
				IssueNumber:  42,
				State:        state.StateImplement,
				AgeHours:     48.0,
				RecoveryType: state.RecoveryTypeCleanup,
			},
			{
				AgentType:    "developer",
				IssueNumber:  43,
				State:        state.StatePR,
				AgeHours:     12.0,
				RecoveryType: state.RecoveryTypeResume,
			},
		},
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err := sv.detectOrphanedWork(context.Background(), report)
	assert.NoError(t, err)

	assert.Len(t, report.OrphanedWorkFound, 2)
	assert.False(t, report.Valid)
}

func TestStartupValidator_DetectOrphanedWork_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockValidator := &mockValidatorForDev{
		detectError: fmt.Errorf("detect error"),
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
	}

	err := sv.detectOrphanedWork(context.Background(), report)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "detecting orphaned work")
}

func TestValidateAndRecoverStartup_WithOrphans(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	mockValidator := &mockValidatorForDev{
		detectResult: []*state.OrphanedWorkItem{
			{AgentType: "developer", IssueNumber: 42, State: state.StateImplement, RecoveryType: state.RecoveryTypeCleanup},
		},
		validateResult: &state.ValidationReport{
			Valid:              true,
			IssuesFound:        []*state.ValidationIssue{},
			OrphanedWork:       []*state.OrphanedWorkItem{},
			StateDrifts:        []*state.StateDrift{},
			RecommendedActions: []*state.RecommendedAction{},
		},
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.Valid) // Has orphans
	assert.NotEmpty(t, report.OrphanedWorkFound)
}

func TestValidateAndRecoverStartup_ValidateError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	// Save an existing state so validateExistingState is called
	existingState := &state.AgentWorkState{
		AgentType:   string(agent.TypeDeveloper),
		IssueNumber: 42,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, store.Save(context.Background(), existingState))

	mockValidator := &mockValidatorForDev{
		validateError: fmt.Errorf("validation failed"),
		detectResult:  []*state.OrphanedWorkItem{},
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	require.NoError(t, err) // ValidateAndRecoverStartup doesn't return validateExistingState errors
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe) // validateExistingState failed
}

func TestValidateAndRecoverStartup_DetectError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	mockValidator := &mockValidatorForDev{
		detectError: fmt.Errorf("detect failed"),
	}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  store,
		Logger: logger,
		GitHub: &mockGitHubClient{},
	}

	sv := NewStartupValidator(deps, mockValidator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	require.NoError(t, err) // Doesn't return detect errors
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe)
}

// mockValidatorForDev is a simple mock for state.Validator
type mockValidatorForDev struct {
	validateResult *state.ValidationReport
	validateError  error
	detectResult   []*state.OrphanedWorkItem
	detectError    error
}

func (m *mockValidatorForDev) ValidateWorkState(_ context.Context, _ *state.AgentWorkState) (*state.ValidationReport, error) {
	return m.validateResult, m.validateError
}

func (m *mockValidatorForDev) DetectOrphanedWork(_ context.Context) ([]*state.OrphanedWorkItem, error) {
	return m.detectResult, m.detectError
}

func (m *mockValidatorForDev) ReconcileState(_ context.Context, _ *state.AgentWorkState) error {
	return nil
}

func TestNew_WithRecoveryEnabled(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	deps := agent.Dependencies{
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
					Recovery: config.RecoveryConfig{
						Enabled:           true,
						StartupValidation: true,
					},
				},
			},
		},
		GitHub: &mockGitHubClient{},
		Claude: claude.NewClient("test-key", "claude-sonnet-4-20250514", 4096),
		Store:  store,
		Logger: logger,
	}

	a, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNew_WithAllWorkspaceDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Config with ALL workspace values at zero to test all default branches
	deps := agent.Dependencies{
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
			// Leave Workspace config entirely zero-valued
		},
		GitHub: &mockGitHubClient{},
		Claude: claude.NewClient("test-key", "claude-sonnet-4-20250514", 4096),
		Store:  store,
		Logger: logger,
	}

	a, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, agent.TypeDeveloper, a.Type())
}

func TestNew_WithCreativityEnabled(t *testing.T) {
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	deps := agent.Dependencies{
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
			Creativity: config.CreativityConfig{
				Enabled: true,
			},
		},
		GitHub: &mockGitHubClient{},
		Claude: claude.NewClient("test-key", "claude-sonnet-4-20250514", 4096),
		Store:  store,
		Logger: logger,
	}

	a, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestValidateAndRecoverStartup_LoadError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockStore := &MockStoreForDev{}

	mockGH := &mockGitHubClient{}

	deps := agent.Dependencies{
		Config: &config.Config{},
		Store:  mockStore,
		Logger: logger,
		GitHub: mockGH,
	}

	validator := state.NewStateValidator(mockStore, mockGH, logger)
	sv := NewStartupValidator(deps, validator)

	report, err := sv.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
	// MockStoreForDev.Load returns error
	assert.Error(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe)
}

// MockStoreForDev is a store that returns errors for testing developer_test.go
type MockStoreForDev struct{}

func (m *MockStoreForDev) Save(_ context.Context, _ *state.AgentWorkState) error {
	return fmt.Errorf("mock save error")
}
func (m *MockStoreForDev) Load(_ context.Context, _ string) (*state.AgentWorkState, error) {
	return nil, fmt.Errorf("mock load error")
}
func (m *MockStoreForDev) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("mock delete error")
}
func (m *MockStoreForDev) List(_ context.Context) ([]*state.AgentWorkState, error) {
	return nil, fmt.Errorf("mock list error")
}

func TestDeveloperAgent_ProcessIssue_ContextCancelled(t *testing.T) {
	mock := newTrackingMock()
	da := newTestAgent(t, mock, false)

	issue := &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("Test issue"),
		Body:   github.Ptr("Test body"),
	}

	// Cancel context before calling processIssue
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := da.processIssue(ctx, issue)
	assert.Error(t, err)
	// Should fail because context is cancelled before claiming
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDeveloperAgent_ProcessIssue_NilWorkspaceManager(t *testing.T) {
	mock := newTrackingMock()
	da := newTestAgent(t, mock, false)
	// workspaceManager is nil by default from newTestAgent

	issue := &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("Test issue"),
		Body:   github.Ptr("Test body"),
	}

	err := da.processIssue(context.Background(), issue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "workspace manager not initialized")

	// Should have posted a failure comment
	require.NotEmpty(t, mock.comments[42])
}

func TestDeveloperAgent_Run_CancelledContext(t *testing.T) {
	deps := newTestDeps(t)
	a, err := New(deps)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = a.Run(ctx)
	// Should return quickly - may return nil or context error
	// The important thing is it doesn't hang
	_ = err
}

func TestDeveloperAgent_ProcessIssue_ContextCancelledDuringWorkspace(t *testing.T) {
	mock := newTrackingMock()
	da := newTestAgent(t, mock, false)

	// Set up a real workspace manager
	tempDir := t.TempDir()
	mgr, err := workspace.NewManager(workspace.ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 3,
	}, da.Deps.Logger)
	require.NoError(t, err)
	da.workspaceManager = mgr

	issue := &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("Test issue"),
		Body:   github.Ptr("Test body"),
	}

	// Create a context that will be cancelled after claim but before workspace setup
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel right after function starts processing (will be detected at workspace stage)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = da.processIssue(ctx, issue)
	// Should fail - either context cancelled or workspace setup error
	assert.Error(t, err)
}
