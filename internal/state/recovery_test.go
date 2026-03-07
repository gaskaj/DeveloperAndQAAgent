package state

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
)

func TestNewRecoveryManager(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)

	require.NotNil(t, rm)
	assert.Equal(t, store, rm.store)
	assert.Equal(t, gh, rm.github)
	assert.Equal(t, cfg, rm.config)
	assert.Equal(t, logger, rm.logger)
	assert.Nil(t, rm.structuredLogger)
}

func TestRecoveryManager_WithObservability(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	// WithObservability returns itself for chaining
	result := rm.WithObservability(nil)
	assert.Equal(t, rm, result)
}

func TestRecoveryManager_RecoverInterruptedWorkflows_NoWorkflows(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)

	ctx := context.Background()
	err := rm.RecoverInterruptedWorkflows(ctx)

	assert.NoError(t, err)
}

func TestRecoveryManager_ShouldResumeImplementation(t *testing.T) {
	rm := &RecoveryManager{logger: slog.Default()}

	workflow := &AgentWorkState{
		State:     StateImplement,
		UpdatedAt: time.Now().Add(-30 * time.Minute),
	}

	// Currently always returns false
	assert.False(t, rm.shouldResumeImplementation(workflow))
}

func TestRecoveryManager_ResumeImplementation(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateImplement,
	}

	// resumeImplementation falls back to resetIssueToReady
	gh.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)

	err := rm.resumeImplementation(ctx, workflow)
	assert.NoError(t, err)
	gh.AssertExpectations(t)
}

func TestRecoveryManager_ResetIssueToReady_Enabled(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	gh.On("RemoveLabel", ctx, 10, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 10, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 10, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 10, mock.AnythingOfType("string")).Return(nil)

	err := rm.resetIssueToReady(ctx, 10)
	assert.NoError(t, err)
	gh.AssertExpectations(t)
}

func TestRecoveryManager_ResetIssueToReady_Disabled(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: false,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	err := rm.resetIssueToReady(ctx, 10)
	assert.NoError(t, err)
	// No GitHub calls expected
	gh.AssertNotCalled(t, "RemoveLabel")
	gh.AssertNotCalled(t, "AddLabels")
}

func TestRecoveryManager_ResetIssueToReady_AddLabelError(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	gh.On("RemoveLabel", ctx, 10, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 10, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 10, []string{"agent:ready"}).Return(errors.New("api error"))
	gh.On("CreateComment", ctx, 10, mock.AnythingOfType("string")).Return(nil)

	err := rm.resetIssueToReady(ctx, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding ready label")
}

func TestRecoveryManager_ResetIssueToReady_RemoveLabelErrors(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	// Both remove label calls fail - should still continue
	gh.On("RemoveLabel", ctx, 10, "agent:claimed").Return(errors.New("not found"))
	gh.On("RemoveLabel", ctx, 10, "agent:in-progress").Return(errors.New("not found"))
	gh.On("AddLabels", ctx, 10, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 10, mock.AnythingOfType("string")).Return(nil)

	err := rm.resetIssueToReady(ctx, 10)
	assert.NoError(t, err)
}

func TestRecoveryManager_ResetIssueToReady_CommentError(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	gh.On("RemoveLabel", ctx, 10, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 10, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 10, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 10, mock.AnythingOfType("string")).Return(errors.New("comment error"))

	// Comment error should not fail the overall operation
	err := rm.resetIssueToReady(ctx, 10)
	assert.NoError(t, err)
}

func TestRecoveryManager_CheckWorkflowCompletion_WithOpenPR(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateCommit,
		PRNumber:    100,
	}

	pr := &github.PullRequest{
		State: github.String("open"),
	}
	gh.On("GetPR", ctx, 100).Return(pr, nil)

	err := rm.checkWorkflowCompletion(ctx, workflow)
	assert.NoError(t, err)
	gh.AssertExpectations(t)
}

func TestRecoveryManager_CheckWorkflowCompletion_WithClosedPR(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateCommit,
		PRNumber:    100,
	}

	pr := &github.PullRequest{
		State: github.String("closed"),
	}
	gh.On("GetPR", ctx, 100).Return(pr, nil)

	// Since PR is closed, it falls back to resetIssueToReady
	gh.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)

	err := rm.checkWorkflowCompletion(ctx, workflow)
	assert.NoError(t, err)
	gh.AssertExpectations(t)
}

