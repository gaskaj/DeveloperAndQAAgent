package developer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/gitops"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
	"github.com/google/go-github/v68/github"
)

// handleIssues is called by the poller when new issues are found.
func (d *DeveloperAgent) handleIssues(ctx context.Context, issues []*github.Issue) error {
	for _, issue := range issues {
		if err := d.processIssue(ctx, issue); err != nil {
			d.logger().Error("failed to process issue",
				"issue", issue.GetNumber(),
				"error", err,
			)
			continue
		}
	}
	return nil
}

func (d *DeveloperAgent) processIssue(ctx context.Context, issue *github.Issue) error {
	issueNum := issue.GetNumber()
	issueTitle := issue.GetTitle()
	issueBody := issue.GetBody()

	// Create enriched correlation context for this issue
	ctx = observability.EnsureCorrelationContext(ctx, string(d.Type()), issueNum)
	
	// Add issue metadata to correlation context
	ctx = observability.WithMetadata(ctx, "issue_title", issueTitle)
	ctx = observability.WithMetadata(ctx, "issue_number", fmt.Sprintf("%d", issueNum))

	d.logger().Info("processing issue", "number", issueNum, "title", issueTitle)

	// Create checkpoint manager for this workflow
	checkpointer := NewCheckpointManager(d.Deps.Store, d.logger())

	// Step 1: Claim — also removes agent:ready to prevent re-processing on restart.
	ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageClaim)
	d.updateStatus(state.StateClaim, issueNum, "claiming issue")
	
	// Check for context cancellation before proceeding
	if ctx.Err() != nil {
		d.logger().Info("context cancelled before claiming issue", "issue", issueNum)
		return ctx.Err()
	}
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "idle", "claim", "new_issue_detected")
	}
	
	if err := d.claimIssue(ctx, issueNum); err != nil {
		return fmt.Errorf("claiming issue: %w", err)
	}

	// Save state
	ws := &state.AgentWorkState{
		AgentType:   string(d.Type()),
		IssueNumber: issueNum,
		IssueTitle:  issueTitle,
		State:       state.StateClaim,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := d.Deps.Store.Save(ctx, ws); err != nil {
		d.logger().Error("failed to save state", "error", err)
	}

	// Create initial checkpoint
	if err := checkpointer.CreateCheckpoint(ctx, ws, "claim", map[string]interface{}{
		"issue_title": issueTitle,
		"action":      "claimed_issue",
	}); err != nil {
		d.logger().Error("failed to create claim checkpoint", "error", err)
	}

	// Step 2: Setup workspace — moved before analyze so Claude has real codebase context.
	ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStage("workspace"))
	d.updateStatus(state.StateWorkspace, issueNum, "setting up workspace")
	
	// Ensure workspace cleanup on exit
	var workspaceCreated bool
	defer func() {
		if workspaceCreated {
			// Mark workspace as stale on normal completion, failed on error
			newState := workspace.WorkspaceStateStale
			if ws.State == state.StateFailed {
				newState = workspace.WorkspaceStateFailed
			}
			if err := d.workspaceManager.UpdateWorkspaceState(context.Background(), issueNum, newState); err != nil {
				d.logger().Error("failed to update workspace state on exit", "issue", issueNum, "error", err)
			}
		}
	}()
	
	// Check for context cancellation before workspace setup
	if ctx.Err() != nil {
		d.logger().Info("context cancelled during workspace setup", "issue", issueNum)
		return d.handleGracefulShutdown(ctx, ws, "workspace_setup")
	}
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "claim", "workspace", "setup_development_environment")
	}
	
	// Create managed workspace
	managedWorkspace, err := d.workspaceManager.CreateWorkspace(ctx, issueNum)
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("creating managed workspace: %w", err))
		return err
	}
	workspaceCreated = true
	
	branchName := fmt.Sprintf("agent/issue-%d", issueNum)
	ws.BranchName = branchName
	ws.State = state.StateWorkspace
	ws.UpdatedAt = time.Now()
	ws.WorkspaceDir = managedWorkspace.Path
	_ = d.Deps.Store.Save(ctx, ws)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git",
		d.Deps.Config.GitHub.Owner, d.Deps.Config.GitHub.Repo)

	repo, err := gitops.Clone(repoURL, managedWorkspace.Path, d.Deps.Config.GitHub.Token)
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("cloning repo: %w", err))
		return err
	}

	if err := repo.CheckoutBranch(branchName, true); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("creating branch: %w", err))
		return err
	}
	
	// Validate workspace size after clone
	if err := d.validateWorkspaceSize(ctx, managedWorkspace.Path); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("workspace size validation failed: %w", err))
		return err
	}

	// Create workspace checkpoint
	if err := checkpointer.CreateCheckpoint(ctx, ws, "workspace", map[string]interface{}{
		"workspace_dir": managedWorkspace.Path,
		"branch_name":   branchName,
		"repo_url":      repoURL,
	}); err != nil {
		d.logger().Error("failed to create workspace checkpoint", "error", err)
	}

	// Create workspace snapshot after successful workspace setup
	if snapshot, err := d.workspaceManager.CreateWorkspaceSnapshot(ctx, issueNum, ws); err != nil {
		d.logger().Error("failed to create workspace snapshot", "error", err)
	} else if snapshot != nil {
		ws.LastSnapshotTime = time.Now()
		ws.ImplementationHash = snapshot.ImplementationHash
		_ = d.Deps.Store.Save(ctx, ws)
	}

	// Gather codebase context for Claude.
	repoContext := gatherRepoContext(repo)

	// Step 3: Analyze — now receives real codebase structure.
	ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageAnalyze)
	d.updateStatus(state.StateAnalyze, issueNum, "analyzing requirements")
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "workspace", "analyze", "begin_requirement_analysis")
	}
	
	var labels []string
	for _, l := range issue.Labels {
		labels = append(labels, l.GetName())
	}
	issueContext := claude.FormatIssueContext(issueNum, issueTitle, issueBody, labels)
	
	// Log LLM call for analysis
	startTime := time.Now()
	plan, tooComplex, err := d.analyze(ctx, issueContext, repoContext)
	analysisTime := time.Since(startTime)
	
	if err != nil {
		// Log analysis failure
		if d.Deps.StructuredLogger != nil {
			d.Deps.StructuredLogger.LogDecisionPoint(ctx, string(d.Type()), "analyze_failed", err.Error(), map[string]interface{}{
				"analysis_time_ms": analysisTime.Milliseconds(),
				"issue_complexity": "unknown",
			})
		}
		d.failIssue(ctx, ws, fmt.Errorf("analysis failed: %w", err))
		return err
	}
	
	// Log successful analysis with decision point
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogDecisionPoint(ctx, string(d.Type()), "analyze_complete", "issue analysis successful", map[string]interface{}{
			"analysis_time_ms": analysisTime.Milliseconds(),
			"too_complex":      tooComplex,
			"plan_length":      len(plan),
		})
	}

	// Post analysis plan as a comment on the issue.
	analysisComment := fmt.Sprintf("🤖 **Analysis complete**\n\n%s", plan)
	if d.Deps.Config.Decomposition.Enabled {
		if est := parseEstimatedIterations(plan); est > 0 {
			analysisComment += fmt.Sprintf("\n\n---\n**Estimated iterations**: %d | **Budget**: %d",
				est, d.Deps.Config.Decomposition.MaxIterationBudget)
		}
	}
	_ = d.Deps.GitHub.CreateComment(ctx, issueNum, analysisComment)

	// Step 3.5: Proactive decomposition
	if tooComplex && d.Deps.Config.Decomposition.Enabled {
		ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageDecompose)
		
		// Log decision point and workflow transition
		if d.Deps.StructuredLogger != nil {
			d.Deps.StructuredLogger.LogDecisionPoint(ctx, string(d.Type()), "proactive_decomposition", "issue exceeds complexity threshold", map[string]interface{}{
				"decomposition_enabled": true,
				"complexity_reason":     "too_complex_for_single_agent",
			})
			d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "analyze", "decompose", "proactive_decomposition_triggered")
		}
		
		d.logger().Info("issue too complex, decomposing", "issue", issueNum)
		childNums, err := d.decompose(ctx, issueNum, issueContext, plan)
		if err != nil {
			d.failIssue(ctx, ws, fmt.Errorf("decomposition failed: %w", err))
			return err
		}
		ws.ChildIssues = childNums
		ws.State = state.StateDecompose
		ws.UpdatedAt = time.Now()
		_ = d.Deps.Store.Save(ctx, ws)

		// Log handoff to child issues
		if d.Deps.StructuredLogger != nil {
			d.Deps.StructuredLogger.LogAgentHandoff(ctx, string(d.Type()), "child_agents", "decomposition_complete", len(childNums)*1024) // Estimate payload
		}

		_ = d.processChildIssues(ctx, childNums, issueNum)
		d.updateStatus(state.StateIdle, 0, "waiting for issues")
		return nil
	}

	// Step 4: Implement
	ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageImplement)
	d.updateStatus(state.StateImplement, issueNum, "implementing changes")
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "analyze", "implement", "begin_implementation")
	}
	
	_ = d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:in-progress"})
	ws.State = state.StateImplement
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	// Create snapshot before implementation begins
	if snapshot, err := d.workspaceManager.CreateWorkspaceSnapshot(ctx, issueNum, ws); err != nil {
		d.logger().Error("failed to create pre-implementation snapshot", "error", err)
	} else if snapshot != nil {
		ws.LastSnapshotTime = time.Now()
		ws.ImplementationHash = snapshot.ImplementationHash
		_ = d.Deps.Store.Save(ctx, ws)
	}

	implementStartTime := time.Now()
	if err := d.implement(ctx, repo, issueContext, plan, repoContext); err != nil {
		implementDuration := time.Since(implementStartTime)
		
		// Reactive decomposition: if iteration limit hit and decomposition enabled.
		if claude.IsMaxIterationsError(err) && d.Deps.Config.Decomposition.Enabled {
			// Log decision point for reactive decomposition
			if d.Deps.StructuredLogger != nil {
				d.Deps.StructuredLogger.LogDecisionPoint(ctx, string(d.Type()), "reactive_decomposition", "iteration limit exceeded", map[string]interface{}{
					"implementation_duration_ms": implementDuration.Milliseconds(),
					"error_type":                 "max_iterations_exceeded",
					"decomposition_trigger":      "reactive",
				})
				d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "implement", "decompose", "iteration_limit_exceeded")
			}
			
			d.logger().Info("iteration limit hit, reactively decomposing", "issue", issueNum)
			childNums, decompErr := d.reactiveDecompose(ctx, issueNum, issueContext, plan)
			if decompErr != nil {
				d.failIssue(ctx, ws, fmt.Errorf("reactive decomposition failed: %w", decompErr))
				return decompErr
			}
			ws.ChildIssues = childNums
			ws.State = state.StateDecompose
			ws.UpdatedAt = time.Now()
			_ = d.Deps.Store.Save(ctx, ws)

			// Log handoff to child agents
			if d.Deps.StructuredLogger != nil {
				d.Deps.StructuredLogger.LogAgentHandoff(ctx, string(d.Type()), "child_agents", "reactive_decomposition", len(childNums)*1024)
			}

			_ = d.processChildIssues(ctx, childNums, issueNum)
			d.updateStatus(state.StateIdle, 0, "waiting for issues")
			return nil
		}
		
		// Log implementation failure
		if d.Deps.StructuredLogger != nil {
			d.Deps.StructuredLogger.LogDecisionPoint(ctx, string(d.Type()), "implementation_failed", err.Error(), map[string]interface{}{
				"implementation_duration_ms": implementDuration.Milliseconds(),
				"error_type":                 "implementation_error",
			})
		}
		
		d.failIssue(ctx, ws, fmt.Errorf("implementation failed: %w", err))
		return err
	}

	// Step 5: Commit
	d.updateStatus(state.StateCommit, issueNum, "committing changes")
	ws.State = state.StateCommit
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	// Create snapshot after successful implementation
	if snapshot, err := d.workspaceManager.CreateWorkspaceSnapshot(ctx, issueNum, ws); err != nil {
		d.logger().Error("failed to create post-implementation snapshot", "error", err)
	} else if snapshot != nil {
		ws.LastSnapshotTime = time.Now()
		ws.ImplementationHash = snapshot.ImplementationHash
		_ = d.Deps.Store.Save(ctx, ws)
	}

	if err := repo.StageAll(); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("staging: %w", err))
		return err
	}

	hasChanges, err := repo.HasChanges()
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("checking for changes: %w", err))
		return err
	}
	if !hasChanges {
		d.failIssue(ctx, ws, fmt.Errorf("implementation produced no file changes"))
		return fmt.Errorf("implementation produced no file changes")
	}

	commitMsg := fmt.Sprintf("feat: implement #%d - %s", issueNum, issueTitle)
	if err := repo.Commit(commitMsg); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("committing: %w", err))
		return err
	}

	if err := repo.Push(); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("pushing: %w", err))
		return err
	}

	// Step 6: Create PR
	d.updateStatus(state.StatePR, issueNum, "creating pull request")
	ws.State = state.StatePR
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	pr, err := d.Deps.GitHub.CreatePR(ctx, ghub.PROptions{
		Title: fmt.Sprintf("feat: implement #%d - %s", issueNum, issueTitle),
		Body:  fmt.Sprintf("Closes #%d\n\n## Implementation\n\n%s", issueNum, plan),
		Head:  branchName,
		Base:  "main",
	})
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("creating PR: %w", err))
		return err
	}

	ws.PRNumber = pr.GetNumber()
	ws.State = state.StateReview
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	// Step 7: Validate PR checks
	ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStage("validate"))
	d.updateStatus(state.StateValidation, issueNum, "validating PR checks")
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, issueNum, "pr", "validate", "monitoring_pr_checks")
	}
	
	ws.State = state.StateValidation
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	validationErr := d.validatePRChecks(ctx, ws, repo, issueContext, plan)
	if validationErr != nil {
		d.failIssue(ctx, ws, fmt.Errorf("PR validation failed: %w", validationErr))
		return validationErr
	}

	// Step 8: Update labels and complete
	d.updateStatus(state.StateReview, issueNum, "awaiting review")
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:in-progress")
	_ = d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:in-review"})

	d.logger().Info("issue completed",
		"issue", issueNum,
		"pr", pr.GetNumber(),
		"branch", branchName,
	)

	ws.State = state.StateComplete
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	d.updateStatus(state.StateIdle, 0, "waiting for issues")

	return nil
}

