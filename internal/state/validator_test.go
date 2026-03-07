package state

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockGitHubClient mocks the GitHub client for testing
type MockGitHubClient struct {
	mock.Mock
}

// Implement all required methods from ghub.Client interface
func (m *MockGitHubClient) ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error) {
	args := m.Called(ctx, labels)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*github.Issue), args.Error(1)
}

func (m *MockGitHubClient) ListIssuesByState(ctx context.Context, labels []string, state string) ([]*github.Issue, error) {
	args := m.Called(ctx, labels, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*github.Issue), args.Error(1)
}

func (m *MockGitHubClient) GetIssue(ctx context.Context, number int) (*github.Issue, error) {
	args := m.Called(ctx, number)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*github.Issue), args.Error(1)
}

func (m *MockGitHubClient) AssignIssue(ctx context.Context, number int, assignees []string) error {
	args := m.Called(ctx, number, assignees)
	return args.Error(0)
}

func (m *MockGitHubClient) AssignSelfIfNoAssignees(ctx context.Context, number int) error {
	args := m.Called(ctx, number)
	return args.Error(0)
}

func (m *MockGitHubClient) AddLabels(ctx context.Context, number int, labels []string) error {
	args := m.Called(ctx, number, labels)
	return args.Error(0)
}

func (m *MockGitHubClient) RemoveLabel(ctx context.Context, number int, label string) error {
	args := m.Called(ctx, number, label)
	return args.Error(0)
}

func (m *MockGitHubClient) CreateBranch(ctx context.Context, name string, fromRef string) error {
	args := m.Called(ctx, name, fromRef)
	return args.Error(0)
}

func (m *MockGitHubClient) CreatePR(ctx context.Context, opts ghub.PROptions) (*github.PullRequest, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*github.PullRequest), args.Error(1)
}

func (m *MockGitHubClient) ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*github.PullRequest), args.Error(1)
}

func (m *MockGitHubClient) GetPR(ctx context.Context, number int) (*github.PullRequest, error) {
	args := m.Called(ctx, number)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*github.PullRequest), args.Error(1)
}

func (m *MockGitHubClient) ValidatePR(ctx context.Context, prNumber int, opts ghub.PRValidationOptions) (*ghub.PRValidationResult, error) {
	args := m.Called(ctx, prNumber, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ghub.PRValidationResult), args.Error(1)
}

func (m *MockGitHubClient) GetPRCheckStatus(ctx context.Context, prNumber int) (*ghub.PRValidationResult, error) {
	args := m.Called(ctx, prNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ghub.PRValidationResult), args.Error(1)
}

func (m *MockGitHubClient) CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	args := m.Called(ctx, title, body, labels)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*github.Issue), args.Error(1)
}

func (m *MockGitHubClient) CreateComment(ctx context.Context, number int, body string) error {
	args := m.Called(ctx, number, body)
	return args.Error(0)
}

func (m *MockGitHubClient) ListComments(ctx context.Context, number int) ([]*github.IssueComment, error) {
	args := m.Called(ctx, number)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*github.IssueComment), args.Error(1)
}

// MockStore mocks the state store for testing
type MockStore struct {
	mock.Mock
}

func (m *MockStore) Save(ctx context.Context, state *AgentWorkState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func (m *MockStore) Load(ctx context.Context, agentType string) (*AgentWorkState, error) {
	args := m.Called(ctx, agentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*AgentWorkState), args.Error(1)
}

func (m *MockStore) Delete(ctx context.Context, agentType string) error {
	args := m.Called(ctx, agentType)
	return args.Error(0)
}

func (m *MockStore) List(ctx context.Context) ([]*AgentWorkState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*AgentWorkState), args.Error(1)
}

func TestStateValidator_ValidateWorkState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateAnalyze,
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
	}

	// Mock successful issue fetch
	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.Valid) // Should be valid with no issues
	assert.Empty(t, report.IssuesFound)
	assert.WithinDuration(t, time.Now(), report.ValidatedAt, 1*time.Second)

	mockGithub.AssertExpectations(t)
}

