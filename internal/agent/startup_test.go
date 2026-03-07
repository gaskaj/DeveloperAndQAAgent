package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
)

// MockStateValidator mocks the state validator for testing
type MockStateValidator struct {
	mock.Mock
}

func (m *MockStateValidator) ValidateWorkState(ctx context.Context, workState *state.AgentWorkState) (*state.ValidationReport, error) {
	args := m.Called(ctx, workState)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*state.ValidationReport), args.Error(1)
}

func (m *MockStateValidator) DetectOrphanedWork(ctx context.Context) ([]*state.OrphanedWorkItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*state.OrphanedWorkItem), args.Error(1)
}

func (m *MockStateValidator) ReconcileState(ctx context.Context, workState *state.AgentWorkState) error {
	args := m.Called(ctx, workState)
	return args.Error(0)
}

// MockStore mocks the state store for testing
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

func TestStartupValidator_ValidateAndRecoverStartup(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						Enabled:             true,
						AutoCleanupOrphaned: false, // Don't auto-cleanup in this test
					},
				},
			},
		},
	}

	startupValidator := NewStartupValidator(deps, mockValidator)

	ctx := context.Background()

	// Mock no existing state
	mockStore.On("Load", ctx, "developer").Return(nil, nil)

	// Mock orphaned work detection
	orphanedWork := []*state.OrphanedWorkItem{
		{
			AgentType:    "developer",
			IssueNumber:  123,
			State:        state.StateImplement,
			AgeHours:     2.5,
			RecoveryType: state.RecoveryTypeCleanup,
		},
	}
	mockValidator.On("DetectOrphanedWork", ctx).Return(orphanedWork, nil)

	report, err := startupValidator.ValidateAndRecoverStartup(ctx, TypeDeveloper)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.Valid)      // Should be invalid due to orphaned work
	assert.True(t, report.StartupSafe) // But still safe to start
	assert.Len(t, report.OrphanedWorkFound, 1)
	assert.Equal(t, 123, report.OrphanedWorkFound[0].IssueNumber)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecoverStartup_WithExistingState(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						Enabled:             true,
						AutoCleanupOrphaned: false,
					},
				},
			},
		},
	}

	startupValidator := NewStartupValidator(deps, mockValidator)

	ctx := context.Background()

	// Mock existing state
	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 456,
		State:       state.StateImplement,
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
	}
	mockStore.On("Load", ctx, "developer").Return(existingState, nil)

	// Mock state validation
	validationReport := &state.ValidationReport{
		Valid:              true,
		IssuesFound:        []*state.ValidationIssue{},
		OrphanedWork:       []*state.OrphanedWorkItem{},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, existingState).Return(validationReport, nil)

	// Mock no additional orphaned work
	mockValidator.On("DetectOrphanedWork", ctx).Return([]*state.OrphanedWorkItem{}, nil)

	report, err := startupValidator.ValidateAndRecoverStartup(ctx, TypeDeveloper)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.True(t, report.Valid)
	assert.True(t, report.StartupSafe)
	assert.Empty(t, report.OrphanedWorkFound)
	assert.Empty(t, report.ValidationIssues)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecoverStartup_WithValidationIssues(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						Enabled:             true,
						AutoCleanupOrphaned: false,
					},
				},
			},
		},
	}

	startupValidator := NewStartupValidator(deps, mockValidator)

	ctx := context.Background()

	// Mock existing state
	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 456,
		State:       state.StateImplement,
		UpdatedAt:   time.Now().Add(-30 * time.Minute),
	}
	mockStore.On("Load", ctx, "developer").Return(existingState, nil)

	// Mock state validation with issues
	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{
				Type:        state.IssueTypeInconsistentState,
				Severity:    state.SeverityHigh,
				Description: "Issue state inconsistent with GitHub",
			},
		},
		OrphanedWork: []*state.OrphanedWorkItem{},
		StateDrifts: []*state.StateDrift{
			{
				Type:          state.DriftTypeIssueState,
				LocalState:    state.StateImplement,
				ExternalState: "closed",
				CanReconcile:  true,
			},
		},
		RecommendedActions: []*state.RecommendedAction{
			{
				Type:     state.ActionTypeReconcile,
				Priority: state.PriorityHigh,
			},
		},
	}
	mockValidator.On("ValidateWorkState", ctx, existingState).Return(validationReport, nil)

	// Mock no additional orphaned work
	mockValidator.On("DetectOrphanedWork", ctx).Return([]*state.OrphanedWorkItem{}, nil)

	report, err := startupValidator.ValidateAndRecoverStartup(ctx, TypeDeveloper)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.False(t, report.StartupSafe) // Not safe due to validation issues
	assert.Len(t, report.ValidationIssues, 1)
	assert.Len(t, report.RecommendedActions, 1)
	assert.Equal(t, state.SeverityHigh, report.ValidationIssues[0].Severity)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_PerformPeriodicValidation(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						Enabled: true,
						Consistency: config.ConsistencyConfig{
							ReconcileDrift: false, // Don't auto-reconcile in this test
						},
					},
				},
			},
		},
	}

	startupValidator := NewStartupValidator(deps, mockValidator)

	ctx := context.Background()

	// Mock agent states
	states := []*state.AgentWorkState{
		{
			AgentType:   "developer",
			IssueNumber: 123,
			State:       state.StateImplement,
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		},
		{
			AgentType:   "developer",
			IssueNumber: 456,
			State:       state.StateIdle, // This should be skipped
			UpdatedAt:   time.Now(),
		},
	}
	mockStore.On("List", ctx).Return(states, nil)

	// Mock validation for the active state only
	validationReport := &state.ValidationReport{
		Valid:              true,
		IssuesFound:        []*state.ValidationIssue{},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, states[0]).Return(validationReport, nil)
	// states[1] should not be validated because it's idle

	err := startupValidator.PerformPeriodicValidation(ctx)

	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_PerformPeriodicValidation_WithReconciliation(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Recovery: config.RecoveryConfig{
						Enabled: true,
						Consistency: config.ConsistencyConfig{
							ReconcileDrift: true, // Auto-reconcile enabled
						},
					},
				},
			},
		},
	}

	startupValidator := NewStartupValidator(deps, mockValidator)

	ctx := context.Background()

	// Mock agent states
	states := []*state.AgentWorkState{
		{
			AgentType:   "developer",
			IssueNumber: 123,
			State:       state.StateImplement,
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
		},
	}
	mockStore.On("List", ctx).Return(states, nil)

	// Mock validation with issues
	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{Type: state.IssueTypeInconsistentState, Severity: state.SeverityMedium},
		},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, states[0]).Return(validationReport, nil)

	// Mock successful reconciliation
	mockValidator.On("ReconcileState", ctx, states[0]).Return(nil)

	err := startupValidator.PerformPeriodicValidation(ctx)

	assert.NoError(t, err)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}
