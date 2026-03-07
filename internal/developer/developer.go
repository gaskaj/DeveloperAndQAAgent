package developer

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/creativity"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/memory"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/gaskaj/OpenAgentFramework/internal/workspace"
	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
)

// DeveloperAgent monitors GitHub for issues and implements solutions.
type DeveloperAgent struct {
	agent.BaseAgent
	poller           *ghub.Poller
	status           agent.StatusReport
	workspaceManager workspace.Manager
	validator        *state.StateValidator
	recoveryManager  *RecoveryManager
	memoryStore      *memory.Store
}

// New creates a new DeveloperAgent.
func New(deps agent.Dependencies) (agent.Agent, error) {
	// Create workspace manager configuration
	// Use repo-specific workspace path to prevent collisions when running multiple agents
	repoWorkspaceDir := deps.Config.GetWorkspacePath(deps.Config.Agents.Developer.WorkspaceDir)
	workspaceConfig := workspace.ManagerConfig{
		BaseDir:           repoWorkspaceDir,
		MaxSizeMB:         deps.Config.Workspace.Limits.MaxSizeMB,
		MinFreeDiskMB:     deps.Config.Workspace.Limits.MinFreeDiskMB,
		MaxConcurrent:     deps.Config.Workspace.Cleanup.MaxConcurrent,
		SuccessRetention:  deps.Config.Workspace.Cleanup.SuccessRetention,
		FailureRetention:  deps.Config.Workspace.Cleanup.FailureRetention,
		DiskCheckInterval: deps.Config.Workspace.Monitoring.DiskCheckInterval,
		CleanupInterval:   deps.Config.Workspace.Monitoring.CleanupInterval,
		CleanupEnabled:    deps.Config.Workspace.Cleanup.Enabled,
	}

	// Use defaults for any zero values
	if workspaceConfig.MaxSizeMB == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.MaxSizeMB = defaultConfig.MaxSizeMB
	}
	if workspaceConfig.MinFreeDiskMB == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.MinFreeDiskMB = defaultConfig.MinFreeDiskMB
	}
	if workspaceConfig.MaxConcurrent == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.MaxConcurrent = defaultConfig.MaxConcurrent
	}
	if workspaceConfig.SuccessRetention == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.SuccessRetention = defaultConfig.SuccessRetention
	}
	if workspaceConfig.FailureRetention == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.FailureRetention = defaultConfig.FailureRetention
	}
	if workspaceConfig.DiskCheckInterval == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.DiskCheckInterval = defaultConfig.DiskCheckInterval
	}
	if workspaceConfig.CleanupInterval == 0 {
		defaultConfig := workspace.DefaultConfig()
		workspaceConfig.CleanupInterval = defaultConfig.CleanupInterval
	}

	// Create workspace manager
	workspaceManager, err := workspace.NewManager(workspaceConfig, deps.Logger)
	if err != nil {
		return nil, fmt.Errorf("creating workspace manager: %w", err)
	}

	// Initialize repo memory store if enabled
	var memStore *memory.Store
	if deps.Config.Memory.Enabled {
		memoryDir := filepath.Join(repoWorkspaceDir, ".memory")
		var memErr error
		memStore, memErr = memory.NewStore(memoryDir)
		if memErr != nil {
			deps.Logger.Warn("failed to initialize memory store, continuing without memory", "error", memErr)
		} else {
			deps.Logger.Info("repo memory initialized", "dir", memoryDir, "entries", memStore.Count())
		}
	}

	da := &DeveloperAgent{
		BaseAgent:        agent.NewBaseAgent(deps),
		workspaceManager: workspaceManager,
		memoryStore:      memStore,
		status: agent.StatusReport{
			Type:    agent.TypeDeveloper,
			State:   string(state.StateIdle),
			Message: "waiting for issues",
		},
	}

	da.poller = ghub.NewPoller(
		deps.GitHub,
		deps.Config.GitHub.WatchLabels,
		deps.Config.GitHub.PollInterval,
		da.handleIssues,
		deps.Logger,
	)

	// Build the idle handler chain: auto-issue-processing first, then creativity.
	var idleHandlers []func(ctx context.Context) error

	// Auto-issue processing: promote a suggestion to agent:ready when idle.
	if deps.Config.Agents.Developer.AllowAutoIssueProcessing {
		idleHandlers = append(idleHandlers, da.promoteOneSuggestion)
	}

	// Wire up creativity engine as idle handler when enabled.
	if deps.Config.Creativity.Enabled {
		ghAdapter := creativity.NewGitHubAdapter(deps.GitHub)
		aiAdapter := creativity.NewClaudeAdapter(deps.Claude)
		repoCfg := creativity.RepoConfig{
			URL:          fmt.Sprintf("https://github.com/%s/%s.git", deps.Config.GitHub.Owner, deps.Config.GitHub.Repo),
			Token:        deps.Config.GitHub.Token,
			WorkspaceDir: repoWorkspaceDir,
		}
		engine := creativity.NewCreativityEngine(
			ghAdapter,
			aiAdapter,
			deps.Config.Creativity,
			repoCfg,
			string(agent.TypeDeveloper),
			deps.Logger.With("component", "creativity"),
			memStore,
		)

		idleHandlers = append(idleHandlers, func(ctx context.Context) error {
			da.updateStatus(state.StateCreativeThink, 0, "generating improvement suggestions")
			defer da.updateStatus(state.StateIdle, 0, "waiting for issues")
			da.reportEvent(ctx, apitypes.AgentEvent{
				EventType:     apitypes.EventSuggestionCreated,
				Severity:      apitypes.SeverityInfo,
				WorkflowState: "creative_thinking",
				Payload:       map[string]any{"action": "creativity_cycle_started"},
				Timestamp:     time.Now(),
			})
			return engine.Run(ctx)
		})
	}

	if len(idleHandlers) > 0 {
		da.poller.IdleHandler = func(ctx context.Context) error {
			for _, handler := range idleHandlers {
				if err := handler(ctx); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Create state validator and recovery manager
	da.validator = state.NewStateValidator(deps.Store, deps.GitHub, deps.Logger)
	da.recoveryManager = NewRecoveryManager(deps, da.validator)

	// Perform startup validation if enabled
	if deps.Config.Agents.Developer.Recovery.Enabled && deps.Config.Agents.Developer.Recovery.StartupValidation {
		startupValidator := NewStartupValidator(deps, da.validator)
		report, err := startupValidator.ValidateAndRecoverStartup(context.Background(), agent.TypeDeveloper)
		if err != nil {
			deps.Logger.Error("startup validation failed", "error", err)
		} else {
			deps.Logger.Info("startup validation completed",
				"valid", report.Valid,
				"startup_safe", report.StartupSafe,
				"orphaned_count", len(report.OrphanedWorkFound),
				"recovery_actions", len(report.RecoveryActions))
		}
	}

	return da, nil
}

// NewStartupValidator creates a startup validator - wrapper function.
func NewStartupValidator(deps agent.Dependencies, validator state.Validator) *StartupValidator {
	return &StartupValidator{
		deps:      deps,
		validator: validator,
		logger:    deps.Logger.With("component", "startup_validator"),
	}
}

// StartupValidator handles agent initialization validation and recovery.
type StartupValidator struct {
	deps      agent.Dependencies
	validator state.Validator
	logger    *slog.Logger
}

// ValidateAndRecoverStartup performs comprehensive startup validation and recovery.
func (s *StartupValidator) ValidateAndRecoverStartup(ctx context.Context, agentType agent.AgentType) (*StartupValidationReport, error) {
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

// StartupValidationReport contains the results of startup validation.
type StartupValidationReport struct {
	Valid              bool                       `json:"valid"`
	OrphanedWorkFound  []*state.OrphanedWorkItem  `json:"orphaned_work_found"`
	RecoveryActions    []*RecoveryAction          `json:"recovery_actions"`
	ValidationIssues   []*state.ValidationIssue   `json:"validation_issues"`
	RecommendedActions []*state.RecommendedAction `json:"recommended_actions"`
	StartupSafe        bool                       `json:"startup_safe"`
	ValidationDuration time.Duration              `json:"validation_duration"`
	ValidatedAt        time.Time                  `json:"validated_at"`
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
	ActionCleanupOrphaned RecoveryActionType = "cleanup_orphaned"
	ActionResumeWork      RecoveryActionType = "resume_work"
	ActionValidateState   RecoveryActionType = "validate_state"
	ActionReconcileDrift  RecoveryActionType = "reconcile_drift"
	ActionFlagForManual   RecoveryActionType = "flag_for_manual"
)

// Type returns the developer agent type.
func (d *DeveloperAgent) Type() agent.AgentType {
	return agent.TypeDeveloper
}

// Run starts the developer agent's polling loop.
func (d *DeveloperAgent) Run(ctx context.Context) error {
	// Ensure enriched correlation context for this agent run
	ctx = observability.EnsureCorrelationContext(ctx, string(d.Type()), 0)

	// Log agent start with structured logging
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogAgentStart(ctx, string(d.Type()), "developer agent started")
	}
	d.Deps.Logger.Info("developer agent started")

	// Start timing for agent lifecycle
	var timer *observability.Timer
	if d.Deps.Metrics != nil {
		timer = d.Deps.Metrics.Timer("agent_lifecycle", map[string]string{
			"agent_type": string(d.Type()),
		})
	}

	// Start heartbeat in background.
	go agent.Heartbeat(ctx, d.Type(), 60*time.Second, d.Deps.Logger)

	err := d.poller.Run(ctx)

	// Log agent stop with metrics
	var duration time.Duration
	if timer != nil {
		duration = timer.StopWithContext(ctx, "agent_lifecycle")
	}

	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogAgentStop(ctx, string(d.Type()), duration, err)
	}

	return err
}

// Status returns the current agent status.
func (d *DeveloperAgent) Status() agent.StatusReport {
	status := d.status

	// Add workspace statistics
	if stats, err := d.workspaceManager.GetWorkspaceStats(context.Background()); err == nil {
		status.WorkspaceStats = &agent.WorkspaceStats{
			TotalWorkspaces:  stats.TotalWorkspaces,
			ActiveWorkspaces: stats.ActiveWorkspaces,
			TotalSizeMB:      stats.TotalSizeMB,
			DiskFreeMB:       stats.DiskFreeMB,
		}
	}

	return status
}

func (d *DeveloperAgent) updateStatus(s state.WorkflowState, issueID int, msg string) {
	d.status = agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(s),
		IssueID: issueID,
		Message: msg,
	}
}