func TestStateValidator_DetectOrphanedWork(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()

	// Create test states - one recent, one old
	recentState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		UpdatedAt:   time.Now().Add(-30 * time.Minute), // 30 minutes ago
	}

	orphanedState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 456,
		State:       StateImplement,
		UpdatedAt:   time.Now().Add(-2 * time.Hour), // 2 hours ago
	}

	mockStore.On("List", ctx).Return([]*AgentWorkState{recentState, orphanedState}, nil)

	orphanedItems, err := validator.DetectOrphanedWork(ctx)

	assert.NoError(t, err)
	assert.Len(t, orphanedItems, 1)
	assert.Equal(t, 456, orphanedItems[0].IssueNumber)
	assert.Equal(t, "developer", orphanedItems[0].AgentType)
	assert.True(t, orphanedItems[0].AgeHours > 1.0)

	mockStore.AssertExpectations(t)
}

func TestStateValidator_isOrphanedWork(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	tests := []struct {
		name     string
		state    *AgentWorkState
		expected bool
	}{
		{
			name: "recent work is not orphaned",
			state: &AgentWorkState{
				State:     StateImplement,
				UpdatedAt: time.Now().Add(-30 * time.Minute),
			},
			expected: false,
		},
		{
			name: "old work is orphaned",
			state: &AgentWorkState{
				State:     StateImplement,
				UpdatedAt: time.Now().Add(-2 * time.Hour),
			},
			expected: true,
		},
		{
			name: "terminal state is not orphaned",
			state: &AgentWorkState{
				State:     StateComplete,
				UpdatedAt: time.Now().Add(-5 * time.Hour),
			},
			expected: false,
		},
		{
			name: "error state with recent age",
			state: &AgentWorkState{
				State:     StateImplement,
				Error:     "some error",
				UpdatedAt: time.Now().Add(-15 * time.Minute),
			},
			expected: false,
		},
		{
			name: "error state with old age",
			state: &AgentWorkState{
				State:     StateImplement,
				Error:     "some error",
				UpdatedAt: time.Now().Add(-45 * time.Minute),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.isOrphanedWork(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStateValidator_determineRecoveryType(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	tests := []struct {
		name     string
		state    *AgentWorkState
		expected OrphanRecoveryType
	}{
		{
			name: "early state should cleanup",
			state: &AgentWorkState{
				State: StateClaim,
			},
			expected: RecoveryTypeCleanup,
		},
		{
			name: "state with PR should be manual",
			state: &AgentWorkState{
				State:    StateValidation,
				PRNumber: 123,
			},
			expected: RecoveryTypeManual,
		},
		{
			name: "recent checkpoint should resume",
			state: &AgentWorkState{
				State:          StateImplement,
				CheckpointedAt: time.Now().Add(-30 * time.Minute),
			},
			expected: RecoveryTypeResume,
		},
		{
			name: "old checkpoint defaults to resume",
			state: &AgentWorkState{
				State:          StateImplement,
				CheckpointedAt: time.Now().Add(-5 * time.Hour),
			},
			expected: RecoveryTypeResume,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.determineRecoveryType(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStateValidator_ReconcileState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		UpdatedAt:   time.Now(),
	}

	// Mock issue fetch that shows valid state
	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	err := validator.ReconcileState(ctx, workState)

	assert.NoError(t, err)
	mockGithub.AssertExpectations(t)
}

func TestStateValidator_ReconcileState_WithIssues(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	// Use a non-standard branch to trigger validation issue, and missing claimed label for drift
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		BranchName:  "wrong-branch",
		UpdatedAt:   time.Now(),
	}

	// Issue is open but missing claimed label -> drift detected (generates reconcile action)
	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:ready")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	// Reconcile action will try to add claimed label and remove ready label
	mockGithub.On("AddLabels", ctx, 123, []string{"agent:claimed"}).Return(nil)
	mockGithub.On("RemoveLabel", ctx, 123, "agent:ready").Return(nil)

	err := validator.ReconcileState(ctx, workState)

	assert.NoError(t, err)
	mockGithub.AssertExpectations(t)
}

func TestStateValidator_ReconcileState_WithOrphanedWork(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	// Create a work state that will produce an urgent manual_fix action
	// by having orphaned work with a PR (manual recovery type)
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		PRNumber:    50,
		UpdatedAt:   time.Now().Add(-3 * time.Hour), // Old enough to be orphaned
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	// PR check during validation
	pr := &github.PullRequest{
		State: github.String("open"),
		Body:  github.String("Closes #123"),
	}
	mockGithub.On("GetPR", ctx, 50).Return(pr, nil)

	err := validator.ReconcileState(ctx, workState)
	// No error - manual_fix actions are not auto-executed by executeReconciliationAction
	assert.NoError(t, err)
}

func TestStateValidator_ValidateWorkState_ClosedIssue(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		UpdatedAt:   time.Now(),
	}

	// Issue is closed but state says implement
	issue := &github.Issue{
		State: github.String("closed"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	// Should detect state drift: issue closed but agent active
	assert.NotEmpty(t, report.StateDrifts)
	assert.Equal(t, DriftTypeIssueState, report.StateDrifts[0].Type)
}

func TestStateValidator_ValidateWorkState_MissingClaimedLabel(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		UpdatedAt:   time.Now(),
	}

	// Open issue but no agent:claimed label
	issue := &github.Issue{
		State:  github.String("open"),
		Labels: []*github.Label{},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.NotEmpty(t, report.StateDrifts)
}

func TestStateValidator_ValidateWorkState_FetchError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		UpdatedAt:   time.Now(),
	}

	mockGithub.On("GetIssue", ctx, 123).Return(nil, assert.AnError)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	// Should report inconsistent state issue
	assert.NotEmpty(t, report.IssuesFound)
	assert.Equal(t, IssueTypeInconsistentState, report.IssuesFound[0].Type)
	assert.Equal(t, SeverityCritical, report.IssuesFound[0].Severity)
}

func TestStateValidator_ValidateWorkState_NoIssueNumber(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType: "developer",
		State:     StateIdle,
		UpdatedAt: time.Now(),
	}

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.Valid)
	// No GitHub calls should be made
	mockGithub.AssertNotCalled(t, "GetIssue")
}

func TestStateValidator_ValidateWorkState_WithBranch(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		BranchName:  "agent/issue-123",
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.True(t, report.Valid)
}

func TestStateValidator_ValidateWorkState_WithWrongBranch(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
		BranchName:  "feature/wrong-branch",
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.IssuesFound)
	assert.Equal(t, IssueTypeBranchDrift, report.IssuesFound[0].Type)
}

func TestStateValidator_ValidateWorkState_WithPR(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	body := "This PR implements changes. Closes #123"
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateReview,
		PRNumber:    50,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	pr := &github.PullRequest{
		State: github.String("open"),
		Body:  github.String(body),
	}
	mockGithub.On("GetPR", ctx, 50).Return(pr, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.True(t, report.Valid)
}

func TestStateValidator_ValidateWorkState_PRClosed(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateReview,
		PRNumber:    50,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	pr := &github.PullRequest{
		State: github.String("closed"),
		Body:  github.String("Closes #123"),
	}
	mockGithub.On("GetPR", ctx, 50).Return(pr, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotEmpty(t, report.StateDrifts)

	// Find the PR drift
	var prDriftFound bool
	for _, drift := range report.StateDrifts {
		if drift.Type == DriftTypePRState {
			prDriftFound = true
			assert.Equal(t, "closed", drift.ExternalState)
		}
	}
	assert.True(t, prDriftFound)
}

func TestStateValidator_ValidateWorkState_PRFetchError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateReview,
		PRNumber:    50,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)
	mockGithub.On("GetPR", ctx, 50).Return(nil, assert.AnError)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotEmpty(t, report.IssuesFound)
	assert.Equal(t, IssueTypePRInconsistency, report.IssuesFound[0].Type)
}

func TestStateValidator_ValidateWorkState_PRMissingIssueRef(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateReview,
		PRNumber:    50,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	// PR body does not reference issue #123
	pr := &github.PullRequest{
		State: github.String("open"),
		Body:  github.String("This PR fixes something unrelated."),
	}
	mockGithub.On("GetPR", ctx, 50).Return(pr, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.IssuesFound)

	// Find PR inconsistency issue
	var found bool
	for _, vi := range report.IssuesFound {
		if vi.Type == IssueTypePRInconsistency {
			found = true
			break
		}
	}
	assert.True(t, found, "expected PR inconsistency issue for missing issue reference")
}

func TestStateValidator_ValidateWorkState_WithWorkspace(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	workState := &AgentWorkState{
		AgentType:    "developer",
		IssueNumber:  123,
		State:        StateImplement,
		WorkspaceDir: "/tmp/workspace-123",
		UpdatedAt:    time.Now(),
	}

	issue := &github.Issue{
		State: github.String("open"),
		Labels: []*github.Label{
			{Name: github.String("agent:claimed")},
		},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	assert.NotNil(t, report)
}

func TestStateValidator_ValidateIssueStateConsistency_IdleState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateIdle,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State:  github.String("open"),
		Labels: []*github.Label{},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	// Idle state does not require claimed label
	assert.True(t, report.Valid)
}

func TestStateValidator_ValidateIssueStateConsistency_CompleteState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateComplete,
		UpdatedAt:   time.Now(),
	}

	issue := &github.Issue{
		State:  github.String("closed"),
		Labels: []*github.Label{},
	}
	mockGithub.On("GetIssue", ctx, 123).Return(issue, nil)

	report, err := validator.ValidateWorkState(ctx, workState)

	assert.NoError(t, err)
	// Closed issue + Complete state is consistent
	assert.True(t, report.Valid)
}

