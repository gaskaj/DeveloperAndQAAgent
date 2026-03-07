package developer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
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

// --- determineResumeState tests ---

func Test_determineResumeState(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	tests := []struct {
		name     string
		state    *state.AgentWorkState
		expected state.WorkflowState
	}{
		{
			name: "checkpoint stage analysis",
			state: &state.AgentWorkState{
				State:           state.StateAnalyze,
				CheckpointStage: "analysis",
			},
			expected: state.StateAnalyze,
		},
		{
			name: "checkpoint stage workspace",
			state: &state.AgentWorkState{
				State:           state.StateWorkspace,
				CheckpointStage: "workspace",
			},
			expected: state.StateWorkspace,
		},
		{
			name: "checkpoint stage implementation",
			state: &state.AgentWorkState{
				State:           state.StateImplement,
				CheckpointStage: "implementation",
			},
			expected: state.StateImplement,
		},
		{
			name: "checkpoint stage commit",
			state: &state.AgentWorkState{
				State:           state.StateCommit,
				CheckpointStage: "commit",
			},
			expected: state.StateCommit,
		},
		{
			name: "checkpoint stage pr",
			state: &state.AgentWorkState{
				State:           state.StatePR,
				CheckpointStage: "pr",
			},
			expected: state.StatePR,
		},
		{
			name: "checkpoint stage unknown falls back to current",
			state: &state.AgentWorkState{
				State:           state.StateValidation,
				CheckpointStage: "unknown_stage",
			},
			expected: state.StateValidation,
		},
		{
			name: "no checkpoint - claim state",
			state: &state.AgentWorkState{
				State: state.StateClaim,
			},
			expected: state.StateClaim,
		},
		{
			name: "no checkpoint - analyze state",
			state: &state.AgentWorkState{
				State: state.StateAnalyze,
			},
			expected: state.StateAnalyze,
		},
		{
			name: "no checkpoint - workspace state",
			state: &state.AgentWorkState{
				State: state.StateWorkspace,
			},
			expected: state.StateWorkspace,
		},
		{
			name: "no checkpoint - implement steps back to workspace",
			state: &state.AgentWorkState{
				State: state.StateImplement,
			},
			expected: state.StateWorkspace,
		},
		{
			name: "no checkpoint - commit steps back to implement",
			state: &state.AgentWorkState{
				State: state.StateCommit,
			},
			expected: state.StateImplement,
		},
		{
			name: "no checkpoint - pr steps back to commit",
			state: &state.AgentWorkState{
				State: state.StatePR,
			},
			expected: state.StateCommit,
		},
		{
			name: "no checkpoint - validation steps back to pr",
			state: &state.AgentWorkState{
				State: state.StateValidation,
			},
			expected: state.StatePR,
		},
		{
			name: "no checkpoint - review steps back to validation",
			state: &state.AgentWorkState{
				State: state.StateReview,
			},
			expected: state.StateValidation,
		},
		{
			name: "no checkpoint - default uses current state",
			state: &state.AgentWorkState{
				State: state.StateFailed,
			},
			expected: state.StateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recoveryManager.determineResumeState(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- estimateResumptionDuration tests ---

func Test_estimateResumptionDuration(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	tests := []struct {
		name        string
		state       *state.AgentWorkState
		minDuration time.Duration
	}{
		{
			name:        "claim state",
			state:       &state.AgentWorkState{State: state.StateClaim},
			minDuration: 10 * time.Minute, // 5 min base + 10 min overhead
		},
		{
			name:        "implement state (steps back to workspace)",
			state:       &state.AgentWorkState{State: state.StateImplement},
			minDuration: 10 * time.Minute,
		},
		{
			name:        "unknown state gets default",
			state:       &state.AgentWorkState{State: "unknown"},
			minDuration: 20 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := recoveryManager.estimateResumptionDuration(tt.state)
			assert.True(t, duration >= tt.minDuration, "expected at least %v, got %v", tt.minDuration, duration)
		})
	}
}

// --- generateCleanupActions tests ---

func Test_generateCleanupActions(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	report := &state.ValidationReport{
		IssuesFound: []*state.ValidationIssue{
			{
				Type:     state.IssueTypeStaleWorkspace,
				Severity: state.SeverityCritical,
			},
			{
				Type:     state.IssueTypeBranchDrift,
				Severity: state.SeverityMedium,
			},
			{
				Type:     state.IssueTypeCheckpointCorrupt,
				Severity: state.SeverityHigh,
			},
		},
		StateDrifts: []*state.StateDrift{
			{
				Type:         state.DriftTypeIssueState,
				CanReconcile: false,
			},
		},
	}

	ws := &state.AgentWorkState{
		State: state.StateImplement,
	}

	actions := recoveryManager.generateCleanupActions(ws, report)
	assert.NotEmpty(t, actions)

	// Should have workspace cleanup, branch cleanup, checkpoint cleanup, and state cleanup
	types := make(map[CleanupActionType]bool)
	for _, action := range actions {
		types[action.Type] = true
	}
	assert.True(t, types[CleanupWorkspace])
	assert.True(t, types[CleanupBranch])
	assert.True(t, types[CleanupCheckpoint])
	assert.True(t, types[CleanupState])
}

func Test_generateCleanupActions_EmptyReport(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	report := &state.ValidationReport{
		IssuesFound: []*state.ValidationIssue{},
		StateDrifts: []*state.StateDrift{},
	}

	ws := &state.AgentWorkState{
		State: state.StateImplement,
	}

	actions := recoveryManager.generateCleanupActions(ws, report)
	assert.Empty(t, actions)
}

// --- makeResumptionRecommendation tests ---

func Test_makeResumptionRecommendation(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	tests := []struct {
		name             string
		workState        *state.AgentWorkState
		plan             *ResumptionPlan
		report           *state.ValidationReport
		expectedAction   RecommendedResumption
		expectedContains string
	}{
		{
			name:      "cannot resume",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume:       false,
				RiskLevel:       RiskLow,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendCleanup,
			expectedContains: "not eligible",
		},
		{
			name:      "critical risk",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume:       true,
				RiskLevel:       RiskCritical,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendManual,
			expectedContains: "manual review",
		},
		{
			name:      "high risk",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume:       true,
				RiskLevel:       RiskHigh,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendRestart,
			expectedContains: "restart recommended",
		},
		{
			name:      "required cleanup",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume: true,
				RiskLevel: RiskMedium,
				RequiredCleanup: []CleanupAction{
					{Type: CleanupWorkspace, Required: true},
				},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendCleanup,
			expectedContains: "cleanup actions",
		},
		{
			name:      "old work",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now().Add(-13 * time.Hour)},
			plan: &ResumptionPlan{
				CanResume:       true,
				RiskLevel:       RiskLow,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendRestart,
			expectedContains: "too old",
		},
		{
			name:      "medium risk acceptable",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume:       true,
				RiskLevel:       RiskMedium,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendResume,
			expectedContains: "Medium risk",
		},
		{
			name:      "low risk resume",
			workState: &state.AgentWorkState{State: state.StateImplement, UpdatedAt: time.Now()},
			plan: &ResumptionPlan{
				CanResume:       true,
				RiskLevel:       RiskLow,
				RequiredCleanup: []CleanupAction{},
			},
			report:           &state.ValidationReport{},
			expectedAction:   RecommendResume,
			expectedContains: "Low risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, reason := recoveryManager.makeResumptionRecommendation(tt.workState, tt.plan, tt.report)
			assert.Equal(t, tt.expectedAction, action)
			assert.Contains(t, reason, tt.expectedContains)
		})
	}
}

// --- canResumeWork additional tests ---

func Test_canResumeWork_EarlyStates(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	report := &state.ValidationReport{
		Valid:       true,
		IssuesFound: []*state.ValidationIssue{},
		StateDrifts: []*state.StateDrift{},
	}

	// Claim state should be resumable
	claimState := &state.AgentWorkState{
		State:     state.StateClaim,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, recoveryManager.canResumeWork(claimState, report))

	// Analyze state should be resumable
	analyzeState := &state.AgentWorkState{
		State:     state.StateAnalyze,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, recoveryManager.canResumeWork(analyzeState, report))

	// Workspace state with dir should be resumable
	workspaceState := &state.AgentWorkState{
		State:        state.StateWorkspace,
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
		WorkspaceDir: "/tmp/test",
	}
	assert.True(t, recoveryManager.canResumeWork(workspaceState, report))

	// Workspace state without dir should NOT be resumable
	workspaceNoDir := &state.AgentWorkState{
		State:     state.StateWorkspace,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	assert.False(t, recoveryManager.canResumeWork(workspaceNoDir, report))
}

// --- performFullCleanup tests ---

func Test_performFullCleanup(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateClaim,
	}

	ctx := context.Background()

	// Mock GitHub cleanup
	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)

	// Mock store cleanup
	mockStore.On("Delete", ctx, "developer").Return(nil)

	err := recoveryManager.performFullCleanup(ctx, workState)
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
	mockGH.AssertExpectations(t)
}

func Test_performFullCleanup_WithWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	wsDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(wsDir, 0755))

	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        state.StateImplement,
		WorkspaceDir: wsDir,
	}

	ctx := context.Background()

	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	mockStore.On("Delete", ctx, "developer").Return(nil)

	err := recoveryManager.performFullCleanup(ctx, workState)
	assert.NoError(t, err)

	// Workspace should be cleaned up
	_, err = os.Stat(wsDir)
	assert.True(t, os.IsNotExist(err))
}

