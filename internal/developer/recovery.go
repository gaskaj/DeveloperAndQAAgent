package developer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// RecoveryManager handles workflow recovery and cleanup operations.
type RecoveryManager struct {
	deps      agent.Dependencies
	validator state.Validator
	logger    *slog.Logger
}

// ResumptionPlan describes how to resume an interrupted workflow.
type ResumptionPlan struct {
	CanResume         bool                   `json:"can_resume"`
	ResumeFromState   state.WorkflowState    `json:"resume_from_state"`
	RequiredCleanup   []CleanupAction        `json:"required_cleanup"`
	EstimatedDuration time.Duration          `json:"estimated_duration"`
	RiskLevel         ResumptionRisk         `json:"risk_level"`
	RecommendedAction RecommendedResumption  `json:"recommended_action"`
	Reason            string                 `json:"reason"`
	Details           map[string]interface{} `json:"details,omitempty"`
}

// CleanupAction represents a cleanup operation required before resumption.
type CleanupAction struct {
	Type        CleanupActionType `json:"type"`
	Description string            `json:"description"`
	Required    bool              `json:"required"`
	Details     map[string]string `json:"details,omitempty"`
}

// Enum types for recovery operations
type ResumptionRisk string

const (
	RiskLow      ResumptionRisk = "low"
	RiskMedium   ResumptionRisk = "medium"
	RiskHigh     ResumptionRisk = "high"
	RiskCritical ResumptionRisk = "critical"
)

type RecommendedResumption string

const (
	RecommendResume   RecommendedResumption = "resume"
	RecommendRestart  RecommendedResumption = "restart"
	RecommendCleanup  RecommendedResumption = "cleanup"
	RecommendManual   RecommendedResumption = "manual"
)

type CleanupActionType string

const (
	CleanupWorkspace   CleanupActionType = "cleanup_workspace"
	CleanupBranch      CleanupActionType = "cleanup_branch"
	CleanupPR          CleanupActionType = "cleanup_pr"
	CleanupLabels      CleanupActionType = "cleanup_labels"
	CleanupCheckpoint  CleanupActionType = "cleanup_checkpoint"
	CleanupState       CleanupActionType = "cleanup_state"
)

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(deps agent.Dependencies, validator state.Validator) *RecoveryManager {
	return &RecoveryManager{
		deps:      deps,
		validator: validator,
		logger:    deps.Logger.With("component", "recovery_manager"),
	}
}

// AttemptResume analyzes a work state and creates a resumption plan.
func (r *RecoveryManager) AttemptResume(ctx context.Context, workState *state.AgentWorkState) (*ResumptionPlan, error) {
	r.logger.Info("analyzing resumption possibilities", 
		"agent_type", workState.AgentType,
		"issue_number", workState.IssueNumber,
		"current_state", workState.State,
		"last_update", workState.UpdatedAt)

	// First validate the current state
	validationReport, err := r.validator.ValidateWorkState(ctx, workState)
	if err != nil {
		return nil, fmt.Errorf("validating work state: %w", err)
	}

	plan := &ResumptionPlan{
		RequiredCleanup: make([]CleanupAction, 0),
		Details:         make(map[string]interface{}),
	}

	// Check basic resumption conditions
	plan.CanResume = r.canResumeWork(workState, validationReport)
	plan.RiskLevel = r.assessResumptionRisk(workState, validationReport)
	plan.ResumeFromState = r.determineResumeState(workState)
	plan.EstimatedDuration = r.estimateResumptionDuration(workState)

	// Generate required cleanup actions
	plan.RequiredCleanup = r.generateCleanupActions(workState, validationReport)

	// Make final recommendation
	plan.RecommendedAction, plan.Reason = r.makeResumptionRecommendation(workState, plan, validationReport)

	// Add detailed context
	plan.Details["validation_issues"] = len(validationReport.IssuesFound)
	plan.Details["orphaned_work"] = len(validationReport.OrphanedWork)
	plan.Details["state_drifts"] = len(validationReport.StateDrifts)
	plan.Details["age_hours"] = time.Since(workState.UpdatedAt).Hours()

	r.logger.Info("resumption analysis completed",
		"can_resume", plan.CanResume,
		"recommended_action", plan.RecommendedAction,
		"risk_level", plan.RiskLevel,
		"reason", plan.Reason)

	return plan, nil
}

