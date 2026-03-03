package developer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
)

// ResumeManager handles intelligent resume operations for developer workflows.
type ResumeManager struct {
	persistence   *workspace.WorkspacePersistence
	logger        *slog.Logger
	recoveryMgr   *RecoveryManager
}

// ResumePoint represents an optimal point to resume work from.
type ResumePoint struct {
	Stage           string                 `json:"stage"`
	Snapshot        *workspace.WorkspaceSnapshot `json:"snapshot"`
	RecommendedActions []ResumeAction       `json:"recommended_actions"`
	ConfidenceLevel ResumeConfidence       `json:"confidence_level"`
	RiskAssessment  ResumeRisk             `json:"risk_assessment"`
	EstimatedSavings time.Duration          `json:"estimated_savings"`
}

// ResumeAction represents an action recommended during resume.
type ResumeAction string

const (
	ResumeActionValidateFiles     ResumeAction = "validate_files"
	ResumeActionRestoreGitState   ResumeAction = "restore_git_state"
	ResumeActionCleanWorkspace    ResumeAction = "clean_workspace"
	ResumeActionVerifyBuild       ResumeAction = "verify_build"
	ResumeActionRestoreContext    ResumeAction = "restore_context"
	ResumeActionStartFresh        ResumeAction = "start_fresh"
)

// ResumeConfidence represents confidence in resume operation safety.
type ResumeConfidence string

const (
	ResumeConfidenceHigh   ResumeConfidence = "high"
	ResumeConfidenceMedium ResumeConfidence = "medium"
	ResumeConfidenceLow    ResumeConfidence = "low"
)

// ResumeRisk represents the risk level of resume operation.
type ResumeRisk string

const (
	ResumeRiskLow    ResumeRisk = "low"
	ResumeRiskMedium ResumeRisk = "medium"
	ResumeRiskHigh   ResumeRisk = "high"
)

// ResumeStrategy represents different strategies for resuming work.
type ResumeStrategy string

const (
	ResumeStrategyFromSnapshot ResumeStrategy = "from_snapshot"
	ResumeStrategyFromCheckpoint ResumeStrategy = "from_checkpoint"
	ResumeStrategyCleanRestart   ResumeStrategy = "clean_restart"
	ResumeStrategyManualReview   ResumeStrategy = "manual_review"
)

// NewResumeManager creates a new resume manager.
func NewResumeManager(persistence *workspace.WorkspacePersistence, recoveryMgr *RecoveryManager, logger *slog.Logger) *ResumeManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &ResumeManager{
		persistence: persistence,
		logger:      logger,
		recoveryMgr: recoveryMgr,
	}
}

// AnalyzeResumeOptions analyzes available resume options for an agent restart.
func (rm *ResumeManager) AnalyzeResumeOptions(ctx context.Context, agentState *state.AgentWorkState) (*ResumePoint, error) {
	rm.logger.Info("analyzing resume options", 
		"agent_type", agentState.AgentType,
		"issue", agentState.IssueNumber,
		"state", agentState.State,
	)

	// Check for available snapshots
	snapshot, err := rm.persistence.RestoreSnapshot(ctx, agentState.IssueNumber)
	if err != nil {
		rm.logger.Error("failed to restore snapshot", "error", err)
		return rm.createFallbackResumePoint(agentState), nil
	}

	if snapshot == nil {
		rm.logger.Info("no snapshots available", "issue", agentState.IssueNumber)
		return rm.createFallbackResumePoint(agentState), nil
	}

	// Analyze snapshot compatibility with current state
	resumePoint, err := rm.analyzeSnapshotResume(ctx, agentState, snapshot)
	if err != nil {
		rm.logger.Error("failed to analyze snapshot resume", "error", err)
		return rm.createFallbackResumePoint(agentState), nil
	}

	rm.logger.Info("resume analysis complete",
		"strategy", ResumeStrategyFromSnapshot,
		"confidence", resumePoint.ConfidenceLevel,
		"risk", resumePoint.RiskAssessment,
		"estimated_savings", resumePoint.EstimatedSavings,
	)

	return resumePoint, nil
}

