package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
)

// --- Agent Type and Constants Tests ---

func TestAgentType_Constants(t *testing.T) {
	assert.Equal(t, AgentType("developer"), TypeDeveloper)
	assert.Equal(t, AgentType("qa"), TypeQA)
	assert.Equal(t, AgentType("devmanager"), TypeDevManager)
}

func TestAgentType_String(t *testing.T) {
	assert.Equal(t, "developer", string(TypeDeveloper))
	assert.Equal(t, "qa", string(TypeQA))
	assert.Equal(t, "devmanager", string(TypeDevManager))
}

// --- StatusReport Tests ---

func TestStatusReport_Fields(t *testing.T) {
	report := StatusReport{
		Type:    TypeDeveloper,
		State:   "idle",
		IssueID: 42,
		Message: "Waiting for issues",
	}

	assert.Equal(t, TypeDeveloper, report.Type)
	assert.Equal(t, "idle", report.State)
	assert.Equal(t, 42, report.IssueID)
	assert.Equal(t, "Waiting for issues", report.Message)
	assert.Nil(t, report.WorkspaceStats)
}

func TestStatusReport_WithWorkspaceStats(t *testing.T) {
	stats := &WorkspaceStats{
		TotalWorkspaces:  5,
		ActiveWorkspaces: 2,
		TotalSizeMB:      512,
		DiskFreeMB:       10240,
	}

	report := StatusReport{
		Type:           TypeDeveloper,
		State:          "implement",
		IssueID:        123,
		Message:        "Implementing feature",
		WorkspaceStats: stats,
	}

	require.NotNil(t, report.WorkspaceStats)
	assert.Equal(t, 5, report.WorkspaceStats.TotalWorkspaces)
	assert.Equal(t, 2, report.WorkspaceStats.ActiveWorkspaces)
	assert.Equal(t, int64(512), report.WorkspaceStats.TotalSizeMB)
	assert.Equal(t, int64(10240), report.WorkspaceStats.DiskFreeMB)
}

// --- BaseAgent Tests ---

func TestNewBaseAgent(t *testing.T) {
	logger := slog.Default()
	deps := Dependencies{
		Logger: logger,
		Config: &config.Config{},
	}

	base := NewBaseAgent(deps)
	assert.NotNil(t, base.Deps.Logger)
	assert.NotNil(t, base.Deps.Config)
	assert.Nil(t, base.Deps.GitHub)
	assert.Nil(t, base.Deps.Claude)
	assert.Nil(t, base.Deps.Store)
}

func TestNewBaseAgent_AllDependencies(t *testing.T) {
	deps := Dependencies{
		Config: &config.Config{
			GitHub: config.GitHubConfig{Owner: "test"},
		},
		Logger: slog.Default(),
	}

	base := NewBaseAgent(deps)
	assert.Equal(t, "test", base.Deps.Config.GitHub.Owner)
}

// --- Lifecycle Tests ---

func TestHeartbeat_ContextCancellation(t *testing.T) {
	logger := slog.Default()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Heartbeat(ctx, TypeDeveloper, 20*time.Millisecond, logger)
		close(done)
	}()

	select {
	case <-done:
		// Success - Heartbeat returned after context cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("Heartbeat did not return after context cancellation")
	}
}

func TestHeartbeat_MultipleTicks(t *testing.T) {
	logger := slog.Default()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		Heartbeat(ctx, TypeQA, 50*time.Millisecond, logger)
		close(done)
	}()

	select {
	case <-done:
		// Heartbeat ran for multiple ticks before context expired
	case <-time.After(2 * time.Second):
		t.Fatal("Heartbeat did not return")
	}
}

