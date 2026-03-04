package state

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
)

// StateValidator provides comprehensive state consistency validation and reconciliation.
type StateValidator struct {
	store  Store
	github ghub.Client
	logger *slog.Logger
}

// ValidationReport contains the results of a state consistency validation.
type ValidationReport struct {
	Valid              bool                    `json:"valid"`
	IssuesFound        []*ValidationIssue      `json:"issues_found"`
	OrphanedWork       []*OrphanedWorkItem     `json:"orphaned_work"`
	StateDrifts        []*StateDrift           `json:"state_drifts"`
	RecommendedActions []*RecommendedAction    `json:"recommended_actions"`
	ValidatedAt        time.Time               `json:"validated_at"`
	ValidationDuration time.Duration           `json:"validation_duration"`
}

// ValidationIssue represents a specific consistency problem found during validation.
type ValidationIssue struct {
	Type        ValidationIssueType `json:"type"`
	Severity    ValidationSeverity  `json:"severity"`
	Description string              `json:"description"`
	AgentType   string              `json:"agent_type,omitempty"`
	IssueNumber int                 `json:"issue_number,omitempty"`
	Details     map[string]string   `json:"details,omitempty"`
}

// OrphanedWorkItem represents work that has been abandoned or is inconsistent.
type OrphanedWorkItem struct {
	AgentType     string            `json:"agent_type"`
	IssueNumber   int               `json:"issue_number"`
	State         WorkflowState     `json:"state"`
	LastUpdate    time.Time         `json:"last_update"`
	AgeHours      float64           `json:"age_hours"`
	BranchName    string            `json:"branch_name,omitempty"`
	WorkspaceDir  string            `json:"workspace_dir,omitempty"`
	PRNumber      int               `json:"pr_number,omitempty"`
	RecoveryType  OrphanRecoveryType `json:"recovery_type"`
	Details       map[string]string `json:"details,omitempty"`
}

// StateDrift represents inconsistency between local state and external systems.
type StateDrift struct {
	Type          StateDriftType    `json:"type"`
	AgentType     string            `json:"agent_type"`
	IssueNumber   int               `json:"issue_number"`
	LocalState    WorkflowState     `json:"local_state"`
	ExternalState string            `json:"external_state"`
	Description   string            `json:"description"`
	CanReconcile  bool              `json:"can_reconcile"`
	Details       map[string]string `json:"details,omitempty"`
}