// handleGracefulShutdown handles graceful shutdown during workflow processing
func (d *DeveloperAgent) handleGracefulShutdown(ctx context.Context, ws *state.AgentWorkState, stage string) error {
	d.logger().Info("handling graceful shutdown", "issue", ws.IssueNumber, "stage", stage)
	
	// Create checkpoint for interrupted work
	checkpointer := NewCheckpointManager(d.Deps.Store, d.logger())
	if err := checkpointer.CreateCheckpoint(ctx, ws, stage, map[string]interface{}{
		"interrupted_by": "graceful_shutdown",
		"interrupted_at": time.Now(),
		"stage":          stage,
	}); err != nil {
		d.logger().Error("failed to create shutdown checkpoint", "error", err)
	}
	
	// Clean up workspace on shutdown if in early stages
	if shouldCleanupWorkspaceOnShutdown(ws.State) {
		if err := d.workspaceManager.CleanupWorkspace(context.Background(), ws.IssueNumber); err != nil {
			d.logger().Error("failed to cleanup workspace on shutdown", "issue", ws.IssueNumber, "error", err)
		}
	} else {
		// Mark workspace as stale for later cleanup
		if err := d.workspaceManager.UpdateWorkspaceState(context.Background(), ws.IssueNumber, workspace.WorkspaceStateStale); err != nil {
			d.logger().Error("failed to mark workspace as stale on shutdown", "issue", ws.IssueNumber, "error", err)
		}
	}
	
	// Reset issue to ready state if we haven't made significant progress
	if shouldResetOnShutdown(ws.State) {
		if err := d.resetIssueToReady(ctx, ws.IssueNumber); err != nil {
			d.logger().Error("failed to reset issue to ready", "issue", ws.IssueNumber, "error", err)
		}
	}
	
	// Log workflow transition
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogWorkflowTransition(ctx, ws.IssueNumber, string(ws.State), "interrupted", "graceful_shutdown")
	}
	
	return ctx.Err()
}

