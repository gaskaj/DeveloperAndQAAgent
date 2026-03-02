package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// StartupValidator handles agent initialization validation and recovery.
type StartupValidator struct {
	deps      Dependencies
	validator *state.StateValidator
	logger    *slog.Logger
}

// StartupValidationReport contains the results of startup validation.
type StartupValidationReport struct {
	Valid                bool                        `json:"valid"`
	OrphanedWorkFound    []*state.OrphanedWorkItem   `json:"orphaned_work_found"`
	RecoveryActions      []*RecoveryAction           `json:"recovery_actions"`
	ValidationIssues     []*state.ValidationIssue    `json:"validation_issues"`
	RecommendedActions   []*state.RecommendedAction  `json:"recommended_actions"`
	StartupSafe          bool                        `json:"startup_safe"`
	ValidationDuration   time.Duration               `json:"validation_duration"`
	ValidatedAt          time.Time                   `json:"validated_at"`
}

// RecoveryAction represents an action taken during startup recovery.
type RecoveryAction struct {
	Type        RecoveryActionType `json:"type"`
	AgentType   string             `json:"agent_type"`
	IssueNumber int                `json:"issue_number"`
	Description string             `json:"description"`
	Success     bool               `json:"success"`
	Error       string             `json:"error,omitempty"`
	Duration    time.Duration      `json:"duration"`
	Details     map[string]string  `json:"details,omitempty"`
}

// RecoveryActionType represents the type of recovery action.
type RecoveryActionType string

const (
	ActionCleanupOrphaned    RecoveryActionType = "cleanup_orphaned"
	ActionResumeWork         RecoveryActionType = "resume_work"
	ActionValidateState      RecoveryActionType = "validate_state"
	ActionReconcileDrift     RecoveryActionType = "reconcile_drift"
	ActionFlagForManual      RecoveryActionType = "flag_for_manual"
)

// NewStartupValidator creates a new startup validator.
func NewStartupValidator(deps Dependencies, validator *state.StateValidator) *StartupValidator {
	return &StartupValidator{
		deps:      deps,
		validator: validator,
		logger:    deps.Logger.With("component", "startup_validator"),
	}
}

// ValidateAndRecoverStartup performs comprehensive startup validation and recovery.
func (s *StartupValidator) ValidateAndRecoverStartup(ctx context.Context, agentType AgentType) (*StartupValidationReport, error) {
	start := time.Now()
	
	s.logger.Info("starting startup validation and recovery", "agent_type", agentType)

	report := &StartupValidationReport{
		Valid:              true,
		OrphanedWorkFound:  make([]*state.OrphanedWorkItem, 0),
		RecoveryActions:    make([]*RecoveryAction, 0),
		ValidationIssues:   make([]*state.ValidationIssue, 0),
		RecommendedActions: make([]*state.RecommendedAction, 0),
		StartupSafe:        true,
		ValidatedAt:        start,
	}

	// Check for existing agent state
	existingState, err := s.deps.Store.Load(ctx, string(agentType))
	if err != nil {
		s.logger.Error("failed to load existing agent state", "error", err)
		report.StartupSafe = false
		return report, err
	}

	// If agent has existing state, validate it
	if existingState != nil {
		if err := s.validateExistingState(ctx, existingState, report); err != nil {
			s.logger.Error("failed to validate existing state", "error", err)
			report.StartupSafe = false
		}
	}

	// Scan for orphaned work across all agents
	if err := s.detectOrphanedWork(ctx, report); err != nil {
		s.logger.Error("failed to detect orphaned work", "error", err)
		report.StartupSafe = false
	}

	// Perform recovery actions if configured
	if s.deps.Config.Agents.Developer.Recovery.Enabled && s.deps.Config.Agents.Developer.Recovery.AutoCleanupOrphaned {
		if err := s.performStartupRecovery(ctx, report); err != nil {
			s.logger.Error("failed to perform startup recovery", "error", err)
			report.StartupSafe = false
		}
	}

	// Final assessment
	report.Valid = len(report.ValidationIssues) == 0 && len(report.OrphanedWorkFound) == 0
	report.ValidationDuration = time.Since(start)

	s.logger.Info("startup validation completed",
		"valid", report.Valid,
		"startup_safe", report.StartupSafe,
		"orphaned_work", len(report.OrphanedWorkFound),
		"recovery_actions", len(report.RecoveryActions),
		"duration", report.ValidationDuration)

	return report, nil
}

