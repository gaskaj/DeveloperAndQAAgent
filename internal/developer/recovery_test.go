package developer

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// MockValidator mocks the state validator for testing
type MockValidator struct {
	mock.Mock
}

func (m *MockValidator) ValidateWorkState(ctx context.Context, workState *state.AgentWorkState) (*state.ValidationReport, error) {
	args := m.Called(ctx, workState)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*state.ValidationReport), args.Error(1)
}

func (m *MockValidator) DetectOrphanedWork(ctx context.Context) ([]*state.OrphanedWorkItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*state.OrphanedWorkItem), args.Error(1)
}

func (m *MockValidator) ReconcileState(ctx context.Context, workState *state.AgentWorkState) error {
	args := m.Called(ctx, workState)
	return args.Error(0)
}

// MockGitHub mocks the GitHub client for testing
type MockGitHub struct {
	mock.Mock
}

func (m *MockGitHub) ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error) {
	args := m.Called(ctx, labels)
	return args.Get(0).([]*github.Issue), args.Error(1)
}
func (m *MockGitHub) ListIssuesByState(ctx context.Context, labels []string, state string) ([]*github.Issue, error) {
	args := m.Called(ctx, labels, state)
	return args.Get(0).([]*github.Issue), args.Error(1)
}
func (m *MockGitHub) GetIssue(ctx context.Context, number int) (*github.Issue, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(*github.Issue), args.Error(1)
}
func (m *MockGitHub) AssignIssue(ctx context.Context, number int, assignees []string) error {
	args := m.Called(ctx, number, assignees)
	return args.Error(0)
}
func (m *MockGitHub) AssignSelfIfNoAssignees(ctx context.Context, number int) error {
	args := m.Called(ctx, number)
	return args.Error(0)
}
func (m *MockGitHub) AddLabels(ctx context.Context, number int, labels []string) error {
	args := m.Called(ctx, number, labels)
	return args.Error(0)
}
func (m *MockGitHub) RemoveLabel(ctx context.Context, number int, label string) error {
	args := m.Called(ctx, number, label)
	return args.Error(0)
}
func (m *MockGitHub) CreateBranch(ctx context.Context, name string, fromRef string) error {
	args := m.Called(ctx, name, fromRef)
	return args.Error(0)
}
func (m *MockGitHub) CreatePR(ctx context.Context, opts ghub.PROptions) (*github.PullRequest, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*github.PullRequest), args.Error(1)
}
func (m *MockGitHub) ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error) {
	args := m.Called(ctx, state)
	return args.Get(0).([]*github.PullRequest), args.Error(1)
}
func (m *MockGitHub) GetPR(ctx context.Context, number int) (*github.PullRequest, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(*github.PullRequest), args.Error(1)
}
func (m *MockGitHub) ValidatePR(ctx context.Context, prNumber int, opts ghub.PRValidationOptions) (*ghub.PRValidationResult, error) {
	args := m.Called(ctx, prNumber, opts)
	return args.Get(0).(*ghub.PRValidationResult), args.Error(1)
}
func (m *MockGitHub) GetPRCheckStatus(ctx context.Context, prNumber int) (*ghub.PRValidationResult, error) {
	args := m.Called(ctx, prNumber)
	return args.Get(0).(*ghub.PRValidationResult), args.Error(1)
}
func (m *MockGitHub) CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	args := m.Called(ctx, title, body, labels)
	return args.Get(0).(*github.Issue), args.Error(1)
}
func (m *MockGitHub) CreateComment(ctx context.Context, number int, body string) error {
	args := m.Called(ctx, number, body)
	return args.Error(0)
}
func (m *MockGitHub) ListComments(ctx context.Context, number int) ([]*github.IssueComment, error) {
	args := m.Called(ctx, number)
	return args.Get(0).([]*github.IssueComment), args.Error(1)
}

func TestRecoveryManager_AttemptResume(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						MaxResumeAge: 24 * time.Hour,
					},
				},
			},
		},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	ctx := context.Background()
	checkpointTime := time.Now().Add(-1 * time.Hour)
	workState := &state.AgentWorkState{
		AgentType:       "developer",
		IssueNumber:     123,
		State:           state.StateImplement,
		UpdatedAt:       checkpointTime,
		CheckpointedAt:  checkpointTime,
		CheckpointStage: "implement",
	}

	// Mock validation report showing clean state
	validationReport := &state.ValidationReport{
		Valid:              true,
		IssuesFound:        []*state.ValidationIssue{},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, workState).Return(validationReport, nil)

	plan, err := recoveryManager.AttemptResume(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, plan)
	assert.True(t, plan.CanResume)
	assert.Equal(t, RecommendResume, plan.RecommendedAction)
	assert.Equal(t, RiskLow, plan.RiskLevel)

	mockValidator.AssertExpectations(t)
}

func TestRecoveryManager_AttemptResume_HighRisk(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	ctx := context.Background()
	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateImplement,
		UpdatedAt:   time.Now().Add(-20 * time.Hour), // Very old
		Error:       "some critical error",
	}

	// Mock validation report with critical issues
	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{
				Type:     state.IssueTypeInconsistentState,
				Severity: state.SeverityCritical,
			},
			{
				Type:     state.IssueTypeStaleWorkspace,
				Severity: state.SeverityHigh,
			},
		},
		StateDrifts: []*state.StateDrift{
			{Type: state.DriftTypeIssueState},
		},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, workState).Return(validationReport, nil)

	plan, err := recoveryManager.AttemptResume(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, plan)
	assert.False(t, plan.CanResume)
	assert.Equal(t, RecommendCleanup, plan.RecommendedAction)
	assert.Equal(t, RiskCritical, plan.RiskLevel)

	mockValidator.AssertExpectations(t)
}