// CleanupOrphanedWork cleans up orphaned work items.
func (r *RecoveryManager) CleanupOrphanedWork(ctx context.Context, orphaned *state.OrphanedWorkItem) error {
	r.logger.Info("cleaning up orphaned work",
		"agent_type", orphaned.AgentType,
		"issue_number", orphaned.IssueNumber,
		"state", orphaned.State,
		"age_hours", orphaned.AgeHours)

	// Load the full work state
	workState, err := r.deps.Store.Load(ctx, orphaned.AgentType)
	if err != nil {
		return fmt.Errorf("loading work state for cleanup: %w", err)
	}

	if workState == nil {
		r.logger.Warn("work state not found for cleanup", "agent_type", orphaned.AgentType)
		return nil // Already cleaned up
	}

	// Only clean up if it matches the orphaned item
	if workState.IssueNumber != orphaned.IssueNumber {
		r.logger.Warn("work state mismatch during cleanup",
			"expected_issue", orphaned.IssueNumber,
			"actual_issue", workState.IssueNumber)
		return nil
	}

	// Perform cleanup based on the current state
	switch orphaned.RecoveryType {
	case state.RecoveryTypeCleanup:
		return r.performFullCleanup(ctx, workState)
	case state.RecoveryTypeResume:
		// For resume type, first try to resume, fallback to cleanup
		plan, err := r.AttemptResume(ctx, workState)
		if err != nil || !plan.CanResume || plan.RecommendedAction != RecommendResume {
			r.logger.Info("resumption not viable, falling back to cleanup")
			return r.performFullCleanup(ctx, workState)
		}
		r.logger.Info("work item eligible for resumption, skipping cleanup")
		return nil
	case state.RecoveryTypeManual:
		r.logger.Info("orphaned work requires manual intervention", 
			"issue_number", orphaned.IssueNumber)
		return r.flagForManualIntervention(ctx, workState)
	}

	return nil
}

// ValidateWorkspaceConsistency validates workspace directory consistency.
func (r *RecoveryManager) ValidateWorkspaceConsistency(ctx context.Context, workspaceDir string, issueNum int) error {
	if workspaceDir == "" {
		return nil // No workspace to validate
	}

	r.logger.Debug("validating workspace consistency", 
		"workspace_dir", workspaceDir,
		"issue_number", issueNum)

	// Check if workspace directory exists
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		return fmt.Errorf("workspace directory does not exist: %s", workspaceDir)
	} else if err != nil {
		return fmt.Errorf("checking workspace directory: %w", err)
	}

	// Check if it's a valid git repository
	gitDir := filepath.Join(workspaceDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("workspace is not a git repository: %s", workspaceDir)
	}

	// Additional workspace validation would go here:
	// - Check if correct branch is checked out
	// - Validate remote configuration
	// - Check for uncommitted changes
	// - Verify workspace size and file count

	r.logger.Debug("workspace consistency validation passed", "workspace_dir", workspaceDir)
	return nil
}

// canResumeWork determines if work can be resumed.
func (r *RecoveryManager) canResumeWork(workState *state.AgentWorkState, report *state.ValidationReport) bool {
	// Cannot resume terminal states
	if workState.State == state.StateComplete || workState.State == state.StateFailed {
		return false
	}

	// Cannot resume if too old (more than 24 hours)
	if time.Since(workState.UpdatedAt) > 24*time.Hour {
		return false
	}

	// Cannot resume if there are critical validation issues
	for _, issue := range report.IssuesFound {
		if issue.Severity == state.SeverityCritical {
			return false
		}
	}

	// Cannot resume if the issue is closed
	for _, drift := range report.StateDrifts {
		if drift.Type == state.DriftTypeIssueState && drift.ExternalState == "closed" {
			return false
		}
	}

	// Can resume if we have a valid checkpoint
	if !workState.CheckpointedAt.IsZero() && workState.CheckpointStage != "" {
		return true
	}

	// Can resume early states
	if workState.State == state.StateClaim || workState.State == state.StateAnalyze {
		return true
	}

	// Can resume if workspace setup is complete
	if workState.State == state.StateWorkspace && workState.WorkspaceDir != "" {
		return true
	}

	return false
}

// assessResumptionRisk evaluates the risk of resuming work.
func (r *RecoveryManager) assessResumptionRisk(workState *state.AgentWorkState, report *state.ValidationReport) ResumptionRisk {
	riskScore := 0

	// Age factor
	age := time.Since(workState.UpdatedAt)
	if age > 12*time.Hour {
		riskScore += 2
	} else if age > 6*time.Hour {
		riskScore += 1
	}

	// Validation issues
	for _, issue := range report.IssuesFound {
		switch issue.Severity {
		case state.SeverityCritical:
			riskScore += 4
		case state.SeverityHigh:
			riskScore += 2
		case state.SeverityMedium:
			riskScore += 1
		}
	}

	// State drifts
	riskScore += len(report.StateDrifts)

	// Has error
	if workState.Error != "" {
		riskScore += 1
	}

	// Advanced states are riskier to resume
	if workState.State == state.StatePR || workState.State == state.StateValidation || workState.State == state.StateReview {
		riskScore += 2
	}

	// Convert score to risk level
	if riskScore >= 8 {
		return RiskCritical
	} else if riskScore >= 5 {
		return RiskHigh
	} else if riskScore >= 2 {
		return RiskMedium
	}
	return RiskLow
}