// shouldCleanupWorkspaceOnShutdown determines if workspace should be cleaned up immediately on shutdown
func shouldCleanupWorkspaceOnShutdown(currentState state.State) bool {
	switch currentState {
	case state.StateClaim, state.StateWorkspace:
		return true // Very early states - clean up immediately
	default:
		return false // Later states - mark as stale for scheduled cleanup
	}
}

// validateWorkspaceSize checks if the workspace size is within limits
func (d *DeveloperAgent) validateWorkspaceSize(ctx context.Context, workspacePath string) error {
	// Calculate workspace size
	var size int64
	err := filepath.Walk(workspacePath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("calculating workspace size: %w", err)
	}

	sizeMB := size / (1024 * 1024)
	maxSizeMB := d.Deps.Config.Workspace.Limits.MaxSizeMB
	if maxSizeMB == 0 {
		maxSizeMB = workspace.DefaultConfig().MaxSizeMB
	}

	if sizeMB > maxSizeMB {
		return fmt.Errorf("workspace size %d MB exceeds limit %d MB", sizeMB, maxSizeMB)
	}

	d.logger().Debug("workspace size validation passed",
		"path", workspacePath,
		"size_mb", sizeMB,
		"max_size_mb", maxSizeMB,
	)

	return nil
}

// shouldResetOnShutdown determines if an issue should be reset to ready state on shutdown
func shouldResetOnShutdown(currentState state.State) bool {
	switch currentState {
	case state.StateClaim, state.StateWorkspace, state.StateAnalyze:
		return true // Early states - safe to reset
	case state.StateImplement:
		return true // Implementation can be restarted
	default:
		return false // Later states - leave as-is
	}
}