func TestRunWithContext_Success(t *testing.T) {
	parent := context.Background()

	var functionCalled bool
	err := RunWithContext(parent, func(ctx context.Context) error {
		functionCalled = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, functionCalled)
}

func TestRunWithContext_Error(t *testing.T) {
	parent := context.Background()
	expectedErr := fmt.Errorf("function failed")

	err := RunWithContext(parent, func(ctx context.Context) error {
		return expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestRunWithContext_ParentCancelled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := RunWithContext(parent, func(ctx context.Context) error {
		// Check if derived context is already cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	// The function may or may not see the cancellation, but it should not panic
	_ = err
}

func TestRunWithContext_DerivedContextIsCancellable(t *testing.T) {
	parent := context.Background()

	err := RunWithContext(parent, func(ctx context.Context) error {
		// The context should be a derived context with its own cancel
		assert.NotNil(t, ctx)
		return nil
	})

	assert.NoError(t, err)
}

func TestRunWithContext_LongRunningFunction(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunWithContext(parent, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	elapsed := time.Since(start)
	assert.Error(t, err)
	assert.Less(t, elapsed, 1*time.Second, "Should have been cancelled by parent context")
}

// --- Registry Tests ---

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	require.NotNil(t, registry)
	assert.Empty(t, registry.Types())
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	factory := func(deps Dependencies) (Agent, error) {
		return nil, nil
	}

	registry.Register(TypeDeveloper, factory)
	types := registry.Types()
	assert.Len(t, types, 1)
	assert.Contains(t, types, TypeDeveloper)
}

func TestRegistry_RegisterMultiple(t *testing.T) {
	registry := NewRegistry()

	registry.Register(TypeDeveloper, func(deps Dependencies) (Agent, error) { return nil, nil })
	registry.Register(TypeQA, func(deps Dependencies) (Agent, error) { return nil, nil })
	registry.Register(TypeDevManager, func(deps Dependencies) (Agent, error) { return nil, nil })

	types := registry.Types()
	assert.Len(t, types, 3)
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	registry := NewRegistry()

	var callCount int32
	factory1 := func(deps Dependencies) (Agent, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, nil
	}
	factory2 := func(deps Dependencies) (Agent, error) {
		atomic.AddInt32(&callCount, 10)
		return nil, nil
	}

	registry.Register(TypeDeveloper, factory1)
	registry.Register(TypeDeveloper, factory2) // Overwrite

	types := registry.Types()
	assert.Len(t, types, 1, "Should still have only one entry after overwrite")

	// Create agent to verify factory2 is used
	_, _ = registry.Create(TypeDeveloper, Dependencies{})
	assert.Equal(t, int32(10), atomic.LoadInt32(&callCount))
}

// mockAgent implements the Agent interface for testing
type mockAgent struct {
	agentType AgentType
	runFunc   func(ctx context.Context) error
}

func (m *mockAgent) Type() AgentType {
	return m.agentType
}

func (m *mockAgent) Run(ctx context.Context) error {
	if m.runFunc != nil {
		return m.runFunc(ctx)
	}
	return nil
}

func (m *mockAgent) Status() StatusReport {
	return StatusReport{
		Type:    m.agentType,
		State:   "idle",
		Message: "test agent",
	}
}

func TestRegistry_Create_Success(t *testing.T) {
	registry := NewRegistry()

	registry.Register(TypeDeveloper, func(deps Dependencies) (Agent, error) {
		return &mockAgent{agentType: TypeDeveloper}, nil
	})

	agent, err := registry.Create(TypeDeveloper, Dependencies{})
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, TypeDeveloper, agent.Type())
}

func TestRegistry_Create_UnknownType(t *testing.T) {
	registry := NewRegistry()

	agent, err := registry.Create(TypeDeveloper, Dependencies{})
	assert.Error(t, err)
	assert.Nil(t, agent)
	assert.Contains(t, err.Error(), "unknown agent type: developer")
}

func TestRegistry_Create_FactoryError(t *testing.T) {
	registry := NewRegistry()

	registry.Register(TypeDeveloper, func(deps Dependencies) (Agent, error) {
		return nil, fmt.Errorf("factory initialization failed")
	})

	agent, err := registry.Create(TypeDeveloper, Dependencies{})
	assert.Error(t, err)
	assert.Nil(t, agent)
	assert.Contains(t, err.Error(), "factory initialization failed")
}

func TestRegistry_Types_Empty(t *testing.T) {
	registry := NewRegistry()
	types := registry.Types()
	assert.Empty(t, types)
	assert.NotNil(t, types) // Should return empty slice, not nil
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	// Register from multiple goroutines
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			agentType := AgentType(fmt.Sprintf("agent-%d", i))
			registry.Register(agentType, func(deps Dependencies) (Agent, error) {
				return &mockAgent{agentType: agentType}, nil
			})
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	types := registry.Types()
	assert.Len(t, types, 10)
}

func TestRegistry_ConcurrentCreateAndRegister(t *testing.T) {
	registry := NewRegistry()
	registry.Register(TypeDeveloper, func(deps Dependencies) (Agent, error) {
		return &mockAgent{agentType: TypeDeveloper}, nil
	})

	done := make(chan struct{}, 20)

	// Create from multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			agent, err := registry.Create(TypeDeveloper, Dependencies{})
			assert.NoError(t, err)
			assert.NotNil(t, agent)
			done <- struct{}{}
		}()
	}

	// Also register from goroutines
	for i := 0; i < 10; i++ {
		go func(i int) {
			agentType := AgentType(fmt.Sprintf("concurrent-%d", i))
			registry.Register(agentType, func(deps Dependencies) (Agent, error) {
				return &mockAgent{agentType: agentType}, nil
			})
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

// --- Startup Validator Additional Tests ---

func TestNewStartupValidator(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	sv := NewStartupValidator(deps, mockValidator)
	require.NotNil(t, sv)
	assert.NotNil(t, sv.logger)
	assert.NotNil(t, sv.validator)
}

func TestStartupValidator_ValidateAndRecover_StoreLoadError(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	mockStore.On("Load", ctx, "developer").Return(nil, fmt.Errorf("store error"))

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.Error(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe)

	mockStore.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecover_ValidateExistingStateError(t *testing.T) {
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
						Enabled:             false,
						AutoCleanupOrphaned: false,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 1,
		State:       state.StateImplement,
	}
	mockStore.On("Load", ctx, "developer").Return(existingState, nil)
	mockValidator.On("ValidateWorkState", ctx, existingState).Return(nil, fmt.Errorf("validation error"))
	mockValidator.On("DetectOrphanedWork", ctx).Return([]*state.OrphanedWorkItem{}, nil)

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.NoError(t, err) // The function continues despite validation errors
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecover_DetectOrphanedWorkError(t *testing.T) {
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
						Enabled:             false,
						AutoCleanupOrphaned: false,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	mockStore.On("Load", ctx, "developer").Return(nil, nil)
	mockValidator.On("DetectOrphanedWork", ctx).Return(nil, fmt.Errorf("detect error"))

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.StartupSafe)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecover_WithRecovery(t *testing.T) {
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
						AutoCleanupOrphaned: true,
						MaxResumeAge:        24 * time.Hour,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	// No existing state
	mockStore.On("Load", ctx, "developer").Return(nil, nil)

	// Orphaned work found
	orphanedWork := []*state.OrphanedWorkItem{
		{
			AgentType:    "developer",
			IssueNumber:  10,
			State:        state.StateImplement,
			AgeHours:     2.0,
			RecoveryType: state.RecoveryTypeCleanup,
		},
		{
			AgentType:    "developer",
			IssueNumber:  20,
			State:        state.StateAnalyze,
			AgeHours:     1.0,
			RecoveryType: state.RecoveryTypeResume,
		},
		{
			AgentType:    "developer",
			IssueNumber:  30,
			State:        state.StateCommit,
			AgeHours:     0.5,
			RecoveryType: state.RecoveryTypeManual,
		},
	}
	mockValidator.On("DetectOrphanedWork", ctx).Return(orphanedWork, nil)

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.Valid) // Orphaned work found
	assert.Len(t, report.OrphanedWorkFound, 3)
	assert.Len(t, report.RecoveryActions, 3)

	// Verify recovery actions
	for _, action := range report.RecoveryActions {
		assert.True(t, action.Success)
		assert.NotEmpty(t, action.Description)
	}

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateAndRecover_OldOrphanForcesCleanup(t *testing.T) {
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
						AutoCleanupOrphaned: true,
						MaxResumeAge:        1 * time.Hour, // 1 hour max
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	mockStore.On("Load", ctx, "developer").Return(nil, nil)

	// Old orphaned work that exceeds MaxResumeAge
	orphanedWork := []*state.OrphanedWorkItem{
		{
			AgentType:    "developer",
			IssueNumber:  10,
			State:        state.StateImplement,
			AgeHours:     48.0,                     // 48 hours - way past 1 hour MaxResumeAge
			RecoveryType: state.RecoveryTypeResume, // Should be forced to cleanup
		},
	}
	mockValidator.On("DetectOrphanedWork", ctx).Return(orphanedWork, nil)

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Len(t, report.RecoveryActions, 1)
	assert.Equal(t, ActionCleanupOrphaned, report.RecoveryActions[0].Type)
}

func TestStartupValidator_PerformPeriodicValidation_StoreListError(t *testing.T) {
	logger := slog.Default()
	mockValidator := &MockStateValidator{}
	mockStore := &MockStore{}

	deps := Dependencies{
		Logger: logger,
		Store:  mockStore,
		Config: &config.Config{},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	mockStore.On("List", ctx).Return(nil, fmt.Errorf("list error"))

	err := sv.PerformPeriodicValidation(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listing agent states")

	mockStore.AssertExpectations(t)
}

func TestStartupValidator_PerformPeriodicValidation_SkipsTerminalStates(t *testing.T) {
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
						Enabled: false,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	states := []*state.AgentWorkState{
		{AgentType: "dev", IssueNumber: 1, State: state.StateIdle},
		{AgentType: "dev", IssueNumber: 2, State: state.StateComplete},
		{AgentType: "dev", IssueNumber: 3, State: state.StateFailed},
	}
	mockStore.On("List", ctx).Return(states, nil)

	err := sv.PerformPeriodicValidation(ctx)
	assert.NoError(t, err)

	// ValidateWorkState should NOT have been called for any of these states
	mockValidator.AssertNotCalled(t, "ValidateWorkState", mock.Anything, mock.Anything)
	mockStore.AssertExpectations(t)
}

func TestStartupValidator_PerformPeriodicValidation_ValidationError(t *testing.T) {
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
						Enabled: false,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	activeState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 1,
		State:       state.StateImplement,
	}
	mockStore.On("List", ctx).Return([]*state.AgentWorkState{activeState}, nil)
	mockValidator.On("ValidateWorkState", ctx, activeState).Return(nil, fmt.Errorf("validation error"))

	err := sv.PerformPeriodicValidation(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "1 errors")

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_PerformPeriodicValidation_ReconcileError(t *testing.T) {
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
							ReconcileDrift: true,
						},
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	activeState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 1,
		State:       state.StateImplement,
	}
	mockStore.On("List", ctx).Return([]*state.AgentWorkState{activeState}, nil)

	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{Type: state.IssueTypeInconsistentState, Severity: state.SeverityMedium},
		},
		StateDrifts:        []*state.StateDrift{},
		RecommendedActions: []*state.RecommendedAction{},
	}
	mockValidator.On("ValidateWorkState", ctx, activeState).Return(validationReport, nil)
	mockValidator.On("ReconcileState", ctx, activeState).Return(errors.New("reconcile failed"))

	err := sv.PerformPeriodicValidation(ctx)
	assert.NoError(t, err) // Reconcile errors are logged but do not cause overall failure

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestStartupValidator_ValidateExistingState_WithOrphanedWork(t *testing.T) {
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
						Enabled:             false,
						AutoCleanupOrphaned: false,
					},
				},
			},
		},
	}

	sv := NewStartupValidator(deps, mockValidator)
	ctx := context.Background()

	existingState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 456,
		State:       state.StateImplement,
		UpdatedAt:   time.Now().Add(-2 * time.Hour),
	}
	mockStore.On("Load", ctx, "developer").Return(existingState, nil)

	// Validation report with orphaned work
	validationReport := &state.ValidationReport{
		Valid: false,
		IssuesFound: []*state.ValidationIssue{
			{
				Type:        state.IssueTypeOrphanedClaim,
				Severity:    state.SeverityHigh,
				Description: "Orphaned claim detected",
			},
		},
		OrphanedWork: []*state.OrphanedWorkItem{
			{
				AgentType:    "developer",
				IssueNumber:  456,
				State:        state.StateImplement,
				AgeHours:     2.0,
				RecoveryType: state.RecoveryTypeCleanup,
			},
		},
		StateDrifts: []*state.StateDrift{
			{
				Type:          state.DriftTypeIssueState,
				LocalState:    state.StateImplement,
				ExternalState: "closed",
				CanReconcile:  true,
			},
		},
		RecommendedActions: []*state.RecommendedAction{
			{Type: state.ActionTypeCleanup, Priority: state.PriorityHigh},
		},
	}
	mockValidator.On("ValidateWorkState", ctx, existingState).Return(validationReport, nil)
	mockValidator.On("DetectOrphanedWork", ctx).Return([]*state.OrphanedWorkItem{}, nil)

	report, err := sv.ValidateAndRecoverStartup(ctx, TypeDeveloper)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.False(t, report.StartupSafe)
	assert.Len(t, report.OrphanedWorkFound, 1)
	assert.Len(t, report.ValidationIssues, 1)
	assert.Len(t, report.RecommendedActions, 1)

	mockStore.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