func TestRecoveryManager_CheckWorkflowCompletion_NoPR(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateCommit,
		PRNumber:    0, // No PR
	}

	// Falls through to resetIssueToReady
	gh.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)

	err := rm.checkWorkflowCompletion(ctx, workflow)
	assert.NoError(t, err)
}

func TestRecoveryManager_CheckWorkflowCompletion_PRFetchError(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateCommit,
		PRNumber:    100,
	}

	gh.On("GetPR", ctx, 100).Return(nil, errors.New("not found"))

	// PR fetch error => falls back to resetIssueToReady
	gh.On("RemoveLabel", ctx, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", ctx, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", ctx, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", ctx, 42, mock.AnythingOfType("string")).Return(nil)

	err := rm.checkWorkflowCompletion(ctx, workflow)
	assert.NoError(t, err)
}

func TestRecoveryManager_CleanupCompletedWorkflow(t *testing.T) {
	rm := &RecoveryManager{logger: slog.Default()}
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateComplete,
	}

	err := rm.cleanupCompletedWorkflow(ctx, workflow)
	assert.NoError(t, err)
}

func TestRecoveryManager_CleanupCheckpoint(t *testing.T) {
	store := &MockStore{}
	logger := slog.Default()

	rm := &RecoveryManager{
		store:  store,
		logger: logger,
	}
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:     "developer",
		IssueNumber:   42,
		State:         StateImplement,
		InterruptedBy: "shutdown",
	}

	store.On("Save", ctx, mock.MatchedBy(func(s *AgentWorkState) bool {
		return s.State == "recovered" && s.InterruptedBy == ""
	})).Return(nil)

	err := rm.cleanupCheckpoint(ctx, workflow)
	assert.NoError(t, err)
	assert.Equal(t, WorkflowState("recovered"), workflow.State)
	assert.Empty(t, workflow.InterruptedBy)
	store.AssertExpectations(t)
}

func TestRecoveryManager_CleanupCheckpoint_SaveError(t *testing.T) {
	store := &MockStore{}
	logger := slog.Default()

	rm := &RecoveryManager{
		store:  store,
		logger: logger,
	}
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		State:       StateImplement,
	}

	store.On("Save", ctx, mock.Anything).Return(errors.New("disk error"))

	err := rm.cleanupCheckpoint(ctx, workflow)
	assert.Error(t, err)
}

func TestRecoveryManager_RecoverWorkflow_EarlyStates(t *testing.T) {
	states := []WorkflowState{StateClaim, StateWorkspace, StateAnalyze}

	for _, st := range states {
		t.Run(string(st), func(t *testing.T) {
			store := &MockStore{}
			gh := &MockGitHubClient{}
			cfg := &config.Config{
				Shutdown: config.ShutdownConfig{
					ResetClaims: true,
				},
			}
			logger := slog.Default()

			rm := NewRecoveryManager(store, gh, cfg, logger)
			ctx := context.Background()

			workflow := &AgentWorkState{
				AgentType:      "developer",
				IssueNumber:    42,
				State:          st,
				CheckpointedAt: time.Now().Add(-10 * time.Minute),
			}

			// resetIssueToReady calls
			gh.On("RemoveLabel", mock.Anything, 42, "agent:claimed").Return(nil)
			gh.On("RemoveLabel", mock.Anything, 42, "agent:in-progress").Return(nil)
			gh.On("AddLabels", mock.Anything, 42, []string{"agent:ready"}).Return(nil)
			gh.On("CreateComment", mock.Anything, 42, mock.AnythingOfType("string")).Return(nil)

			// cleanupCheckpoint call
			store.On("Save", mock.Anything, mock.Anything).Return(nil)

			err := rm.recoverWorkflow(ctx, workflow)
			assert.NoError(t, err)
		})
	}
}

func TestRecoveryManager_RecoverWorkflow_ImplementState(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:      "developer",
		IssueNumber:    42,
		State:          StateImplement,
		CheckpointedAt: time.Now().Add(-10 * time.Minute),
	}

	// shouldResumeImplementation returns false, so resetIssueToReady is called
	gh.On("RemoveLabel", mock.Anything, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", mock.Anything, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", mock.Anything, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", mock.Anything, 42, mock.AnythingOfType("string")).Return(nil)
	store.On("Save", mock.Anything, mock.Anything).Return(nil)

	err := rm.recoverWorkflow(ctx, workflow)
	assert.NoError(t, err)
}