// resetIssueToReady resets an issue back to ready state
func (d *DeveloperAgent) resetIssueToReady(ctx context.Context, issueNum int) error {
	d.logger().Info("resetting issue to ready state", "issue", issueNum)
	
	// Remove agent:claimed and agent:in-progress labels
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:claimed")
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:in-progress")
	
	// Add agent:ready label
	if err := d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:ready"}); err != nil {
		return fmt.Errorf("adding ready label: %w", err)
	}
	
	return nil
}

// validatePRChecks monitors and validates PR checks, fixing failures as needed
func (d *DeveloperAgent) validatePRChecks(ctx context.Context, ws *state.AgentWorkState, repo *gitops.Repo, issueContext, plan string) error {
	prNumber := ws.PRNumber
	issueNum := ws.IssueNumber
	
	d.logger().Info("starting PR validation", "pr", prNumber, "issue", issueNum)
	_ = d.Deps.GitHub.CreateComment(ctx, issueNum, "🤖 Monitoring PR checks and will fix any failures...")

	// Default validation options
	opts := ghub.DefaultPRValidationOptions()
	maxRetries := opts.MaxRetries

	for attempt := 1; attempt <= maxRetries; attempt++ {
		d.logger().Info("validating PR checks", "pr", prNumber, "attempt", attempt)
		
		result, err := d.Deps.GitHub.ValidatePR(ctx, prNumber, opts)
		if err != nil {
			return fmt.Errorf("validating PR %d (attempt %d): %w", prNumber, attempt, err)
		}

		// Success! All checks are passing
		if result.AllChecksPassing && result.Status == ghub.PRCheckStatusSuccess {
			d.logger().Info("PR validation successful", "pr", prNumber, "total_checks", result.TotalChecks)
			_ = d.Deps.GitHub.CreateComment(ctx, issueNum, 
				fmt.Sprintf("✅ All PR checks are passing! (%d checks completed successfully)", result.TotalChecks))
			return nil
		}

		// Checks failed, try to fix them
		if result.Status == ghub.PRCheckStatusFailed {
			d.logger().Warn("PR checks failed", "pr", prNumber, "failed_checks", len(result.FailedChecks))
			
			failureAnalysis := result.AnalyzeFailures()
			
			// Post failure analysis as comment
			failureComment := fmt.Sprintf("❌ **PR checks failed (attempt %d/%d)**\n\n%s\n\n🔧 Attempting to fix these issues...", 
				attempt, maxRetries, failureAnalysis)
			_ = d.Deps.GitHub.CreateComment(ctx, issueNum, failureComment)

			// If this is the last attempt, don't try to fix
			if attempt >= maxRetries {
				return fmt.Errorf("PR checks failed after %d attempts: %s", maxRetries, failureAnalysis)
			}

			// Generate fix prompt and attempt to fix the failures
			fixErr := d.fixPRFailures(ctx, repo, result, issueContext, plan)
			if fixErr != nil {
				d.logger().Error("failed to fix PR failures", "pr", prNumber, "attempt", attempt, "error", fixErr)
				_ = d.Deps.GitHub.CreateComment(ctx, issueNum, 
					fmt.Sprintf("⚠️ Failed to automatically fix PR issues (attempt %d): %v", attempt, fixErr))
				
				// Continue to next attempt
				continue
			}

			// Push the fixes and continue to next validation attempt
			if err := repo.StageAll(); err != nil {
				return fmt.Errorf("staging fixes: %w", err)
			}

			hasChanges, chkErr := repo.HasChanges()
			if chkErr != nil {
				return fmt.Errorf("checking for fix changes: %w", chkErr)
			}
			if !hasChanges {
				d.logger().Warn("fix attempt produced no file changes, skipping commit", "pr", prNumber, "attempt", attempt)
				_ = d.Deps.GitHub.CreateComment(ctx, issueNum,
					fmt.Sprintf("⚠️ Fix attempt %d produced no file changes, retrying...", attempt))
				continue
			}

			commitMsg := fmt.Sprintf("fix: address PR check failures (#%d)", issueNum)
			if err := repo.Commit(commitMsg); err != nil {
				return fmt.Errorf("committing fixes: %w", err)
			}

			if err := repo.Push(); err != nil {
				return fmt.Errorf("pushing fixes: %w", err)
			}

			d.logger().Info("fixes pushed, waiting for new check run", "pr", prNumber, "attempt", attempt)
			_ = d.Deps.GitHub.CreateComment(ctx, issueNum, 
				fmt.Sprintf("🔄 Fixes pushed! Waiting for checks to run again... (attempt %d/%d)", attempt, maxRetries))

			// Wait a bit for the new checks to start
			time.Sleep(30 * time.Second)
		}
	}

	return fmt.Errorf("PR validation failed after %d attempts", maxRetries)
}