// determineResumeState determines which state to resume from.
func (r *RecoveryManager) determineResumeState(workState *state.AgentWorkState) state.WorkflowState {
	// If we have a checkpoint, use its stage
	if workState.CheckpointStage != "" {
		// Map checkpoint stages to states
		switch workState.CheckpointStage {
		case "analysis":
			return state.StateAnalyze
		case "workspace":
			return state.StateWorkspace
		case "implementation":
			return state.StateImplement
		case "commit":
			return state.StateCommit
		case "pr":
			return state.StatePR
		default:
			return workState.State
		}
	}

	// For early states, resume from current state
	if workState.State == state.StateClaim || workState.State == state.StateAnalyze {
		return workState.State
	}

	// For workspace state, might need to re-setup
	if workState.State == state.StateWorkspace {
		return state.StateWorkspace
	}

	// For later states, step back one state to be safe
	switch workState.State {
	case state.StateImplement:
		return state.StateWorkspace
	case state.StateCommit:
		return state.StateImplement
	case state.StatePR:
		return state.StateCommit
	case state.StateValidation:
		return state.StatePR
	case state.StateReview:
		return state.StateValidation
	}

	// Default to current state
	return workState.State
}

// estimateResumptionDuration estimates how long resumption might take.
func (r *RecoveryManager) estimateResumptionDuration(workState *state.AgentWorkState) time.Duration {
	// Base durations for each state (rough estimates)
	baseDurations := map[state.WorkflowState]time.Duration{
		state.StateClaim:      5 * time.Minute,
		state.StateAnalyze:    10 * time.Minute,
		state.StateWorkspace:  5 * time.Minute,
		state.StateImplement: 30 * time.Minute,
		state.StateCommit:     5 * time.Minute,
		state.StatePR:         10 * time.Minute,
		state.StateValidation: 15 * time.Minute,
		state.StateReview:     10 * time.Minute,
	}

	resumeFromState := r.determineResumeState(workState)
	baseDuration, exists := baseDurations[resumeFromState]
	if !exists {
		baseDuration = 20 * time.Minute // Default
	}

	// Add buffer for resumption overhead
	return baseDuration + 10*time.Minute
}

// generateCleanupActions generates required cleanup actions before resumption.
func (r *RecoveryManager) generateCleanupActions(workState *state.AgentWorkState, report *state.ValidationReport) []CleanupAction {
	var actions []CleanupAction

	// Cleanup actions based on validation issues
	for _, issue := range report.IssuesFound {
		switch issue.Type {
		case state.IssueTypeStaleWorkspace:
			actions = append(actions, CleanupAction{
				Type:        CleanupWorkspace,
				Description: "Clean up stale workspace directory",
				Required:    issue.Severity == state.SeverityCritical,
			})
		case state.IssueTypeBranchDrift:
			actions = append(actions, CleanupAction{
				Type:        CleanupBranch,
				Description: "Reconcile branch state",
				Required:    false,
			})
		case state.IssueTypeCheckpointCorrupt:
			actions = append(actions, CleanupAction{
				Type:        CleanupCheckpoint,
				Description: "Remove corrupted checkpoint",
				Required:    true,
			})
		}
	}

	// Cleanup actions based on state drifts
	for _, drift := range report.StateDrifts {
		if !drift.CanReconcile {
			actions = append(actions, CleanupAction{
				Type:        CleanupState,
				Description: fmt.Sprintf("Reset %s state drift", drift.Type),
				Required:    true,
			})
		}
	}

	return actions
}

// makeResumptionRecommendation makes the final recommendation for resumption.
func (r *RecoveryManager) makeResumptionRecommendation(workState *state.AgentWorkState, plan *ResumptionPlan, report *state.ValidationReport) (RecommendedResumption, string) {
	// Cannot resume if not eligible
	if !plan.CanResume {
		return RecommendCleanup, "Work state is not eligible for resumption"
	}

	// Critical risk requires manual intervention
	if plan.RiskLevel == RiskCritical {
		return RecommendManual, "Resumption risk is too high, manual review required"
	}

	// High risk suggests restart
	if plan.RiskLevel == RiskHigh {
		return RecommendRestart, "High risk resumption, restart recommended"
	}

	// Check for required cleanup actions
	hasRequiredCleanup := false
	for _, action := range plan.RequiredCleanup {
		if action.Required {
			hasRequiredCleanup = true
			break
		}
	}

	if hasRequiredCleanup {
		return RecommendCleanup, "Required cleanup actions must be completed first"
	}

	// Check if work is too old
	if time.Since(workState.UpdatedAt) > 12*time.Hour {
		return RecommendRestart, "Work is too old, restart recommended"
	}

	// Default recommendation based on risk
	if plan.RiskLevel == RiskMedium {
		return RecommendResume, "Medium risk resumption acceptable"
	}

	return RecommendResume, "Low risk resumption recommended"
}