// ExecuteResume executes the recommended resume strategy.
func (rm *ResumeManager) ExecuteResume(ctx context.Context, resumePoint *ResumePoint, workspaceDir string) error {
	rm.logger.Info("executing resume", 
		"stage", resumePoint.Stage,
		"actions", len(resumePoint.RecommendedActions),
		"confidence", resumePoint.ConfidenceLevel,
	)

	// Execute recommended actions in order
	for i, action := range resumePoint.RecommendedActions {
		rm.logger.Debug("executing resume action", 
			"action", action, 
			"step", fmt.Sprintf("%d/%d", i+1, len(resumePoint.RecommendedActions)),
		)

		if err := rm.executeResumeAction(ctx, action, resumePoint, workspaceDir); err != nil {
			return fmt.Errorf("executing resume action %s: %w", action, err)
		}
	}

	rm.logger.Info("resume execution completed successfully")
	return nil
}

// DetermineOptimalResumeStrategy determines the best strategy for resuming work.
func (rm *ResumeManager) DetermineOptimalResumeStrategy(ctx context.Context, agentState *state.AgentWorkState, snapshot *workspace.WorkspaceSnapshot) ResumeStrategy {
	// Check if snapshot is too old or incompatible
	if snapshot == nil {
		return ResumeStrategyCleanRestart
	}

	snapshotAge := time.Since(snapshot.Timestamp)
	maxResumeAge := 24 * time.Hour // Could be configurable
	
	if snapshotAge > maxResumeAge {
		rm.logger.Info("snapshot too old for resume", 
			"age", snapshotAge, 
			"max_age", maxResumeAge,
		)
		return ResumeStrategyCleanRestart
	}

	// Check for state compatibility
	if !rm.isStateCompatible(agentState.State, snapshot.AgentState.State) {
		rm.logger.Info("incompatible states for snapshot resume",
			"current_state", agentState.State,
			"snapshot_state", snapshot.AgentState.State,
		)
		return ResumeStrategyFromCheckpoint
	}

	// Check implementation hash for significant changes
	if rm.hasSignificantChanges(snapshot) {
		rm.logger.Info("significant changes detected since snapshot")
		return ResumeStrategyFromCheckpoint
	}

	return ResumeStrategyFromSnapshot
}

// CalculateResumeSavings estimates time/resource savings from resume vs restart.
func (rm *ResumeManager) CalculateResumeSavings(resumePoint *ResumePoint) time.Duration {
	if resumePoint.Snapshot == nil {
		return 0
	}

	// Base estimates for different stages (would be refined based on metrics)
	stageSavings := map[state.WorkflowState]time.Duration{
		state.StateAnalyze:   10 * time.Minute, // Avoid re-analyzing issue
		state.StateWorkspace: 2 * time.Minute,  // Avoid re-cloning repository
		state.StateImplement: 15 * time.Minute, // Resume from partial implementation
		state.StateCommit:    1 * time.Minute,  // Avoid re-staging files
		state.StatePR:        1 * time.Minute,  // Avoid PR creation
	}

	currentState := resumePoint.Snapshot.AgentState.State
	if savings, exists := stageSavings[currentState]; exists {
		// Adjust based on confidence and risk
		confidence_multiplier := rm.getConfidenceMultiplier(resumePoint.ConfidenceLevel)
		risk_adjustment := rm.getRiskAdjustment(resumePoint.RiskAssessment)
		
		adjusted := time.Duration(float64(savings) * confidence_multiplier * risk_adjustment)
		return adjusted
	}

	return 0
}

// analyzeSnapshotResume analyzes whether a snapshot can be used for resume.
func (rm *ResumeManager) analyzeSnapshotResume(ctx context.Context, agentState *state.AgentWorkState, snapshot *workspace.WorkspaceSnapshot) (*ResumePoint, error) {
	resumePoint := &ResumePoint{
		Stage:    string(snapshot.AgentState.State),
		Snapshot: snapshot,
	}

	// Determine confidence based on various factors
	confidence := rm.assessResumeConfidence(agentState, snapshot)
	resumePoint.ConfidenceLevel = confidence

	// Assess risks
	risk := rm.assessResumeRisk(snapshot)
	resumePoint.RiskAssessment = risk

	// Generate recommended actions based on analysis
	actions := rm.generateResumeActions(agentState, snapshot, confidence, risk)
	resumePoint.RecommendedActions = actions

	// Calculate potential time savings
	savings := rm.CalculateResumeSavings(resumePoint)
	resumePoint.EstimatedSavings = savings

	return resumePoint, nil
}