// fixPRFailures attempts to fix the failed PR checks using Claude
func (d *DeveloperAgent) fixPRFailures(ctx context.Context, repo *gitops.Repo, result *ghub.PRValidationResult, issueContext, originalPlan string) error {
	fixPrompt := result.GenerateFixPrompt(issueContext, originalPlan)
	
	d.logger().Info("generating fixes for PR failures", "failed_checks", len(result.FailedChecks))
	
	executor := d.createToolExecutor(repo)
	
	// Use a shorter iteration limit for fixes to prevent getting stuck
	maxIter := 10
	if d.Deps.Config.Decomposition.Enabled {
		maxIter = d.Deps.Config.Decomposition.MaxIterationBudget / 2
	}
	
	conv := claude.NewConversation(
		d.Deps.Claude,
		SystemPrompt,
		claude.DevTools(),
		executor,
		d.Deps.Logger,
		maxIter,
	)

	_, err := conv.Send(ctx, fixPrompt)
	if err != nil {
		return fmt.Errorf("generating fixes: %w", err)
	}
	
	return nil
}

func (d *DeveloperAgent) claimIssue(ctx context.Context, number int) error {
	// Assign self if no assignees
	if err := d.Deps.GitHub.AssignSelfIfNoAssignees(ctx, number); err != nil {
		d.logger().Warn("failed to assign self to issue", "issue", number, "error", err)
		// Don't fail claiming if assignment fails - continue with labeling and comment
	}
	
	if err := d.Deps.GitHub.AddLabels(ctx, number, []string{"agent:claimed"}); err != nil {
		return err
	}
	// Remove agent:ready immediately to prevent re-processing on crash/restart.
	_ = d.Deps.GitHub.RemoveLabel(ctx, number, "agent:ready")
	if err := d.Deps.GitHub.CreateComment(ctx, number, "🤖 Developer agent claiming this issue. Starting analysis..."); err != nil {
		return err
	}
	return nil
}