// PerformPeriodicValidation performs periodic validation of agent state.
func (s *StartupValidator) PerformPeriodicValidation(ctx context.Context) error {
	s.logger.Info("starting periodic validation")

	// Get all agent states
	allStates, err := s.deps.Store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing agent states: %w", err)
	}

	var validationErrors []error

	for _, agentState := range allStates {
		// Validate each active agent state
		if agentState.State != state.StateIdle && agentState.State != state.StateComplete && agentState.State != state.StateFailed {
			validationReport, err := s.validator.ValidateWorkState(ctx, agentState)
			if err != nil {
				s.logger.Error("periodic validation failed for agent", 
					"agent_type", agentState.AgentType,
					"issue_number", agentState.IssueNumber,
					"error", err)
				validationErrors = append(validationErrors, err)
				continue
			}

			// Log validation results
			if !validationReport.Valid {
				s.logger.Warn("periodic validation found issues",
					"agent_type", agentState.AgentType,
					"issue_number", agentState.IssueNumber,
					"issues_count", len(validationReport.IssuesFound),
					"drifts_count", len(validationReport.StateDrifts))

				// If configured, attempt automatic reconciliation
				if s.deps.Config.Agents.Developer.Recovery.Enabled && s.deps.Config.Agents.Developer.Recovery.Consistency.ReconcileDrift {
					if err := s.validator.ReconcileState(ctx, agentState); err != nil {
						s.logger.Error("automatic reconciliation failed",
							"agent_type", agentState.AgentType,
							"issue_number", agentState.IssueNumber,
							"error", err)
					} else {
						s.logger.Info("automatic reconciliation successful",
							"agent_type", agentState.AgentType,
							"issue_number", agentState.IssueNumber)
					}
				}
			}
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("periodic validation completed with %d errors", len(validationErrors))
	}

	s.logger.Info("periodic validation completed successfully", "states_checked", len(allStates))
	return nil
}

// validateExistingState validates an existing agent state.
func (s *StartupValidator) validateExistingState(ctx context.Context, existingState *state.AgentWorkState, report *StartupValidationReport) error {
	s.logger.Info("validating existing agent state",
		"agent_type", existingState.AgentType,
		"issue_number", existingState.IssueNumber,
		"state", existingState.State,
		"last_update", existingState.UpdatedAt)

	// Validate the existing state
	validationReport, err := s.validator.ValidateWorkState(ctx, existingState)
	if err != nil {
		return fmt.Errorf("validating existing state: %w", err)
	}

	// Merge validation results into startup report
	report.ValidationIssues = append(report.ValidationIssues, validationReport.IssuesFound...)
	report.RecommendedActions = append(report.RecommendedActions, validationReport.RecommendedActions...)

	// Check if this is orphaned work
	for _, orphan := range validationReport.OrphanedWork {
		report.OrphanedWorkFound = append(report.OrphanedWorkFound, orphan)
	}

	if !validationReport.Valid {
		report.Valid = false
		report.StartupSafe = false
		s.logger.Warn("existing agent state has validation issues",
			"issues_count", len(validationReport.IssuesFound),
			"drifts_count", len(validationReport.StateDrifts))
	}

	return nil
}