// --- flagForManualIntervention tests ---

func Test_flagForManualIntervention(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	mockGH.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:manual-review"}).Return(nil)

	err := recoveryManager.flagForManualIntervention(ctx, workState)
	assert.NoError(t, err)
	mockGH.AssertExpectations(t)
}

// --- cleanupWorkspace tests ---

func Test_cleanupWorkspace(t *testing.T) {
	logger := slog.Default()
	deps := agent.Dependencies{Logger: logger, Config: &config.Config{}}
	recoveryManager := NewRecoveryManager(deps, nil)

	// Empty path should succeed
	err := recoveryManager.cleanupWorkspace("")
	assert.NoError(t, err)

	// Valid path
	tempDir := t.TempDir()
	wsDir := filepath.Join(tempDir, "ws")
	require.NoError(t, os.MkdirAll(wsDir, 0755))

	err = recoveryManager.cleanupWorkspace(wsDir)
	assert.NoError(t, err)

	_, err = os.Stat(wsDir)
	assert.True(t, os.IsNotExist(err))
}

// --- cleanupCheckpoint tests ---

func Test_cleanupCheckpoint(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	ws := &state.AgentWorkState{
		AgentType:          "developer",
		IssueNumber:        42,
		CheckpointedAt:     time.Now(),
		CheckpointStage:    "implementation",
		CheckpointMetadata: map[string]interface{}{"key": "val"},
		InterruptedBy:      "shutdown",
	}

	ctx := context.Background()
	mockStore.On("Save", ctx, ws).Return(nil)

	err := recoveryManager.cleanupCheckpoint(ctx, ws)
	assert.NoError(t, err)
	assert.True(t, ws.CheckpointedAt.IsZero())
	assert.Empty(t, ws.CheckpointStage)
	assert.Nil(t, ws.CheckpointMetadata)
	assert.Empty(t, ws.InterruptedBy)
}