func (d *DeveloperAgent) analyze(ctx context.Context, issueContext, repoContext string) (plan string, tooComplex bool, err error) {
	conv := claude.NewConversation(
		d.Deps.Claude,
		SystemPrompt,
		nil, // no tools for analysis
		nil,
		d.Deps.Logger,
		0, // no tools, single-turn — limit doesn't apply
	)

	prompt := fmt.Sprintf(AnalyzePrompt, issueContext)

	// Inject codebase structure so the plan is based on real files.
	if repoContext != "" {
		prompt += "\n\n" + repoContext
	}

	// When decomposition is enabled, append complexity estimation to the prompt.
	if d.Deps.Config.Decomposition.Enabled {
		budget := d.Deps.Config.Decomposition.MaxIterationBudget
		threshold := int(float64(budget) * 0.5) // 50% of budget
		maxSubtasks := d.Deps.Config.Decomposition.MaxSubtasks
		prompt += fmt.Sprintf(ComplexityEstimatePrompt, budget, threshold, maxSubtasks)
	}

	response, err := conv.Send(ctx, prompt)
	if err != nil {
		return "", false, err
	}

	if d.Deps.Config.Decomposition.Enabled {
		tooComplex = parseComplexityResult(response)
	}

	return response, tooComplex, nil
}