func (d *DeveloperAgent) logger() *slog.Logger {
	return d.Deps.Logger.With("agent", "developer")
}

// promoteOneSuggestion finds one issue with only the agent:suggestion label
// (no agent:failed, no agent:suggestion-rejected) and adds agent:ready to it.
// This allows the agent to autonomously process its own suggestions when idle.
func (d *DeveloperAgent) promoteOneSuggestion(ctx context.Context) error {
	// First check if any agents are currently working (not idle)
	states, err := d.Deps.Store.List(ctx)
	if err != nil {
		d.logger().Debug("failed to list agent states for auto-processing check", "error", err)
		return nil // Don't fail, just skip
	}
	for _, ws := range states {
		if ws.State != state.StateIdle && ws.State != state.StateComplete && ws.State != state.StateFailed {
			d.logger().Debug("agents are busy, skipping auto-issue promotion", "busy_agent", ws.AgentType, "state", ws.State)
			return nil
		}
	}

	// Check for open pull requests — do not promote if any exist
	openPRs, err := d.Deps.GitHub.ListPRs(ctx, "open")
	if err != nil {
		d.logger().Debug("failed to list open PRs for auto-processing check", "error", err)
		return nil
	}
	if len(openPRs) > 0 {
		d.logger().Debug("open PRs exist, skipping auto-issue promotion", "open_prs", len(openPRs))
		return nil
	}

	// List issues with agent:suggestion label
	suggestions, err := d.Deps.GitHub.ListIssues(ctx, []string{"agent:suggestion"})
	if err != nil {
		d.logger().Debug("failed to list suggestion issues", "error", err)
		return nil
	}

	for _, issue := range suggestions {
		// Check labels: must have ONLY agent:suggestion (no agent:failed, no agent:suggestion-rejected, no agent:ready, no agent:claimed)
		labels := make(map[string]bool)
		for _, l := range issue.Labels {
			labels[l.GetName()] = true
		}

		// Skip if it has any disqualifying labels
		if labels["agent:failed"] || labels["agent:suggestion-rejected"] ||
			labels["agent:ready"] || labels["agent:claimed"] ||
			labels["agent:in-progress"] || labels["agent:in-review"] {
			continue
		}

		// Found a clean suggestion — promote it
		issueNum := issue.GetNumber()
		d.logger().Info("auto-promoting suggestion to ready", "issue", issueNum, "title", issue.GetTitle())

		if err := d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:ready"}); err != nil {
			d.logger().Error("failed to add agent:ready label", "issue", issueNum, "error", err)
			return nil
		}

		_ = d.Deps.GitHub.CreateComment(ctx, issueNum,
			"🤖 Auto-promoting this suggestion for implementation (auto_issue_processing enabled).")

		// Only promote one at a time
		return nil
	}

	return nil
}

// reportEvent sends an event to the control plane if the reporter is configured.
func (d *DeveloperAgent) reportEvent(ctx context.Context, event apitypes.AgentEvent) {
	if d.Deps.Reporter == nil {
		return
	}
	_ = d.Deps.Reporter.Report(ctx, event)
}