func TestRecoveryManager_RecoverWorkflow_LateStates(t *testing.T) {
	lateStates := []WorkflowState{StateCommit, StatePR, StateValidation}

	for _, st := range lateStates {
		t.Run(string(st), func(t *testing.T) {
			store := &MockStore{}
			gh := &MockGitHubClient{}
			cfg := &config.Config{
				Shutdown: config.ShutdownConfig{
					ResetClaims: true,
				},
			}
			logger := slog.Default()

			rm := NewRecoveryManager(store, gh, cfg, logger)
			ctx := context.Background()

			workflow := &AgentWorkState{
				AgentType:      "developer",
				IssueNumber:    42,
				State:          st,
				PRNumber:       100,
				CheckpointedAt: time.Now().Add(-10 * time.Minute),
			}

			// checkWorkflowCompletion: PR is open
			pr := &github.PullRequest{
				State: github.String("open"),
			}
			gh.On("GetPR", mock.Anything, 100).Return(pr, nil)
			store.On("Save", mock.Anything, mock.Anything).Return(nil)

			err := rm.recoverWorkflow(ctx, workflow)
			assert.NoError(t, err)
		})
	}
}

func TestRecoveryManager_RecoverWorkflow_FinalStates(t *testing.T) {
	finalStates := []WorkflowState{StateReview, StateComplete}

	for _, st := range finalStates {
		t.Run(string(st), func(t *testing.T) {
			store := &MockStore{}
			gh := &MockGitHubClient{}
			cfg := &config.Config{}
			logger := slog.Default()

			rm := NewRecoveryManager(store, gh, cfg, logger)
			ctx := context.Background()

			workflow := &AgentWorkState{
				AgentType:      "developer",
				IssueNumber:    42,
				State:          st,
				CheckpointedAt: time.Now().Add(-10 * time.Minute),
			}

			store.On("Save", mock.Anything, mock.Anything).Return(nil)

			err := rm.recoverWorkflow(ctx, workflow)
			assert.NoError(t, err)
		})
	}
}

func TestRecoveryManager_RecoverWorkflow_DefaultState(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:      "developer",
		IssueNumber:    42,
		State:          WorkflowState("unknown_state"),
		CheckpointedAt: time.Now().Add(-10 * time.Minute),
	}

	gh.On("RemoveLabel", mock.Anything, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", mock.Anything, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", mock.Anything, 42, []string{"agent:ready"}).Return(nil)
	gh.On("CreateComment", mock.Anything, 42, mock.AnythingOfType("string")).Return(nil)
	store.On("Save", mock.Anything, mock.Anything).Return(nil)

	err := rm.recoverWorkflow(ctx, workflow)
	assert.NoError(t, err)
}

func TestRecoveryManager_RecoverWorkflow_RecoveryActionFails(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			ResetClaims: true,
		},
	}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:      "developer",
		IssueNumber:    42,
		State:          StateClaim,
		CheckpointedAt: time.Now().Add(-10 * time.Minute),
	}

	// Make resetIssueToReady fail
	gh.On("RemoveLabel", mock.Anything, 42, "agent:claimed").Return(nil)
	gh.On("RemoveLabel", mock.Anything, 42, "agent:in-progress").Return(nil)
	gh.On("AddLabels", mock.Anything, 42, []string{"agent:ready"}).Return(errors.New("api error"))
	gh.On("CreateComment", mock.Anything, 42, mock.AnythingOfType("string")).Return(nil)

	err := rm.recoverWorkflow(ctx, workflow)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "recovery action")
}

func TestRecoveryManager_RecoverWorkflow_CleanupCheckpointFails(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	workflow := &AgentWorkState{
		AgentType:      "developer",
		IssueNumber:    42,
		State:          StateComplete,
		CheckpointedAt: time.Now().Add(-10 * time.Minute),
	}

	// cleanupCheckpoint fails but that should NOT cause recoverWorkflow to fail
	store.On("Save", mock.Anything, mock.Anything).Return(errors.New("disk error"))

	err := rm.recoverWorkflow(ctx, workflow)
	assert.NoError(t, err) // cleanup errors don't fail recovery
}

func TestRecoveryManager_FindInterruptedWorkflows(t *testing.T) {
	store := &MockStore{}
	gh := &MockGitHubClient{}
	cfg := &config.Config{}
	logger := slog.Default()

	rm := NewRecoveryManager(store, gh, cfg, logger)
	ctx := context.Background()

	// Currently the implementation always returns empty
	workflows, err := rm.findInterruptedWorkflows(ctx)
	assert.NoError(t, err)
	assert.Empty(t, workflows)
}