func (d *DeveloperAgent) implement(ctx context.Context, repo *gitops.Repo, issueContext, plan, repoContext string) error {
	executor := d.createToolExecutor(repo)

	// Use the configured iteration budget when decomposition is enabled; default otherwise.
	maxIter := 0
	if d.Deps.Config.Decomposition.Enabled {
		maxIter = d.Deps.Config.Decomposition.MaxIterationBudget
	}

	conv := claude.NewConversation(
		d.Deps.Claude,
		SystemPrompt,
		claude.DevTools(),
		executor,
		d.Deps.Logger,
		maxIter,
	)

	prompt := fmt.Sprintf(ImplementPrompt, issueContext, plan)

	// Inject codebase structure so Claude doesn't waste iterations discovering it.
	if repoContext != "" {
		prompt += "\n\n" + repoContext
	}

	// Pre-read files mentioned in the plan so Claude can start writing immediately.
	filePaths := extractFilePaths(plan)
	if preRead := preReadFiles(repo, filePaths); preRead != "" {
		prompt += "\n\n" + preRead
	}

	_, err := conv.Send(ctx, prompt)
	return err
}

func (d *DeveloperAgent) createToolExecutor(repo *gitops.Repo) claude.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		var params map[string]string
		if err := json.Unmarshal(input, &params); err != nil {
			return "", fmt.Errorf("parsing tool input: %w", err)
		}

		switch name {
		case "read_file":
			content, err := repo.ReadFile(params["path"])
			if err != nil {
				return "", err
			}
			return content, nil

		case "edit_file":
			return executeEditFile(repo, params)

		case "write_file":
			if err := repo.WriteFile(params["path"], params["content"]); err != nil {
				return "", err
			}
			return "file written successfully", nil

		case "search_files":
			return executeSearchFiles(repo, params)

		case "list_files":
			files, err := repo.ListFiles(params["path"])
			if err != nil {
				return "", err
			}
			return strings.Join(files, "\n"), nil

		case "run_command":
			cmd := exec.CommandContext(ctx, "sh", "-c", params["command"])
			cmd.Dir = repo.Dir()
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("error: %v\noutput: %s", err, string(out)), nil
			}
			return string(out), nil

		default:
			return "", fmt.Errorf("unknown tool: %s", name)
		}
	}
}

func (d *DeveloperAgent) failIssue(ctx context.Context, ws *state.AgentWorkState, err error) {
	d.logger().Error("issue processing failed", "issue", ws.IssueNumber, "error", err)

	ws.State = state.StateFailed
	ws.Error = err.Error()
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	_ = d.Deps.GitHub.CreateComment(ctx, ws.IssueNumber,
		fmt.Sprintf("🤖 Developer agent failed: %v", err))
	_ = d.Deps.GitHub.AddLabels(ctx, ws.IssueNumber, []string{"agent:failed"})

	d.updateStatus(state.StateIdle, 0, "waiting for issues")
}