func TestStateValidator_DetectOrphanedWork_StoreError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()
	mockStore.On("List", ctx).Return(nil, assert.AnError)

	orphaned, err := validator.DetectOrphanedWork(ctx)

	assert.Error(t, err)
	assert.Nil(t, orphaned)
}

func TestStateValidator_DetectOrphanedWork_NoOrphans(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()

	// All states are recent or terminal
	states := []*AgentWorkState{
		{AgentType: "dev1", State: StateImplement, UpdatedAt: time.Now()},
		{AgentType: "dev2", State: StateComplete, UpdatedAt: time.Now().Add(-5 * time.Hour)},
		{AgentType: "dev3", State: StateIdle, UpdatedAt: time.Now().Add(-10 * time.Hour)},
	}
	mockStore.On("List", ctx).Return(states, nil)

	orphaned, err := validator.DetectOrphanedWork(ctx)

	assert.NoError(t, err)
	assert.Empty(t, orphaned)
}

func TestStateValidator_DetectOrphanedWork_WithDetails(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()

	states := []*AgentWorkState{
		{
			AgentType:       "developer",
			IssueNumber:     100,
			State:           StateImplement,
			UpdatedAt:       time.Now().Add(-3 * time.Hour),
			Error:           "timeout error",
			CheckpointStage: "file_editing",
			BranchName:      "agent/issue-100",
			WorkspaceDir:    "/tmp/ws-100",
		},
	}
	mockStore.On("List", ctx).Return(states, nil)

	orphaned, err := validator.DetectOrphanedWork(ctx)

	assert.NoError(t, err)
	require.Len(t, orphaned, 1)
	assert.Equal(t, "developer", orphaned[0].AgentType)
	assert.Equal(t, 100, orphaned[0].IssueNumber)
	assert.Equal(t, "timeout error", orphaned[0].Details["last_error"])
	assert.Equal(t, "file_editing", orphaned[0].Details["checkpoint_stage"])
	assert.Equal(t, "agent/issue-100", orphaned[0].BranchName)
	assert.Equal(t, "/tmp/ws-100", orphaned[0].WorkspaceDir)
}

