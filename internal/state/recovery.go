package state

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
)

// RecoveryManager handles startup recovery from interrupted workflows
type RecoveryManager struct {
	store            Store
	github           ghub.Client
	config           *config.Config
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(store Store, github ghub.Client, cfg *config.Config, logger *slog.Logger) *RecoveryManager {
	return &RecoveryManager{
		store:  store,
		github: github,
		config: cfg,
		logger: logger,
	}
}

// WithObservability adds observability features to the recovery manager
func (rm *RecoveryManager) WithObservability(structuredLogger *observability.StructuredLogger) *RecoveryManager {
	rm.structuredLogger = structuredLogger
	return rm
}

// RecoverInterruptedWorkflows detects and handles interrupted work from previous runs
func (rm *RecoveryManager) RecoverInterruptedWorkflows(ctx context.Context) error {
	// Create enriched correlation context for recovery operations
	ctx = observability.EnsureCorrelationContext(ctx, "recovery_manager", 0)

	rm.logger.Info("starting workflow recovery")

	// Log recovery initiation
	if rm.structuredLogger != nil {
		rm.structuredLogger.LogAgentStart(ctx, "recovery_manager", "checking for interrupted workflows")
		rm.structuredLogger.LogWorkflowTransition(ctx, 0, "startup", "recovery_check", "system_restart_detected")
	}

	// Find all stored workflows
	workflows, err := rm.findInterruptedWorkflows(ctx)
	if err != nil {
		return fmt.Errorf("finding interrupted workflows: %w", err)
	}

	if len(workflows) == 0 {
		rm.logger.Info("no interrupted workflows found")
		if rm.structuredLogger != nil {
			rm.structuredLogger.LogWorkflowTransition(ctx, 0, "recovery_check", "recovery_complete", "no_interrupted_work")
		}
		return nil
	}

	rm.logger.Info("found interrupted workflows", "count", len(workflows))

	// Process each interrupted workflow
	var recoveryErr error
	recoveredCount := 0
	for _, workflow := range workflows {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := rm.recoverWorkflow(ctx, workflow); err != nil {
			rm.logger.Error("failed to recover workflow",
				"agent", workflow.AgentType,
				"issue", workflow.IssueNumber,
				"state", workflow.State,
				"error", err)
			if recoveryErr == nil {
				recoveryErr = err
			}
			continue
		}
		recoveredCount++
	}

	// Log recovery completion
	if rm.structuredLogger != nil {
		rm.structuredLogger.LogWorkflowTransition(ctx, 0, "recovery_check", "recovery_complete",
			fmt.Sprintf("recovered_%d_of_%d_workflows", recoveredCount, len(workflows)))
		rm.structuredLogger.LogDecisionPoint(ctx, "recovery_manager", "recovery_complete",
			fmt.Sprintf("processed %d interrupted workflows", len(workflows)), map[string]interface{}{
				"total_found":     len(workflows),
				"recovered_count": recoveredCount,
				"failed_count":    len(workflows) - recoveredCount,
			})
		rm.structuredLogger.LogAgentStop(ctx, "recovery_manager", 0, recoveryErr)
	}

	if recoveryErr != nil {
		rm.logger.Error("recovery completed with errors", "error", recoveryErr)
	} else {
		rm.logger.Info("recovery completed successfully", "recovered", recoveredCount)
	}

	return recoveryErr
}

// findInterruptedWorkflows searches for workflows that were interrupted
func (rm *RecoveryManager) findInterruptedWorkflows(ctx context.Context) ([]*AgentWorkState, error) {
	// This is a simple implementation - in practice you might want to scan
	// the state directory for checkpoint files or use a database query
	var workflows []*AgentWorkState

	// For file-based storage, we would scan for .checkpoint files
	// This is a placeholder implementation that would need to be expanded
	// based on the actual state store implementation

	rm.logger.Debug("scanning for interrupted workflows")

	// Look for workflows that were interrupted during shutdown
	// In a real implementation, this would query the state store for
	// entries with CheckpointedAt times and InterruptedBy fields

	return workflows, nil
}

// recoverWorkflow handles recovery of a single interrupted workflow
func (rm *RecoveryManager) recoverWorkflow(ctx context.Context, workflow *AgentWorkState) error {
	issueNum := workflow.IssueNumber
	rm.logger.Info("recovering workflow",
		"agent", workflow.AgentType,
		"issue", issueNum,
		"state", workflow.State,
		"interrupted_at", workflow.CheckpointedAt)

	// Create workflow-specific correlation context
	workflowCtx := observability.EnsureCorrelationContext(ctx, "recovery_manager", issueNum)
	workflowCtx = observability.WithMetadata(workflowCtx, "recovery_type", "interrupted_workflow")
	workflowCtx = observability.WithMetadata(workflowCtx, "original_agent", workflow.AgentType)

	// Log workflow recovery start
	if rm.structuredLogger != nil {
		rm.structuredLogger.LogWorkflowTransition(workflowCtx, issueNum, "interrupted", "recovering",
			fmt.Sprintf("recovering_%s_from_%s", workflow.AgentType, workflow.State))
	}

	var recoveryAction string
	var err error

	// Determine recovery action based on workflow state
	switch workflow.State {
	case StateClaim, StateWorkspace, StateAnalyze:
		// Early states - safe to reset and let agent reclaim
		recoveryAction = "reset_to_ready"
		err = rm.resetIssueToReady(workflowCtx, issueNum)

	case StateImplement:
		// In progress - need to check if we should resume or reset
		if rm.shouldResumeImplementation(workflow) {
			recoveryAction = "resume_implementation"
			err = rm.resumeImplementation(workflowCtx, workflow)
		} else {
			recoveryAction = "reset_to_ready"
			err = rm.resetIssueToReady(workflowCtx, issueNum)
		}

	case StateCommit, StatePR, StateValidation:
		// Late states - check if work was completed
		recoveryAction = "check_completion"
		err = rm.checkWorkflowCompletion(workflowCtx, workflow)

	case StateReview, StateComplete:
		// Final states - just clean up
		recoveryAction = "cleanup_completed"
		err = rm.cleanupCompletedWorkflow(workflowCtx, workflow)

	default:
		recoveryAction = "reset_to_ready"
		err = rm.resetIssueToReady(workflowCtx, issueNum)
	}

	// Log recovery decision and outcome
	if rm.structuredLogger != nil {
		rm.structuredLogger.LogDecisionPoint(workflowCtx, "recovery_manager", "recovery_action", recoveryAction, map[string]interface{}{
			"original_state":  workflow.State,
			"recovery_action": recoveryAction,
			"success":         err == nil,
		})

		if err == nil {
			rm.structuredLogger.LogWorkflowTransition(workflowCtx, issueNum, "recovering", "recovered", recoveryAction)
		} else {
			rm.structuredLogger.LogWorkflowTransition(workflowCtx, issueNum, "recovering", "recovery_failed", err.Error())
		}
	}

	if err != nil {
		return fmt.Errorf("recovery action %s failed: %w", recoveryAction, err)
	}

	// Clean up the checkpoint
	if err := rm.cleanupCheckpoint(workflowCtx, workflow); err != nil {
		rm.logger.Error("failed to clean up checkpoint", "issue", issueNum, "error", err)
		// Don't fail recovery for cleanup errors
	}

	return nil
}

// resetIssueToReady resets an issue back to ready state for re-processing
func (rm *RecoveryManager) resetIssueToReady(ctx context.Context, issueNum int) error {
	if !rm.config.Shutdown.ResetClaims {
		rm.logger.Debug("claim reset disabled, skipping", "issue", issueNum)
		return nil
	}

	rm.logger.Info("resetting issue to ready state", "issue", issueNum)

	// Remove agent:claimed label and add agent:ready
	if err := rm.github.RemoveLabel(ctx, issueNum, "agent:claimed"); err != nil {
		rm.logger.Error("failed to remove claimed label", "issue", issueNum, "error", err)
		// Continue anyway
	}

	if err := rm.github.RemoveLabel(ctx, issueNum, "agent:in-progress"); err != nil {
		rm.logger.Debug("failed to remove in-progress label (may not exist)", "issue", issueNum, "error", err)
	}

	if err := rm.github.AddLabels(ctx, issueNum, []string{"agent:ready"}); err != nil {
		return fmt.Errorf("adding ready label: %w", err)
	}

	// Add recovery comment
	comment := fmt.Sprintf("🔄 **Recovery**: This issue was being processed when the system restarted. " +
		"It has been reset to `agent:ready` and will be re-processed automatically.")
	if err := rm.github.CreateComment(ctx, issueNum, comment); err != nil {
		rm.logger.Error("failed to add recovery comment", "issue", issueNum, "error", err)
		// Don't fail for comment errors
	}

	return nil
}

// shouldResumeImplementation determines if implementation should be resumed or reset
func (rm *RecoveryManager) shouldResumeImplementation(workflow *AgentWorkState) bool {
	// For now, always reset implementation to avoid complexity
	// In the future, this could check if significant progress was made
	return false
}

// resumeImplementation attempts to resume an interrupted implementation
func (rm *RecoveryManager) resumeImplementation(ctx context.Context, workflow *AgentWorkState) error {
	// This would require more sophisticated state management
	// For now, fall back to reset
	return rm.resetIssueToReady(ctx, workflow.IssueNumber)
}

// checkWorkflowCompletion checks if a workflow in late stages actually completed
func (rm *RecoveryManager) checkWorkflowCompletion(ctx context.Context, workflow *AgentWorkState) error {
	issueNum := workflow.IssueNumber
	rm.logger.Debug("checking workflow completion status", "issue", issueNum)

	// Check if PR was created and is still open
	if workflow.PRNumber > 0 {
		pr, err := rm.github.GetPR(ctx, workflow.PRNumber)
		if err == nil && pr.GetState() == "open" {
			// PR exists and is open - workflow is legitimate
			rm.logger.Info("found open PR for interrupted workflow", "issue", issueNum, "pr", workflow.PRNumber)
			return nil
		}
	}

	// No valid PR found, reset to ready
	return rm.resetIssueToReady(ctx, issueNum)
}

// cleanupCompletedWorkflow cleans up a workflow that was already completed
func (rm *RecoveryManager) cleanupCompletedWorkflow(ctx context.Context, workflow *AgentWorkState) error {
	rm.logger.Debug("cleaning up completed workflow", "issue", workflow.IssueNumber)
	// Just remove the checkpoint - no other action needed
	return nil
}

// cleanupCheckpoint removes the checkpoint file for a recovered workflow
func (rm *RecoveryManager) cleanupCheckpoint(ctx context.Context, workflow *AgentWorkState) error {
	// Mark the workflow as recovered by updating its state
	workflow.State = "recovered"
	workflow.UpdatedAt = time.Now()
	workflow.InterruptedBy = ""

	// Save the updated state (this effectively marks it as processed)
	return rm.store.Save(ctx, workflow)
}
