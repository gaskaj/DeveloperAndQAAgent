package state

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
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