// --- ValidateWorkspaceConsistency additional tests ---

func TestRecoveryManager_ValidateWorkspaceConsistency_ValidGitRepo(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	// Create a directory with .git
	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0755))

	err := recoveryManager.ValidateWorkspaceConsistency(context.Background(), tempDir, 123)
	assert.NoError(t, err)
}

func TestRecoveryManager_ValidateWorkspaceConsistency_NoGitDir(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockValidator{}

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)

	// Create a directory WITHOUT .git
	tempDir := t.TempDir()

	err := recoveryManager.ValidateWorkspaceConsistency(context.Background(), tempDir, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

// --- CleanupOrphanedWork additional tests ---

func TestRecoveryManager_CleanupOrphanedWork_ManualIntervention(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	mockValidator := &MockValidator{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        state.StateImplement,
		RecoveryType: state.RecoveryTypeManual,
	}

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	mockStore.On("Load", ctx, "developer").Return(workState, nil)
	mockGH.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:manual-review"}).Return(nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err)
	mockGH.AssertExpectations(t)
}

func TestRecoveryManager_CleanupOrphanedWork_NilWorkState(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		RecoveryType: state.RecoveryTypeCleanup,
	}

	mockStore.On("Load", ctx, "developer").Return(nil, nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err)
}

func TestRecoveryManager_CleanupOrphanedWork_ResumeType(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	mockValidator := &MockValidator{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        state.StateImplement,
		RecoveryType: state.RecoveryTypeResume,
	}

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now().Add(-25 * time.Hour), // Old, so resume won't be viable
	}

	mockStore.On("Load", ctx, "developer").Return(workState, nil)

	// Validator returns report that prevents resumption (critical issue)
	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{Severity: state.SeverityCritical, Type: state.IssueTypeInconsistentState},
		},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, workState).Return(validationReport, nil)

	// Since resume isn't viable, it falls back to full cleanup
	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	mockStore.On("Delete", ctx, "developer").Return(nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err)
}