func TestRecoveryManager_CleanupOrphanedWork(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}
	mockStore := &MockStore{}
	mockGitHub := &MockGitHub{}

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGitHub,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	ctx := context.Background()
	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  123,
		State:        state.StateClaim,
		RecoveryType: state.RecoveryTypeCleanup,
	}

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateClaim,
	}

	// Mock store operations
	mockStore.On("Load", ctx, "developer").Return(workState, nil)
	mockStore.On("Delete", ctx, "developer").Return(nil)

	// Mock GitHub operations
	mockGitHub.On("RemoveLabel", ctx, 123, "agent:claimed").Return(nil)
	mockGitHub.On("AddLabels", ctx, 123, []string{"agent:ready"}).Return(nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)

	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
	mockGitHub.AssertExpectations(t)
}

func TestRecoveryManager_ValidateWorkspaceConsistency(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	ctx := context.Background()

	// Test with empty workspace dir
	err := recoveryManager.ValidateWorkspaceConsistency(ctx, "", 123)
	assert.NoError(t, err) // Should handle empty dir gracefully

	// Test with non-existent workspace dir
	err = recoveryManager.ValidateWorkspaceConsistency(ctx, "/non/existent/path", 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "workspace directory does not exist")
}

func Test_canResumeWork(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	tests := []struct {
		name     string
		state    *state.AgentWorkState
		report   *state.ValidationReport
		expected bool
	}{
		{
			name: "can resume recent work with no issues",
			state: &state.AgentWorkState{
				State:           state.StateImplement,
				UpdatedAt:       time.Now().Add(-1 * time.Hour),
				CheckpointedAt:  time.Now().Add(-1 * time.Hour),
				CheckpointStage: "implement",
			},
			report: &state.ValidationReport{
				Valid:       true,
				IssuesFound: []*state.ValidationIssue{},
				StateDrifts: []*state.StateDrift{},
			},
			expected: true,
		},
		{
			name: "cannot resume terminal state",
			state: &state.AgentWorkState{
				State:     state.StateComplete,
				UpdatedAt: time.Now().Add(-1 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid: true,
			},
			expected: false,
		},
		{
			name: "cannot resume very old work",
			state: &state.AgentWorkState{
				State:     state.StateImplement,
				UpdatedAt: time.Now().Add(-25 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid: true,
			},
			expected: false,
		},
		{
			name: "cannot resume with critical issues",
			state: &state.AgentWorkState{
				State:     state.StateImplement,
				UpdatedAt: time.Now().Add(-1 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid: false,
				IssuesFound: []*state.ValidationIssue{
					{Severity: state.SeverityCritical},
				},
			},
			expected: false,
		},
		{
			name: "cannot resume closed issue",
			state: &state.AgentWorkState{
				State:     state.StateImplement,
				UpdatedAt: time.Now().Add(-1 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid: false,
				StateDrifts: []*state.StateDrift{
					{
						Type:          state.DriftTypeIssueState,
						ExternalState: "closed",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recoveryManager.canResumeWork(tt.state, tt.report)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_assessResumptionRisk(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	tests := []struct {
		name     string
		state    *state.AgentWorkState
		report   *state.ValidationReport
		expected ResumptionRisk
	}{
		{
			name: "low risk - recent with no issues",
			state: &state.AgentWorkState{
				State:     state.StateAnalyze,
				UpdatedAt: time.Now().Add(-1 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid:       true,
				IssuesFound: []*state.ValidationIssue{},
				StateDrifts: []*state.StateDrift{},
			},
			expected: RiskLow,
		},
		{
			name: "high risk - old with critical issues",
			state: &state.AgentWorkState{
				State:     state.StatePR,
				UpdatedAt: time.Now().Add(-15 * time.Hour),
				Error:     "some error",
			},
			report: &state.ValidationReport{
				Valid: false,
				IssuesFound: []*state.ValidationIssue{
					{Severity: state.SeverityCritical},
					{Severity: state.SeverityHigh},
				},
				StateDrifts: []*state.StateDrift{
					{Type: state.DriftTypeIssueState},
					{Type: state.DriftTypeBranchState},
				},
			},
			expected: RiskCritical,
		},
		{
			name: "medium risk - some issues",
			state: &state.AgentWorkState{
				State:     state.StateImplement,
				UpdatedAt: time.Now().Add(-3 * time.Hour),
			},
			report: &state.ValidationReport{
				Valid: false,
				IssuesFound: []*state.ValidationIssue{
					{Severity: state.SeverityMedium},
				},
				StateDrifts: []*state.StateDrift{
					{Type: state.DriftTypeIssueState},
				},
			},
			expected: RiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recoveryManager.assessResumptionRisk(tt.state, tt.report)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// MockStore is defined in validator_test.go but redefined here for clarity
type MockStore struct {
	mock.Mock
}

func (m *MockStore) Save(ctx context.Context, state *state.AgentWorkState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func (m *MockStore) Load(ctx context.Context, agentType string) (*state.AgentWorkState, error) {
	args := m.Called(ctx, agentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*state.AgentWorkState), args.Error(1)
}

func (m *MockStore) Delete(ctx context.Context, agentType string) error {
	args := m.Called(ctx, agentType)
	return args.Error(0)
}

func (m *MockStore) List(ctx context.Context) ([]*state.AgentWorkState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*state.AgentWorkState), args.Error(1)
}