// performFullCleanup performs full cleanup of orphaned work.
func (r *RecoveryManager) performFullCleanup(ctx context.Context, workState *state.AgentWorkState) error {
	r.logger.Info("performing full cleanup", 
		"agent_type", workState.AgentType,
		"issue_number", workState.IssueNumber)

	var errors []error

	// Clean up GitHub labels
	if workState.IssueNumber > 0 {
		if err := r.cleanupGitHubLabels(ctx, workState.IssueNumber); err != nil {
			errors = append(errors, fmt.Errorf("cleaning up GitHub labels: %w", err))
		}
	}

	// Clean up workspace
	if workState.WorkspaceDir != "" {
		if err := r.cleanupWorkspace(workState.WorkspaceDir); err != nil {
			errors = append(errors, fmt.Errorf("cleaning up workspace: %w", err))
		}
	}

	// Clean up checkpoint
	if !workState.CheckpointedAt.IsZero() {
		if err := r.cleanupCheckpoint(ctx, workState); err != nil {
			errors = append(errors, fmt.Errorf("cleaning up checkpoint: %w", err))
		}
	}

	// Delete agent state
	if err := r.deps.Store.Delete(ctx, workState.AgentType); err != nil {
		errors = append(errors, fmt.Errorf("deleting agent state: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup completed with %d errors: %v", len(errors), errors)
	}

	r.logger.Info("full cleanup completed successfully")
	return nil
}

// flagForManualIntervention flags work that requires manual intervention.
func (r *RecoveryManager) flagForManualIntervention(ctx context.Context, workState *state.AgentWorkState) error {
	r.logger.Info("flagging for manual intervention",
		"agent_type", workState.AgentType,
		"issue_number", workState.IssueNumber)

	// Add a comment to the issue explaining the situation
	comment := fmt.Sprintf("🚨 **Manual Intervention Required**\n\n" +
		"This issue has been flagged for manual review due to inconsistent agent state.\n\n" +
		"**Details:**\n" +
		"- Agent Type: %s\n" +
		"- Current State: %s\n" +
		"- Last Update: %s\n" +
		"- Age: %.1f hours\n\n" +
		"Please review the issue state and take appropriate action.",
		workState.AgentType,
		workState.State,
		workState.UpdatedAt.Format(time.RFC3339),
		time.Since(workState.UpdatedAt).Hours())

	if err := r.deps.GitHub.CreateComment(ctx, workState.IssueNumber, comment); err != nil {
		return fmt.Errorf("creating manual intervention comment: %w", err)
	}

	// Add a special label for manual review
	if err := r.deps.GitHub.AddLabels(ctx, workState.IssueNumber, []string{"agent:manual-review"}); err != nil {
		return fmt.Errorf("adding manual review label: %w", err)
	}

	return nil
}

// Helper cleanup methods

func (r *RecoveryManager) cleanupGitHubLabels(ctx context.Context, issueNumber int) error {
	// Remove claimed label and add ready label back
	if err := r.deps.GitHub.RemoveLabel(ctx, issueNumber, "agent:claimed"); err != nil {
		r.logger.Debug("could not remove claimed label", "error", err)
	}

	if err := r.deps.GitHub.AddLabels(ctx, issueNumber, []string{"agent:ready"}); err != nil {
		return fmt.Errorf("adding ready label: %w", err)
	}

	return nil
}

func (r *RecoveryManager) cleanupWorkspace(workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}

	r.logger.Info("cleaning up workspace", "workspace_dir", workspaceDir)

	// Remove workspace directory
	if err := os.RemoveAll(workspaceDir); err != nil {
		return fmt.Errorf("removing workspace directory: %w", err)
	}

	return nil
}

func (r *RecoveryManager) cleanupCheckpoint(ctx context.Context, workState *state.AgentWorkState) error {
	// Reset checkpoint fields
	workState.CheckpointedAt = time.Time{}
	workState.CheckpointStage = ""
	workState.CheckpointMetadata = nil
	workState.InterruptedBy = ""

	// Save the updated state
	if err := r.deps.Store.Save(ctx, workState); err != nil {
		return fmt.Errorf("saving state after checkpoint cleanup: %w", err)
	}

	return nil
}