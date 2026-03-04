package developer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// CheckpointManager handles workflow state checkpointing for the developer agent
type CheckpointManager struct {
	store  state.Store
	logger *slog.Logger
}

// NewCheckpointManager creates a new checkpoint manager
func NewCheckpointManager(store state.Store, logger *slog.Logger) *CheckpointManager {
	return &CheckpointManager{
		store:  store,
		logger: logger,
	}
}

// CreateCheckpoint saves the current workflow state at a specific checkpoint
func (cm *CheckpointManager) CreateCheckpoint(ctx context.Context, ws *state.AgentWorkState, stage string, metadata map[string]interface{}) error {
	// Create enriched correlation context for checkpointing
	ctx = observability.EnsureCorrelationContext(ctx, "checkpoint_manager", ws.IssueNumber)
	ctx = observability.WithMetadata(ctx, "checkpoint_stage", stage)

	cm.logger.Debug("creating checkpoint",
		"issue", ws.IssueNumber,
		"stage", stage,
		"state", ws.State)

	// Update checkpoint metadata
	checkpoint := *ws // Copy the workflow state
	checkpoint.CheckpointedAt = time.Now()
	checkpoint.CheckpointStage = stage
	checkpoint.CheckpointMetadata = metadata

	// Save the checkpoint
	if err := cm.store.Save(ctx, &checkpoint); err != nil {
		return fmt.Errorf("saving checkpoint for issue %d at stage %s: %w", ws.IssueNumber, stage, err)
	}

	cm.logger.Debug("checkpoint created successfully",
		"issue", ws.IssueNumber,
		"stage", stage)

	return nil
}

// RestoreCheckpoint loads the most recent checkpoint for a workflow
func (cm *CheckpointManager) RestoreCheckpoint(ctx context.Context, agentType string, issueNum int) (*state.AgentWorkState, error) {
	ctx = observability.EnsureCorrelationContext(ctx, "checkpoint_manager", issueNum)

	cm.logger.Debug("restoring checkpoint",
		"agent", agentType,
		"issue", issueNum)

	// Load the workflow state
	ws, err := cm.store.Load(ctx, agentType)
	if err != nil {
		return nil, fmt.Errorf("loading checkpoint for issue %d: %w", issueNum, err)
	}

	if ws.IssueNumber != issueNum {
		return nil, fmt.Errorf("no checkpoint found for issue %d (state has issue %d)", issueNum, ws.IssueNumber)
	}

	if ws.CheckpointedAt.IsZero() {
		return nil, fmt.Errorf("no checkpoint found for issue %d", issueNum)
	}

	cm.logger.Debug("checkpoint restored",
		"issue", issueNum,
		"stage", ws.CheckpointStage,
		"checkpointed_at", ws.CheckpointedAt)

	return ws, nil
}

// CleanupCheckpoint removes checkpoint data for a completed workflow
func (cm *CheckpointManager) CleanupCheckpoint(ctx context.Context, ws *state.AgentWorkState) error {
	ctx = observability.EnsureCorrelationContext(ctx, "checkpoint_manager", ws.IssueNumber)

	cm.logger.Debug("cleaning up checkpoint",
		"issue", ws.IssueNumber,
		"final_state", ws.State)

	// Clear checkpoint-specific fields
	ws.CheckpointedAt = time.Time{}
	ws.CheckpointStage = ""
	ws.CheckpointMetadata = nil
	ws.InterruptedBy = ""

	// Save the cleaned state
	if err := cm.store.Save(ctx, ws); err != nil {
		return fmt.Errorf("cleaning checkpoint for issue %d: %w", ws.IssueNumber, err)
	}

	return nil
}

// CreateWorkspaceCleanupHandler returns a cleanup handler for workspace directories
func CreateWorkspaceCleanupHandler(workspaceDir string, logger *slog.Logger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		logger.Debug("cleanup handler: workspace directories", "dir", workspaceDir)

		// This would typically:
		// 1. Save any unsaved files
		// 2. Close file handles
		// 3. Clean temporary files
		// 4. Record cleanup completion

		// For now, just log the action
		logger.Info("workspace cleanup handler executed", "dir", workspaceDir)
		return nil
	}
}

// CreateGitCleanupHandler returns a cleanup handler for Git operations
func CreateGitCleanupHandler(logger *slog.Logger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		logger.Debug("cleanup handler: git operations")

		// This would typically:
		// 1. Complete or abort in-progress commits
		// 2. Clean up temporary branches
		// 3. Close Git connections
		// 4. Save Git state

		logger.Info("git cleanup handler executed")
		return nil
	}
}

// CreateFileHandleCleanupHandler returns a cleanup handler for open file handles
func CreateFileHandleCleanupHandler(logger *slog.Logger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		logger.Debug("cleanup handler: file handles")

		// This would typically:
		// 1. Close open files
		// 2. Flush buffers
		// 3. Release locks
		// 4. Clean temporary files

		logger.Info("file handle cleanup handler executed")
		return nil
	}
}