// gatherRepoContext builds a file tree and go.mod summary for injection into prompts.
// This eliminates the need for Claude to waste iterations discovering project structure.
func gatherRepoContext(repo *gitops.Repo) string {
	var sb strings.Builder
	sb.WriteString("## Repository Structure\n\n```\n")

	repoDir := repo.Dir()
	_ = filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoDir, path)
		if rel == "." {
			return nil
		}
		// Skip hidden dirs (.git), workspaces, and vendor.
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "workspaces" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		indent := strings.Repeat("  ", strings.Count(rel, string(filepath.Separator)))
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		sb.WriteString(indent + name + "\n")
		return nil
	})

	sb.WriteString("```\n\n")

	// Include go.mod for module path and dependencies.
	if content, err := repo.ReadFile("go.mod"); err == nil {
		sb.WriteString("### go.mod\n```\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// extractFilePaths finds Go file paths mentioned in the plan text.
func extractFilePaths(plan string) []string {
	re := regexp.MustCompile(`(?:^|[\s` + "`" + `\(])((?:[a-zA-Z0-9_]+/)*[a-zA-Z0-9_]+\.go)\b`)
	matches := re.FindAllStringSubmatch(plan, -1)
	seen := make(map[string]bool)
	var paths []string
	for _, m := range matches {
		p := m[1]
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// executeEditFile handles the edit_file tool: search-and-replace within a file.
func executeEditFile(repo *gitops.Repo, params map[string]string) (string, error) {
	path := params["path"]
	oldStr := params["old_string"]
	newStr := params["new_string"]

	content, err := repo.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_string appears %d times in %s (must be unique — include more surrounding context)", count, path)
	}

	updated := strings.Replace(content, oldStr, newStr, 1)
	if err := repo.WriteFile(path, updated); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return "file edited successfully", nil
}

// searchableExtensions lists file extensions that search_files will inspect.
var searchableExtensions = map[string]bool{
	".go": true, ".yaml": true, ".yml": true, ".json": true,
	".md": true, ".txt": true, ".mod": true, ".sum": true,
	".toml": true, ".cfg": true, ".conf": true, ".sh": true,
}

// executeSearchFiles handles the search_files tool: grep across the workspace.
func executeSearchFiles(repo *gitops.Repo, params map[string]string) (string, error) {
	pattern := params["pattern"]
	searchDir := repo.Dir()
	if p, ok := params["path"]; ok && p != "" {
		searchDir = filepath.Join(repo.Dir(), p)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		// Fall back to literal match if pattern isn't valid regex.
		re = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	var results []string
	_ = filepath.WalkDir(searchDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "workspaces" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if !searchableExtensions[ext] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repo.Dir(), path)
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimRight(line, "\r")))
			}
		}
		return nil
	})

	if len(results) == 0 {
		return "no matches found", nil
	}
	// Cap to prevent huge outputs.
	if len(results) > 50 {
		total := len(results)
		results = results[:50]
		results = append(results, fmt.Sprintf("... (%d more matches not shown)", total-50))
	}
	return strings.Join(results, "\n"), nil
}

// preReadFiles reads files from the repo that are mentioned in the plan,
// so Claude doesn't need to spend iterations reading them.
func preReadFiles(repo *gitops.Repo, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Pre-read Files\n\nThese files from the plan have been pre-loaded to save iterations.\nYou do NOT need to read_file these again — start writing immediately.\n\n")
	count := 0
	for _, p := range paths {
		content, err := repo.ReadFile(p)
		if err != nil {
			continue // file doesn't exist yet, that's fine
		}
		// Truncate very large files.
		if len(content) > 15000 {
			content = content[:15000] + "\n... (truncated at 15000 chars)"
		}
		sb.WriteString(fmt.Sprintf("### %s\n```go\n%s\n```\n\n", p, content))
		count++
		if count >= 8 {
			break // cap to avoid massive prompts
		}
	}
	if count == 0 {
		return ""
	}
	return sb.String()
}