// RecommendedAction represents an action that should be taken to resolve inconsistencies.
type RecommendedAction struct {
	Type        ActionType        `json:"type"`
	Priority    ActionPriority    `json:"priority"`
	Description string            `json:"description"`
	AgentType   string            `json:"agent_type,omitempty"`
	IssueNumber int               `json:"issue_number,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

// Enum types for validation issues
type ValidationIssueType string

const (
	IssueTypeOrphanedClaim      ValidationIssueType = "orphaned_claim"
	IssueTypeInconsistentState  ValidationIssueType = "inconsistent_state"
	IssueTypeStaleWorkspace     ValidationIssueType = "stale_workspace"
	IssueTypeBranchDrift        ValidationIssueType = "branch_drift"
	IssueTypePRInconsistency    ValidationIssueType = "pr_inconsistency"
	IssueTypeCheckpointCorrupt  ValidationIssueType = "checkpoint_corrupt"
)

type ValidationSeverity string

const (
	SeverityCritical ValidationSeverity = "critical"
	SeverityHigh     ValidationSeverity = "high"
	SeverityMedium   ValidationSeverity = "medium"
	SeverityLow      ValidationSeverity = "low"
)

type OrphanRecoveryType string

const (
	RecoveryTypeResume  OrphanRecoveryType = "resume"
	RecoveryTypeCleanup OrphanRecoveryType = "cleanup"
	RecoveryTypeManual  OrphanRecoveryType = "manual"
)

type StateDriftType string

const (
	DriftTypeIssueState  StateDriftType = "issue_state"
	DriftTypeBranchState StateDriftType = "branch_state"
	DriftTypePRState     StateDriftType = "pr_state"
	DriftTypeGitState    StateDriftType = "git_state"
)

type ActionType string

const (
	ActionTypeCleanup    ActionType = "cleanup"
	ActionTypeResume     ActionType = "resume"
	ActionTypeReconcile  ActionType = "reconcile"
	ActionTypeReset      ActionType = "reset"
	ActionTypeManualFix  ActionType = "manual_fix"
)

type ActionPriority string

const (
	PriorityUrgent ActionPriority = "urgent"
	PriorityHigh   ActionPriority = "high"
	PriorityMedium ActionPriority = "medium"
	PriorityLow    ActionPriority = "low"
)

// Validator defines the interface for state validation operations.
// This interface allows test mocking of the StateValidator.
type Validator interface {
	ValidateWorkState(ctx context.Context, workState *AgentWorkState) (*ValidationReport, error)
	DetectOrphanedWork(ctx context.Context) ([]*OrphanedWorkItem, error)
	ReconcileState(ctx context.Context, workState *AgentWorkState) error
}

// NewStateValidator creates a new state validator.
func NewStateValidator(store Store, github ghub.Client, logger *slog.Logger) *StateValidator {
	return &StateValidator{
		store:  store,
		github: github,
		logger: logger.With("component", "state_validator"),
	}
}

// ValidateWorkState validates the consistency of a specific agent work state.
func (v *StateValidator) ValidateWorkState(ctx context.Context, workState *AgentWorkState) (*ValidationReport, error) {
	start := time.Now()
	
	v.logger.Info("validating work state consistency", 
		"agent_type", workState.AgentType, 
		"issue_number", workState.IssueNumber,
		"state", workState.State)

	report := &ValidationReport{
		Valid:              true,
		IssuesFound:        make([]*ValidationIssue, 0),
		OrphanedWork:       make([]*OrphanedWorkItem, 0),
		StateDrifts:        make([]*StateDrift, 0),
		RecommendedActions: make([]*RecommendedAction, 0),
		ValidatedAt:        start,
	}

	// Validate issue state consistency
	if err := v.validateIssueStateConsistency(ctx, workState, report); err != nil {
		v.logger.Error("failed to validate issue state consistency", "error", err)
		// Continue with other validations
	}

	// Validate branch consistency
	if workState.BranchName != "" {
		if err := v.validateBranchConsistency(ctx, workState, report); err != nil {
			v.logger.Error("failed to validate branch consistency", "error", err)
		}
	}

	// Validate PR consistency
	if workState.PRNumber > 0 {
		if err := v.validatePRConsistency(ctx, workState, report); err != nil {
			v.logger.Error("failed to validate PR consistency", "error", err)
		}
	}

	// Validate workspace consistency
	if workState.WorkspaceDir != "" {
		if err := v.validateWorkspaceConsistency(ctx, workState, report); err != nil {
			v.logger.Error("failed to validate workspace consistency", "error", err)
		}
	}

	// Check for orphaned work
	if err := v.checkOrphanedWork(ctx, workState, report); err != nil {
		v.logger.Error("failed to check for orphaned work", "error", err)
	}

	// Generate recommended actions
	v.generateRecommendedActions(report)

	report.Valid = len(report.IssuesFound) == 0
	report.ValidationDuration = time.Since(start)

	v.logger.Info("validation completed", 
		"valid", report.Valid,
		"issues_found", len(report.IssuesFound),
		"duration", report.ValidationDuration)

	return report, nil
}

// DetectOrphanedWork scans all stored states to find orphaned work items.
func (v *StateValidator) DetectOrphanedWork(ctx context.Context) ([]*OrphanedWorkItem, error) {
	v.logger.Info("scanning for orphaned work items")
	
	states, err := v.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing agent states: %w", err)
	}

	var orphaned []*OrphanedWorkItem

	for _, state := range states {
		if v.isOrphanedWork(state) {
			orphanItem := &OrphanedWorkItem{
				AgentType:    state.AgentType,
				IssueNumber:  state.IssueNumber,
				State:        state.State,
				LastUpdate:   state.UpdatedAt,
				AgeHours:     time.Since(state.UpdatedAt).Hours(),
				BranchName:   state.BranchName,
				WorkspaceDir: state.WorkspaceDir,
				PRNumber:     state.PRNumber,
				RecoveryType: v.determineRecoveryType(state),
				Details:      make(map[string]string),
			}

			// Add contextual details
			if state.Error != "" {
				orphanItem.Details["last_error"] = state.Error
			}
			if state.CheckpointStage != "" {
				orphanItem.Details["checkpoint_stage"] = state.CheckpointStage
			}

			orphaned = append(orphaned, orphanItem)
		}
	}

	// Sort by age (oldest first)
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].LastUpdate.Before(orphaned[j].LastUpdate)
	})

	v.logger.Info("orphaned work scan completed", "count", len(orphaned))
	return orphaned, nil
}

// ReconcileState attempts to reconcile inconsistencies in the work state.
func (v *StateValidator) ReconcileState(ctx context.Context, workState *AgentWorkState) error {
	v.logger.Info("reconciling work state", 
		"agent_type", workState.AgentType,
		"issue_number", workState.IssueNumber)

	// Generate validation report first
	report, err := v.ValidateWorkState(ctx, workState)
	if err != nil {
		return fmt.Errorf("validating state before reconciliation: %w", err)
	}

	if report.Valid {
		v.logger.Info("state is already consistent, no reconciliation needed")
		return nil
	}

	// Execute high-priority reconciliation actions
	var reconcileErrors []error
	for _, action := range report.RecommendedActions {
		if action.Priority == PriorityUrgent || action.Priority == PriorityHigh {
			if err := v.executeReconciliationAction(ctx, action, workState); err != nil {
				v.logger.Error("failed to execute reconciliation action", 
					"action_type", action.Type,
					"error", err)
				reconcileErrors = append(reconcileErrors, err)
			}
		}
	}

	if len(reconcileErrors) > 0 {
		return fmt.Errorf("reconciliation completed with %d errors", len(reconcileErrors))
	}

	v.logger.Info("state reconciliation completed successfully")
	return nil
}

// validateIssueStateConsistency validates consistency between local state and GitHub issue state.
func (v *StateValidator) validateIssueStateConsistency(ctx context.Context, workState *AgentWorkState, report *ValidationReport) error {
	if workState.IssueNumber <= 0 {
		return nil // No issue to validate
	}

	issue, err := v.github.GetIssue(ctx, workState.IssueNumber)
	if err != nil {
		v.addValidationIssue(report, IssueTypeInconsistentState, SeverityCritical,
			fmt.Sprintf("Cannot fetch issue #%d from GitHub", workState.IssueNumber),
			workState.AgentType, workState.IssueNumber, 
			map[string]string{"error": err.Error()})
		return nil // Continue validation
	}

	// Check if issue still exists and is accessible
	if issue.GetState() == "closed" && workState.State != StateFailed && workState.State != StateComplete {
		v.addStateDrift(report, DriftTypeIssueState, workState.AgentType, workState.IssueNumber,
			workState.State, "closed", "Issue was closed but agent state indicates active work", true)
	}

	// Check label consistency for claimed issues
	if workState.State != StateIdle && workState.State != StateComplete && workState.State != StateFailed {
		hasClaimedLabel := false
		for _, label := range issue.Labels {
			if label.GetName() == "agent:claimed" {
				hasClaimedLabel = true
				break
			}
		}
		if !hasClaimedLabel {
			v.addStateDrift(report, DriftTypeIssueState, workState.AgentType, workState.IssueNumber,
				workState.State, "not_claimed", "Issue lacks 'agent:claimed' label but agent is working on it", true)
		}
	}

	return nil
}

// validateBranchConsistency validates branch existence and consistency.
func (v *StateValidator) validateBranchConsistency(ctx context.Context, workState *AgentWorkState, report *ValidationReport) error {
	// For now, just validate branch name pattern
	// Full branch validation would require additional GitHub API calls
	
	// Validate branch name follows expected pattern
	expectedBranch := fmt.Sprintf("agent/issue-%d", workState.IssueNumber)
	if workState.BranchName != expectedBranch {
		v.addValidationIssue(report, IssueTypeBranchDrift, SeverityMedium,
			fmt.Sprintf("Branch name %s doesn't follow expected pattern %s", workState.BranchName, expectedBranch),
			workState.AgentType, workState.IssueNumber,
			map[string]string{
				"actual_branch": workState.BranchName,
				"expected_branch": expectedBranch,
			})
	}

	return nil
}

// validatePRConsistency validates PR existence and consistency.
func (v *StateValidator) validatePRConsistency(ctx context.Context, workState *AgentWorkState, report *ValidationReport) error {
	pr, err := v.github.GetPR(ctx, workState.PRNumber)
	if err != nil {
		v.addValidationIssue(report, IssueTypePRInconsistency, SeverityCritical,
			fmt.Sprintf("Cannot fetch PR #%d from GitHub", workState.PRNumber),
			workState.AgentType, workState.IssueNumber,
			map[string]string{"pr_number": fmt.Sprintf("%d", workState.PRNumber)})
		return nil
	}

	// Check PR state consistency
	prState := pr.GetState()
	if prState == "closed" && workState.State != StateComplete && workState.State != StateFailed {
		v.addStateDrift(report, DriftTypePRState, workState.AgentType, workState.IssueNumber,
			workState.State, prState, "PR was closed but agent state indicates active work", true)
	}

	// Check if PR references the correct issue
	if pr.GetBody() != "" {
		expectedRef := fmt.Sprintf("Closes #%d", workState.IssueNumber)
		if !contains(pr.GetBody(), expectedRef) {
			v.addValidationIssue(report, IssueTypePRInconsistency, SeverityMedium,
				fmt.Sprintf("PR #%d doesn't reference issue #%d", workState.PRNumber, workState.IssueNumber),
				workState.AgentType, workState.IssueNumber,
				map[string]string{"pr_number": fmt.Sprintf("%d", workState.PRNumber)})
		}
	}

	return nil
}

// validateWorkspaceConsistency validates workspace directory and contents.
func (v *StateValidator) validateWorkspaceConsistency(ctx context.Context, workState *AgentWorkState, report *ValidationReport) error {
	// This is a placeholder - in a real implementation, we'd check:
	// - Directory exists and is accessible
	// - Git repository is properly initialized
	// - Correct branch is checked out
	// - Working directory is clean or has expected changes
	
	// For now, just add a basic check
	v.logger.Debug("workspace consistency validation placeholder",
		"workspace_dir", workState.WorkspaceDir,
		"agent_type", workState.AgentType)
	
	return nil
}

// checkOrphanedWork determines if the work state represents orphaned work.
func (v *StateValidator) checkOrphanedWork(ctx context.Context, workState *AgentWorkState, report *ValidationReport) error {
	if v.isOrphanedWork(workState) {
		orphanItem := &OrphanedWorkItem{
			AgentType:    workState.AgentType,
			IssueNumber:  workState.IssueNumber,
			State:        workState.State,
			LastUpdate:   workState.UpdatedAt,
			AgeHours:     time.Since(workState.UpdatedAt).Hours(),
			BranchName:   workState.BranchName,
			WorkspaceDir: workState.WorkspaceDir,
			PRNumber:     workState.PRNumber,
			RecoveryType: v.determineRecoveryType(workState),
			Details:      make(map[string]string),
		}

		if workState.Error != "" {
			orphanItem.Details["last_error"] = workState.Error
		}

		report.OrphanedWork = append(report.OrphanedWork, orphanItem)
	}

	return nil
}

// isOrphanedWork determines if a work state represents orphaned work.
func (v *StateValidator) isOrphanedWork(state *AgentWorkState) bool {
	// Consider work orphaned if:
	// 1. It's been more than 1 hour since last update and not in a terminal state
	// 2. It has an error and hasn't been updated recently
	// 3. It's stuck in a non-idle state for too long

	now := time.Now()
	age := now.Sub(state.UpdatedAt)

	// Terminal states are not orphaned
	if state.State == StateComplete || state.State == StateFailed || state.State == StateIdle {
		return false
	}

	// More than 1 hour without update is suspicious
	if age > time.Hour {
		return true
	}

	// Has error and old enough
	if state.Error != "" && age > 30*time.Minute {
		return true
	}

	// Stuck in intermediate state too long
	if (state.State == StateClaim || state.State == StateAnalyze) && age > 30*time.Minute {
		return true
	}

	return false
}

// determineRecoveryType determines the appropriate recovery type for orphaned work.
func (v *StateValidator) determineRecoveryType(state *AgentWorkState) OrphanRecoveryType {
	// If there's a checkpoint and it's recent, try to resume
	if !state.CheckpointedAt.IsZero() && time.Since(state.CheckpointedAt) < 2*time.Hour {
		return RecoveryTypeResume
	}

	// If work is far along (has PR), might need manual intervention
	if state.PRNumber > 0 {
		return RecoveryTypeManual
	}

	// Early stages can usually be cleaned up safely
	if state.State == StateClaim || state.State == StateAnalyze || state.State == StateWorkspace {
		return RecoveryTypeCleanup
	}

	// Default to resume for intermediate states
	return RecoveryTypeResume
}

// generateRecommendedActions generates recommended actions based on validation results.
func (v *StateValidator) generateRecommendedActions(report *ValidationReport) {
	// Generate actions for orphaned work
	for _, orphan := range report.OrphanedWork {
		switch orphan.RecoveryType {
		case RecoveryTypeCleanup:
			report.RecommendedActions = append(report.RecommendedActions, &RecommendedAction{
				Type:        ActionTypeCleanup,
				Priority:    PriorityHigh,
				Description: fmt.Sprintf("Clean up orphaned work for issue #%d", orphan.IssueNumber),
				AgentType:   orphan.AgentType,
				IssueNumber: orphan.IssueNumber,
			})
		case RecoveryTypeResume:
			report.RecommendedActions = append(report.RecommendedActions, &RecommendedAction{
				Type:        ActionTypeResume,
				Priority:    PriorityMedium,
				Description: fmt.Sprintf("Resume orphaned work for issue #%d", orphan.IssueNumber),
				AgentType:   orphan.AgentType,
				IssueNumber: orphan.IssueNumber,
			})
		case RecoveryTypeManual:
			report.RecommendedActions = append(report.RecommendedActions, &RecommendedAction{
				Type:        ActionTypeManualFix,
				Priority:    PriorityUrgent,
				Description: fmt.Sprintf("Manual intervention required for issue #%d", orphan.IssueNumber),
				AgentType:   orphan.AgentType,
				IssueNumber: orphan.IssueNumber,
			})
		}
	}

	// Generate actions for state drifts
	for _, drift := range report.StateDrifts {
		if drift.CanReconcile {
			report.RecommendedActions = append(report.RecommendedActions, &RecommendedAction{
				Type:        ActionTypeReconcile,
				Priority:    PriorityHigh,
				Description: fmt.Sprintf("Reconcile %s drift for issue #%d", drift.Type, drift.IssueNumber),
				AgentType:   drift.AgentType,
				IssueNumber: drift.IssueNumber,
			})
		}
	}
}

// executeReconciliationAction executes a specific reconciliation action.
func (v *StateValidator) executeReconciliationAction(ctx context.Context, action *RecommendedAction, workState *AgentWorkState) error {
	v.logger.Info("executing reconciliation action",
		"action_type", action.Type,
		"agent_type", action.AgentType,
		"issue_number", action.IssueNumber)

	switch action.Type {
	case ActionTypeReconcile:
		return v.executeReconcileAction(ctx, action, workState)
	case ActionTypeReset:
		return v.executeResetAction(ctx, action, workState)
	case ActionTypeCleanup:
		v.logger.Info("cleanup action identified - will be handled by recovery manager")
		return nil
	default:
		v.logger.Warn("unsupported reconciliation action", "action_type", action.Type)
		return nil
	}
}

// executeReconcileAction handles reconciliation of state drift.
func (v *StateValidator) executeReconcileAction(ctx context.Context, action *RecommendedAction, workState *AgentWorkState) error {
	// Add/remove labels as needed
	if workState.State != StateIdle && workState.State != StateComplete && workState.State != StateFailed {
		// Ensure issue is properly claimed
		if err := v.github.AddLabels(ctx, workState.IssueNumber, []string{"agent:claimed"}); err != nil {
			return fmt.Errorf("adding claimed label: %w", err)
		}
		
		// Remove ready label if present
		if err := v.github.RemoveLabel(ctx, workState.IssueNumber, "agent:ready"); err != nil {
			// Ignore error if label doesn't exist
			v.logger.Debug("could not remove ready label", "error", err)
		}
	}

	return nil
}

// executeResetAction handles resetting state to a consistent state.
func (v *StateValidator) executeResetAction(ctx context.Context, action *RecommendedAction, workState *AgentWorkState) error {
	// Reset to idle state and remove claimed label
	workState.State = StateIdle
	workState.UpdatedAt = time.Now()
	
	if err := v.store.Save(ctx, workState); err != nil {
		return fmt.Errorf("saving reset state: %w", err)
	}
	
	// Remove claimed label and add ready label
	if err := v.github.RemoveLabel(ctx, workState.IssueNumber, "agent:claimed"); err != nil {
		v.logger.Debug("could not remove claimed label", "error", err)
	}
	
	if err := v.github.AddLabels(ctx, workState.IssueNumber, []string{"agent:ready"}); err != nil {
		return fmt.Errorf("adding ready label: %w", err)
	}

	return nil
}

// Helper methods for building validation reports

func (v *StateValidator) addValidationIssue(report *ValidationReport, issueType ValidationIssueType, severity ValidationSeverity, description, agentType string, issueNumber int, details map[string]string) {
	issue := &ValidationIssue{
		Type:        issueType,
		Severity:    severity,
		Description: description,
		AgentType:   agentType,
		IssueNumber: issueNumber,
		Details:     details,
	}
	report.IssuesFound = append(report.IssuesFound, issue)
}

func (v *StateValidator) addStateDrift(report *ValidationReport, driftType StateDriftType, agentType string, issueNumber int, localState WorkflowState, externalState, description string, canReconcile bool) {
	drift := &StateDrift{
		Type:          driftType,
		AgentType:     agentType,
		IssueNumber:   issueNumber,
		LocalState:    localState,
		ExternalState: externalState,
		Description:   description,
		CanReconcile:  canReconcile,
	}
	report.StateDrifts = append(report.StateDrifts, drift)
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}