func TestRecoveryManager_CleanupOrphanedWork_ResumeType_Viable(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	mockValidator := &MockValidator{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, mockValidator)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        state.StateImplement,
		RecoveryType: state.RecoveryTypeResume,
	}

	workState := &state.AgentWorkState{
		AgentType:       "developer",
		IssueNumber:     42,
		State:           state.StateImplement,
		UpdatedAt:       time.Now().Add(-1 * time.Hour),
		CheckpointedAt:  time.Now().Add(-1 * time.Hour),
		CheckpointStage: "implementation",
	}

	mockStore.On("Load", ctx, "developer").Return(workState, nil)

	// Validator returns clean report - resume is viable
	validationReport := &state.ValidationReport{
		Valid:              true,
		IssuesFound:        []*state.ValidationIssue{},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, workState).Return(validationReport, nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err) // Should skip cleanup since resume is viable
}

func TestRecoveryManager_CleanupOrphanedWork_LoadError(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		RecoveryType: state.RecoveryTypeCleanup,
	}

	mockStore.On("Load", ctx, "developer").Return(nil, fmt.Errorf("load error"))

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading work state")
}

func Test_performFullCleanup_WithCheckpoint(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:       "developer",
		IssueNumber:     42,
		State:           state.StateImplement,
		CheckpointedAt:  time.Now(),
		CheckpointStage: "implementation",
	}

	ctx := context.Background()

	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	mockStore.On("Save", ctx, workState).Return(nil) // For checkpoint cleanup
	mockStore.On("Delete", ctx, "developer").Return(nil)

	err := recoveryManager.performFullCleanup(ctx, workState)
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func Test_performFullCleanup_DeleteError(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateClaim,
	}

	ctx := context.Background()

	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	mockStore.On("Delete", ctx, "developer").Return(fmt.Errorf("delete error"))

	err := recoveryManager.performFullCleanup(ctx, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup completed with")
}

func Test_cleanupGitHubLabels(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)

	err := recoveryManager.cleanupGitHubLabels(ctx, 42)
	assert.NoError(t, err)
	mockGH.AssertExpectations(t)
}

func Test_cleanupGitHubLabels_AddError(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(fmt.Errorf("add label error"))

	err := recoveryManager.cleanupGitHubLabels(ctx, 42)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding ready label")
}

func Test_cleanupGitHubLabels_RemoveLabelError(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	// RemoveLabel fails but should continue
	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(fmt.Errorf("remove label error"))
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)

	err := recoveryManager.cleanupGitHubLabels(ctx, 42)
	assert.NoError(t, err) // Should succeed despite RemoveLabel error
	mockGH.AssertExpectations(t)
}

func Test_cleanupWorkspace_RemoveAllError(t *testing.T) {
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	// Use a directory path that would fail RemoveAll (read-only parent)
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(wsDir, 0o755))

	// Make it read-only so RemoveAll fails
	require.NoError(t, os.Chmod(tmpDir, 0o444))
	defer os.Chmod(tmpDir, 0o755) // restore for cleanup

	err := recoveryManager.cleanupWorkspace(wsDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "removing workspace directory")
}

func Test_assessResumptionRisk_MediumAge(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	// Test specifically the 6-12 hour age band
	ws := &state.AgentWorkState{
		State:     state.StateImplement,
		UpdatedAt: time.Now().Add(-8 * time.Hour),
	}
	report := &state.ValidationReport{
		Valid:       true,
		IssuesFound: []*state.ValidationIssue{},
		StateDrifts: []*state.StateDrift{},
	}

	risk := recoveryManager.assessResumptionRisk(ws, report)
	assert.Equal(t, RiskLow, risk) // Score is 1 (6-12h age), still low
}

func TestRecoveryManager_CleanupOrphanedWork_UnknownRecoveryType(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        state.StateImplement,
		RecoveryType: "unknown_recovery_type",
	}

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
	}

	mockStore.On("Load", ctx, "developer").Return(workState, nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err) // Falls through switch, returns nil
}