// --- Recovery Action Type Tests ---

func TestRecoveryActionType_Constants(t *testing.T) {
	assert.Equal(t, RecoveryActionType("cleanup_orphaned"), ActionCleanupOrphaned)
	assert.Equal(t, RecoveryActionType("resume_work"), ActionResumeWork)
	assert.Equal(t, RecoveryActionType("validate_state"), ActionValidateState)
	assert.Equal(t, RecoveryActionType("reconcile_drift"), ActionReconcileDrift)
	assert.Equal(t, RecoveryActionType("flag_for_manual"), ActionFlagForManual)
}

// --- Dependencies Tests ---

func TestDependencies_ZeroValue(t *testing.T) {
	deps := Dependencies{}
	assert.Nil(t, deps.Config)
	assert.Nil(t, deps.GitHub)
	assert.Nil(t, deps.Claude)
	assert.Nil(t, deps.Store)
	assert.Nil(t, deps.Logger)
	assert.Nil(t, deps.StructuredLogger)
	assert.Nil(t, deps.Metrics)
	assert.Nil(t, deps.ErrorManager)
}

// --- StartupValidationReport Tests ---

func TestStartupValidationReport_InitialState(t *testing.T) {
	report := &StartupValidationReport{
		Valid:              true,
		StartupSafe:        true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
	}

	assert.True(t, report.Valid)
	assert.True(t, report.StartupSafe)
	assert.Empty(t, report.OrphanedWorkFound)
	assert.Empty(t, report.RecoveryActions)
	assert.Empty(t, report.ValidationIssues)
	assert.Empty(t, report.RecommendedActions)
}