func TestStateValidator_DetectOrphanedWork_SortedByAge(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)

	ctx := context.Background()

	// Newer orphan first in input, older second
	states := []*AgentWorkState{
		{AgentType: "dev1", IssueNumber: 1, State: StateImplement, UpdatedAt: time.Now().Add(-2 * time.Hour)},
		{AgentType: "dev2", IssueNumber: 2, State: StateImplement, UpdatedAt: time.Now().Add(-5 * time.Hour)},
	}
	mockStore.On("List", ctx).Return(states, nil)

	orphaned, err := validator.DetectOrphanedWork(ctx)

	assert.NoError(t, err)
	require.Len(t, orphaned, 2)
	// Oldest should be first
	assert.Equal(t, 2, orphaned[0].IssueNumber)
	assert.Equal(t, 1, orphaned[1].IssueNumber)
}

func TestStateValidator_IsOrphanedWork_FailedState(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State:     StateFailed,
		UpdatedAt: time.Now().Add(-5 * time.Hour),
	}
	assert.False(t, validator.isOrphanedWork(state))
}

func TestStateValidator_IsOrphanedWork_IdleState(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State:     StateIdle,
		UpdatedAt: time.Now().Add(-5 * time.Hour),
	}
	assert.False(t, validator.isOrphanedWork(state))
}