// createFallbackResumePoint creates a fallback resume point when no snapshots are available.
func (rm *ResumeManager) createFallbackResumePoint(agentState *state.AgentWorkState) *ResumePoint {
	return &ResumePoint{
		Stage:    "clean_start",
		Snapshot: nil,
		RecommendedActions: []ResumeAction{
			ResumeActionStartFresh,
		},
		ConfidenceLevel:  ResumeConfidenceHigh, // High confidence in clean start
		RiskAssessment:   ResumeRiskLow,       // Low risk in clean start
		EstimatedSavings: 0,                   // No savings from clean start
	}
}

// assessResumeConfidence assesses confidence in resume operation based on multiple factors.
func (rm *ResumeManager) assessResumeConfidence(agentState *state.AgentWorkState, snapshot *workspace.WorkspaceSnapshot) ResumeConfidence {
	score := 100.0 // Start with high confidence

	// Factor 1: Snapshot age (older snapshots are less reliable)
	age := time.Since(snapshot.Timestamp)
	if age > 2*time.Hour {
		score -= 20
	} else if age > 30*time.Minute {
		score -= 10
	}

	// Factor 2: State compatibility
	if !rm.isStateCompatible(agentState.State, snapshot.AgentState.State) {
		score -= 30
	}

	// Factor 3: File count (more files = more complexity)
	fileCount := len(snapshot.FileStates)
	if fileCount > 100 {
		score -= 10
	} else if fileCount > 50 {
		score -= 5
	}

	// Factor 4: Implementation hash changes
	if rm.hasSignificantChanges(snapshot) {
		score -= 25
	}

	// Convert score to confidence level
	if score >= 80 {
		return ResumeConfidenceHigh
	} else if score >= 60 {
		return ResumeConfidenceMedium
	} else {
		return ResumeConfidenceLow
	}
}

// assessResumeRisk assesses the risk of resume operation.
func (rm *ResumeManager) assessResumeRisk(snapshot *workspace.WorkspaceSnapshot) ResumeRisk {
	risk := 0.0

	// Factor 1: Stage risk (later stages have more to lose)
	switch snapshot.AgentState.State {
	case state.StateImplement:
		risk += 0.3 // Moderate risk - partial implementation
	case state.StateCommit:
		risk += 0.4 // Higher risk - uncommitted changes
	case state.StatePR:
		risk += 0.2 // Lower risk - work already committed
	default:
		risk += 0.1 // Low risk for early stages
	}

	// Factor 2: File modification risk
	modifiedFiles := 0
	for _, fileState := range snapshot.FileStates {
		if fileState.IsModified {
			modifiedFiles++
		}
	}
	if modifiedFiles > 10 {
		risk += 0.3
	} else if modifiedFiles > 5 {
		risk += 0.2
	}

	// Factor 3: Git state risk
	if snapshot.GitState.HasChanges {
		risk += 0.2
	}

	// Convert risk score to risk level
	if risk < 0.3 {
		return ResumeRiskLow
	} else if risk < 0.6 {
		return ResumeRiskMedium
	} else {
		return ResumeRiskHigh
	}
}

// generateResumeActions generates recommended actions for resume operation.
func (rm *ResumeManager) generateResumeActions(agentState *state.AgentWorkState, snapshot *workspace.WorkspaceSnapshot, confidence ResumeConfidence, risk ResumeRisk) []ResumeAction {
	var actions []ResumeAction

	// Always validate files first
	actions = append(actions, ResumeActionValidateFiles)

	// Based on confidence and risk, add additional actions
	if confidence == ResumeConfidenceLow || risk == ResumeRiskHigh {
		actions = append(actions, ResumeActionCleanWorkspace)
		actions = append(actions, ResumeActionStartFresh)
		return actions
	}

	// Medium confidence/risk actions
	if confidence == ResumeConfidenceMedium || risk == ResumeRiskMedium {
		actions = append(actions, ResumeActionVerifyBuild)
	}

	// Git state restoration
	if snapshot.GitState.HasChanges {
		actions = append(actions, ResumeActionRestoreGitState)
	}

	// Context restoration for later stages
	if snapshot.AgentState.State == state.StateImplement || snapshot.AgentState.State == state.StateCommit {
		actions = append(actions, ResumeActionRestoreContext)
	}

	return actions
}