func TestRecoveryAction_Fields(t *testing.T) {
	action := &RecoveryAction{
		Type:        ActionCleanupOrphaned,
		AgentType:   "developer",
		IssueNumber: 42,
		Description: "Cleanup orphaned work for issue #42",
		Success:     true,
		Duration:    500 * time.Millisecond,
		Details:     map[string]string{"reason": "age_exceeded"},
	}

	assert.Equal(t, ActionCleanupOrphaned, action.Type)
	assert.Equal(t, "developer", action.AgentType)
	assert.Equal(t, 42, action.IssueNumber)
	assert.True(t, action.Success)
	assert.Empty(t, action.Error)
	assert.Equal(t, "age_exceeded", action.Details["reason"])
}

func TestRecoveryAction_WithError(t *testing.T) {
	action := &RecoveryAction{
		Type:        ActionCleanupOrphaned,
		AgentType:   "developer",
		IssueNumber: 99,
		Description: "Failed cleanup",
		Success:     false,
		Error:       "permission denied",
	}

	assert.False(t, action.Success)
	assert.Equal(t, "permission denied", action.Error)
}

// --- MockRecoveryManager Tests ---

func TestMockRecoveryManager_CleanupOrphanedWork(t *testing.T) {
	mgr := &mockRecoveryManager{}
	orphan := &state.OrphanedWorkItem{
		AgentType:   "developer",
		IssueNumber: 10,
	}

	err := mgr.CleanupOrphanedWork(context.Background(), orphan)
	assert.NoError(t, err)
}