func TestStateValidator_IsOrphanedWork_ClaimStuckBriefly(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State:     StateClaim,
		UpdatedAt: time.Now().Add(-20 * time.Minute), // less than 30 min
	}
	assert.False(t, validator.isOrphanedWork(state))
}

func TestStateValidator_IsOrphanedWork_ClaimStuckLong(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State:     StateClaim,
		UpdatedAt: time.Now().Add(-35 * time.Minute), // more than 30 min
	}
	assert.True(t, validator.isOrphanedWork(state))
}

func TestStateValidator_IsOrphanedWork_AnalyzeStuckLong(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State:     StateAnalyze,
		UpdatedAt: time.Now().Add(-35 * time.Minute),
	}
	assert.True(t, validator.isOrphanedWork(state))
}

func TestStateValidator_DetermineRecoveryType_AnalyzeState(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State: StateAnalyze,
	}
	assert.Equal(t, RecoveryTypeCleanup, validator.determineRecoveryType(state))
}

func TestStateValidator_DetermineRecoveryType_WorkspaceState(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State: StateWorkspace,
	}
	assert.Equal(t, RecoveryTypeCleanup, validator.determineRecoveryType(state))
}

func TestStateValidator_DetermineRecoveryType_ImplementWithoutCheckpoint(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	state := &AgentWorkState{
		State: StateImplement,
		// No checkpoint, no PR
	}
	assert.Equal(t, RecoveryTypeResume, validator.determineRecoveryType(state))
}