func TestRecoveryManager_CleanupOrphanedWork_MismatchedIssue(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)
	ctx := context.Background()

	orphanedItem := &state.OrphanedWorkItem{
		AgentType:    "developer",
		IssueNumber:  42,
		RecoveryType: state.RecoveryTypeCleanup,
	}

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 99, // Different from orphaned item
	}

	mockStore.On("Load", ctx, "developer").Return(workState, nil)

	err := recoveryManager.CleanupOrphanedWork(ctx, orphanedItem)
	assert.NoError(t, err) // Should return nil due to mismatch
}

func Test_flagForManualIntervention_CommentError(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()
	mockGH.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(fmt.Errorf("comment error"))

	err := recoveryManager.flagForManualIntervention(ctx, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating manual intervention comment")
}

func Test_flagForManualIntervention_LabelError(t *testing.T) {
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       state.StateImplement,
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()
	mockGH.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:manual-review"}).Return(fmt.Errorf("label error"))

	err := recoveryManager.flagForManualIntervention(ctx, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding manual review label")
}

func Test_cleanupCheckpoint_SaveError(t *testing.T) {
	mockStore := &MockStore{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	ws := &state.AgentWorkState{
		AgentType:       "developer",
		IssueNumber:     42,
		CheckpointedAt:  time.Now(),
		CheckpointStage: "implementation",
	}

	ctx := context.Background()
	mockStore.On("Save", ctx, ws).Return(fmt.Errorf("save error"))

	err := recoveryManager.cleanupCheckpoint(ctx, ws)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "saving state after checkpoint cleanup")
}

func Test_cleanupWorkspace_NonExistent(t *testing.T) {
	logger := slog.Default()
	deps := agent.Dependencies{Logger: logger, Config: &config.Config{}}
	recoveryManager := NewRecoveryManager(deps, nil)

	// Non-existent path should succeed (RemoveAll on non-existent path succeeds)
	err := recoveryManager.cleanupWorkspace("/nonexistent/path/that/doesnt/exist")
	assert.NoError(t, err)
}

func Test_performFullCleanup_AllErrors(t *testing.T) {
	mockStore := &MockStore{}
	mockGH := &MockGitHub{}
	logger := slog.Default()

	deps := agent.Dependencies{
		Logger: logger,
		Store:  mockStore,
		GitHub: mockGH,
		Config: &config.Config{},
	}

	recoveryManager := NewRecoveryManager(deps, nil)

	workState := &state.AgentWorkState{
		AgentType:       "developer",
		IssueNumber:     42,
		State:           state.StateImplement,
		WorkspaceDir:    "/nonexistent/workspace",
		CheckpointedAt:  time.Now(),
		CheckpointStage: "implementation",
	}

	ctx := context.Background()

	// All operations should succeed but checkpoint save fails
	mockGH.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	mockGH.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(fmt.Errorf("label error"))
	mockStore.On("Save", ctx, workState).Return(fmt.Errorf("save error"))
	mockStore.On("Delete", ctx, "developer").Return(fmt.Errorf("delete error"))

	err := recoveryManager.performFullCleanup(ctx, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup completed with")
}

func Test_canResumeWork_FailedState(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	report := &state.ValidationReport{
		Valid:       true,
		IssuesFound: []*state.ValidationIssue{},
		StateDrifts: []*state.StateDrift{},
	}

	// Failed state should not be resumable
	failedState := &state.AgentWorkState{
		State:     state.StateFailed,
		UpdatedAt: time.Now(),
	}
	assert.False(t, recoveryManager.canResumeWork(failedState, report))
}

func Test_assessResumptionRisk_HighSeverityOnly(t *testing.T) {
	recoveryManager := &RecoveryManager{
		logger: slog.Default(),
	}

	ws := &state.AgentWorkState{
		State:     state.StateReview, // Advanced state adds 2
		UpdatedAt: time.Now(),
	}
	report := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{Severity: state.SeverityHigh},   // +2
			{Severity: state.SeverityMedium}, // +1
		},
		StateDrifts: []*state.StateDrift{},
	}

	risk := recoveryManager.assessResumptionRisk(ws, report)
	assert.Equal(t, RiskHigh, risk) // Score = 2 (review) + 2 (high) + 1 (medium) = 5
}

func TestRecoveryManager_AttemptResume_ValidationError(t *testing.T) {
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
		UpdatedAt:   time.Now(),
	}

	mockValidator.On("ValidateWorkState", ctx, workState).Return(nil, fmt.Errorf("validation error"))

	plan, err := recoveryManager.AttemptResume(ctx, workState)
	assert.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "validating work state")
}