// executeResumeAction executes a specific resume action.
func (rm *ResumeManager) executeResumeAction(ctx context.Context, action ResumeAction, resumePoint *ResumePoint, workspaceDir string) error {
	switch action {
	case ResumeActionValidateFiles:
		return rm.validateFiles(ctx, resumePoint.Snapshot, workspaceDir)
	case ResumeActionRestoreGitState:
		return rm.restoreGitState(ctx, resumePoint.Snapshot, workspaceDir)
	case ResumeActionCleanWorkspace:
		return rm.cleanWorkspace(ctx, workspaceDir)
	case ResumeActionVerifyBuild:
		return rm.verifyBuild(ctx, workspaceDir)
	case ResumeActionRestoreContext:
		return rm.restoreContext(ctx, resumePoint.Snapshot)
	case ResumeActionStartFresh:
		return rm.startFresh(ctx, resumePoint.Snapshot.IssueNumber)
	default:
		return fmt.Errorf("unknown resume action: %s", action)
	}
}

// isStateCompatible checks if current state is compatible with snapshot state.
func (rm *ResumeManager) isStateCompatible(currentState, snapshotState state.WorkflowState) bool {
	// Define compatible state transitions
	compatibleTransitions := map[state.WorkflowState][]state.WorkflowState{
		state.StateAnalyze:   {state.StateAnalyze, state.StateWorkspace},
		state.StateWorkspace: {state.StateWorkspace, state.StateImplement},
		state.StateImplement: {state.StateImplement, state.StateCommit},
		state.StateCommit:    {state.StateCommit, state.StatePR},
		state.StatePR:        {state.StatePR, state.StateValidation},
	}

	if transitions, exists := compatibleTransitions[snapshotState]; exists {
		for _, compatible := range transitions {
			if compatible == currentState {
				return true
			}
		}
	}

	return currentState == snapshotState
}

// hasSignificantChanges checks if the implementation has changed significantly since snapshot.
func (rm *ResumeManager) hasSignificantChanges(snapshot *workspace.WorkspaceSnapshot) bool {
	// This would typically compare current workspace state with snapshot
	// For now, assume no significant changes
	return false
}

// Helper methods for confidence and risk calculations

func (rm *ResumeManager) getConfidenceMultiplier(confidence ResumeConfidence) float64 {
	switch confidence {
	case ResumeConfidenceHigh:
		return 1.0
	case ResumeConfidenceMedium:
		return 0.7
	case ResumeConfidenceLow:
		return 0.4
	default:
		return 0.5
	}
}

func (rm *ResumeManager) getRiskAdjustment(risk ResumeRisk) float64 {
	switch risk {
	case ResumeRiskLow:
		return 1.0
	case ResumeRiskMedium:
		return 0.8
	case ResumeRiskHigh:
		return 0.5
	default:
		return 0.6
	}
}

// Action implementation methods (simplified for this implementation)

func (rm *ResumeManager) validateFiles(ctx context.Context, snapshot *workspace.WorkspaceSnapshot, workspaceDir string) error {
	rm.logger.Debug("validating files against snapshot", "file_count", len(snapshot.FileStates))
	// Implementation would compare current files with snapshot file states
	return nil
}

func (rm *ResumeManager) restoreGitState(ctx context.Context, snapshot *workspace.WorkspaceSnapshot, workspaceDir string) error {
	rm.logger.Debug("restoring git state", "branch", snapshot.GitState.Branch)
	// Implementation would restore git branch and staged changes
	return nil
}

func (rm *ResumeManager) cleanWorkspace(ctx context.Context, workspaceDir string) error {
	rm.logger.Debug("cleaning workspace", "dir", workspaceDir)
	// Implementation would clean up temporary files and reset workspace
	return nil
}

func (rm *ResumeManager) verifyBuild(ctx context.Context, workspaceDir string) error {
	rm.logger.Debug("verifying build", "dir", workspaceDir)
	// Implementation would run build verification
	return nil
}

func (rm *ResumeManager) restoreContext(ctx context.Context, snapshot *workspace.WorkspaceSnapshot) error {
	rm.logger.Debug("restoring context", "context_summary", snapshot.ClaudeContext.ContextSummary)
	// Implementation would restore conversation context
	return nil
}

func (rm *ResumeManager) startFresh(ctx context.Context, issueNumber int) error {
	rm.logger.Debug("starting fresh", "issue", issueNumber)
	// Implementation would trigger clean restart of the workflow
	return nil
}