func TestStateValidator_GenerateRecommendedActions_AllTypes(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	report := &ValidationReport{
		OrphanedWork: []*OrphanedWorkItem{
			{IssueNumber: 1, AgentType: "dev", RecoveryType: RecoveryTypeCleanup},
			{IssueNumber: 2, AgentType: "dev", RecoveryType: RecoveryTypeResume},
			{IssueNumber: 3, AgentType: "dev", RecoveryType: RecoveryTypeManual},
		},
		StateDrifts: []*StateDrift{
			{Type: DriftTypeIssueState, AgentType: "dev", IssueNumber: 4, CanReconcile: true},
			{Type: DriftTypePRState, AgentType: "dev", IssueNumber: 5, CanReconcile: false},
		},
		RecommendedActions: make([]*RecommendedAction, 0),
	}

	validator.generateRecommendedActions(report)

	// 3 orphan actions + 1 reconcilable drift = 4
	assert.Len(t, report.RecommendedActions, 4)

	// Check cleanup action
	assert.Equal(t, ActionTypeCleanup, report.RecommendedActions[0].Type)
	assert.Equal(t, PriorityHigh, report.RecommendedActions[0].Priority)
	assert.Equal(t, 1, report.RecommendedActions[0].IssueNumber)

	// Check resume action
	assert.Equal(t, ActionTypeResume, report.RecommendedActions[1].Type)
	assert.Equal(t, PriorityMedium, report.RecommendedActions[1].Priority)
	assert.Equal(t, 2, report.RecommendedActions[1].IssueNumber)

	// Check manual fix action
	assert.Equal(t, ActionTypeManualFix, report.RecommendedActions[2].Type)
	assert.Equal(t, PriorityUrgent, report.RecommendedActions[2].Priority)
	assert.Equal(t, 3, report.RecommendedActions[2].IssueNumber)

	// Check reconcile action (only for reconcilable drift)
	assert.Equal(t, ActionTypeReconcile, report.RecommendedActions[3].Type)
	assert.Equal(t, PriorityHigh, report.RecommendedActions[3].Priority)
	assert.Equal(t, 4, report.RecommendedActions[3].IssueNumber)
}

func TestStateValidator_ExecuteReconciliationAction_Reconcile(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReconcile,
		AgentType:   "developer",
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
	}

	mockGithub.On("AddLabels", ctx, 123, []string{"agent:claimed"}).Return(nil)
	mockGithub.On("RemoveLabel", ctx, 123, "agent:ready").Return(nil)

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
	mockGithub.AssertExpectations(t)
}

func TestStateValidator_ExecuteReconciliationAction_Reset(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReset,
		AgentType:   "developer",
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
	}

	mockStore.On("Save", ctx, mock.Anything).Return(nil)
	mockGithub.On("RemoveLabel", ctx, 123, "agent:claimed").Return(nil)
	mockGithub.On("AddLabels", ctx, 123, []string{"agent:ready"}).Return(nil)

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)

	// Verify state was reset
	assert.Equal(t, StateIdle, workState.State)
	mockStore.AssertExpectations(t)
	mockGithub.AssertExpectations(t)
}

func TestStateValidator_ExecuteReconciliationAction_Cleanup(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type: ActionTypeCleanup,
	}
	workState := &AgentWorkState{}

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
}

func TestStateValidator_ExecuteReconciliationAction_UnsupportedType(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type: ActionType("unknown_action"),
	}
	workState := &AgentWorkState{}

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
}

func TestStateValidator_ExecuteResetAction_SaveError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReset,
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
	}

	mockStore.On("Save", ctx, mock.Anything).Return(assert.AnError)

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "saving reset state")
}

func TestStateValidator_ExecuteResetAction_AddLabelError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReset,
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
	}

	mockStore.On("Save", ctx, mock.Anything).Return(nil)
	mockGithub.On("RemoveLabel", ctx, 123, "agent:claimed").Return(nil)
	mockGithub.On("AddLabels", ctx, 123, []string{"agent:ready"}).Return(assert.AnError)

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding ready label")
}

func TestStateValidator_ExecuteReconcileAction_IdleState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReconcile,
		IssueNumber: 123,
	}
	// Idle state => no label changes
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateIdle,
	}

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
	mockGithub.AssertNotCalled(t, "AddLabels")
}

func TestStateValidator_ExecuteReconcileAction_CompleteState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReconcile,
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateComplete,
	}

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
	mockGithub.AssertNotCalled(t, "AddLabels")
}

func TestStateValidator_ExecuteReconcileAction_FailedState(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReconcile,
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateFailed,
	}

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.NoError(t, err)
	mockGithub.AssertNotCalled(t, "AddLabels")
}