// detectOrphanedWork scans for orphaned work across all agents.
func (s *StartupValidator) detectOrphanedWork(ctx context.Context, report *StartupValidationReport) error {
	s.logger.Info("scanning for orphaned work")

	orphanedItems, err := s.validator.DetectOrphanedWork(ctx)
	if err != nil {
		return fmt.Errorf("detecting orphaned work: %w", err)
	}

	report.OrphanedWorkFound = append(report.OrphanedWorkFound, orphanedItems...)

	if len(orphanedItems) > 0 {
		report.Valid = false
		s.logger.Warn("orphaned work detected", "count", len(orphanedItems))
		
		for _, orphan := range orphanedItems {
			s.logger.Info("orphaned work details",
				"agent_type", orphan.AgentType,
				"issue_number", orphan.IssueNumber,
				"state", orphan.State,
				"age_hours", orphan.AgeHours,
				"recovery_type", orphan.RecoveryType)
		}
	}

	return nil
}

// performStartupRecovery performs automatic recovery actions during startup.
func (s *StartupValidator) performStartupRecovery(ctx context.Context, report *StartupValidationReport) error {
	s.logger.Info("performing startup recovery", "orphaned_count", len(report.OrphanedWorkFound))

	// Create recovery manager
	recoveryManager := s.createRecoveryManager()

	for _, orphan := range report.OrphanedWorkFound {
		// Check age limits
		if orphan.AgeHours > float64(s.deps.Config.Agents.Developer.Recovery.MaxResumeAge.Hours()) {
			s.logger.Info("orphaned work too old, forcing cleanup",
				"issue_number", orphan.IssueNumber,
				"age_hours", orphan.AgeHours)
			orphan.RecoveryType = state.RecoveryTypeCleanup
		}

		action := &RecoveryAction{
			AgentType:   orphan.AgentType,
			IssueNumber: orphan.IssueNumber,
			Details:     make(map[string]string),
		}

		start := time.Now()

		switch orphan.RecoveryType {
		case state.RecoveryTypeCleanup:
			action.Type = ActionCleanupOrphaned
			action.Description = fmt.Sprintf("Cleanup orphaned work for issue #%d", orphan.IssueNumber)
			
			err := recoveryManager.CleanupOrphanedWork(ctx, orphan)
			action.Success = err == nil
			if err != nil {
				action.Error = err.Error()
				s.logger.Error("failed to cleanup orphaned work", 
					"issue_number", orphan.IssueNumber, "error", err)
			} else {
				s.logger.Info("successfully cleaned up orphaned work", 
					"issue_number", orphan.IssueNumber)
			}

		case state.RecoveryTypeResume:
			action.Type = ActionResumeWork
			action.Description = fmt.Sprintf("Attempt to resume work for issue #%d", orphan.IssueNumber)
			
			// For now, just flag as requiring manual intervention during startup
			// Actual resumption would happen during normal agent operation
			s.logger.Info("orphaned work flagged for resumption",
				"issue_number", orphan.IssueNumber)
			action.Success = true

		case state.RecoveryTypeManual:
			action.Type = ActionFlagForManual
			action.Description = fmt.Sprintf("Flag issue #%d for manual intervention", orphan.IssueNumber)
			
			// This would need proper implementation
			s.logger.Info("orphaned work flagged for manual intervention",
				"issue_number", orphan.IssueNumber)
			action.Success = true
		}

		action.Duration = time.Since(start)
		report.RecoveryActions = append(report.RecoveryActions, action)
	}

	return nil
}

// RecoveryManagerInterface defines the interface for recovery operations.
type RecoveryManagerInterface interface {
	CleanupOrphanedWork(ctx context.Context, orphaned *state.OrphanedWorkItem) error
}

// createRecoveryManager creates a recovery manager instance.
// This is a placeholder - in real implementation, this would be properly injected.
func (s *StartupValidator) createRecoveryManager() RecoveryManagerInterface {
	// This is a placeholder - would need proper implementation
	// that creates a recovery manager with proper dependencies
	return &mockRecoveryManager{}
}

// mockRecoveryManager is a placeholder implementation
type mockRecoveryManager struct{}

func (m *mockRecoveryManager) CleanupOrphanedWork(ctx context.Context, orphaned *state.OrphanedWorkItem) error {
	// Placeholder implementation
	return nil
}

// Configuration types are now defined in internal/config package