func TestStateValidator_ExecuteReconcileAction_AddLabelError(t *testing.T) {
	logger := slog.Default()
	mockStore := &MockStore{}
	mockGithub := &MockGitHubClient{}

	validator := NewStateValidator(mockStore, mockGithub, logger)
	ctx := context.Background()

	action := &RecommendedAction{
		Type:        ActionTypeReconcile,
		IssueNumber: 123,
	}
	workState := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       StateImplement,
	}

	mockGithub.On("AddLabels", ctx, 123, []string{"agent:claimed"}).Return(assert.AnError)

	err := validator.executeReconciliationAction(ctx, action, workState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding claimed label")
}

func TestContains(t *testing.T) {
	assert.True(t, contains("hello world", "world"))
	assert.True(t, contains("Closes #123", "Closes #123"))
	assert.False(t, contains("hello world", "missing"))
	assert.True(t, contains("abc", ""))
	assert.False(t, contains("", "abc"))
}

func TestStateValidator_AddValidationIssue(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	report := &ValidationReport{
		IssuesFound: make([]*ValidationIssue, 0),
	}

	validator.addValidationIssue(report, IssueTypeOrphanedClaim, SeverityHigh,
		"Test issue", "developer", 42,
		map[string]string{"key": "value"})

	require.Len(t, report.IssuesFound, 1)
	assert.Equal(t, IssueTypeOrphanedClaim, report.IssuesFound[0].Type)
	assert.Equal(t, SeverityHigh, report.IssuesFound[0].Severity)
	assert.Equal(t, "Test issue", report.IssuesFound[0].Description)
	assert.Equal(t, "developer", report.IssuesFound[0].AgentType)
	assert.Equal(t, 42, report.IssuesFound[0].IssueNumber)
	assert.Equal(t, "value", report.IssuesFound[0].Details["key"])
}

func TestStateValidator_AddStateDrift(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	report := &ValidationReport{
		StateDrifts: make([]*StateDrift, 0),
	}

	validator.addStateDrift(report, DriftTypeBranchState, "developer", 42,
		StateImplement, "missing", "Branch not found", false)

	require.Len(t, report.StateDrifts, 1)
	assert.Equal(t, DriftTypeBranchState, report.StateDrifts[0].Type)
	assert.Equal(t, "developer", report.StateDrifts[0].AgentType)
	assert.Equal(t, 42, report.StateDrifts[0].IssueNumber)
	assert.Equal(t, StateImplement, report.StateDrifts[0].LocalState)
	assert.Equal(t, "missing", report.StateDrifts[0].ExternalState)
	assert.Equal(t, "Branch not found", report.StateDrifts[0].Description)
	assert.False(t, report.StateDrifts[0].CanReconcile)
}

func TestStateValidator_CheckOrphanedWork_NotOrphaned(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	report := &ValidationReport{
		OrphanedWork: make([]*OrphanedWorkItem, 0),
	}

	workState := &AgentWorkState{
		State:     StateImplement,
		UpdatedAt: time.Now(),
	}

	err := validator.checkOrphanedWork(context.Background(), workState, report)
	assert.NoError(t, err)
	assert.Empty(t, report.OrphanedWork)
}

func TestStateValidator_CheckOrphanedWork_Orphaned(t *testing.T) {
	logger := slog.Default()
	validator := &StateValidator{logger: logger}

	report := &ValidationReport{
		OrphanedWork: make([]*OrphanedWorkItem, 0),
	}

	workState := &AgentWorkState{
		AgentType:    "developer",
		IssueNumber:  42,
		State:        StateImplement,
		UpdatedAt:    time.Now().Add(-3 * time.Hour),
		BranchName:   "agent/issue-42",
		WorkspaceDir: "/tmp/ws",
		PRNumber:     10,
		Error:        "some error",
	}

	err := validator.checkOrphanedWork(context.Background(), workState, report)
	assert.NoError(t, err)
	require.Len(t, report.OrphanedWork, 1)
	assert.Equal(t, "developer", report.OrphanedWork[0].AgentType)
	assert.Equal(t, 42, report.OrphanedWork[0].IssueNumber)
	assert.Equal(t, "some error", report.OrphanedWork[0].Details["last_error"])
}
