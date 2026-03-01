package developer

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/gitops"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
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

	d.logger().Info("processing issue", "number", issueNum, "title", issueTitle)

	// Step 1: Claim
	d.updateStatus(state.StateClaim, issueNum, "claiming issue")
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

	// Step 2: Analyze
	d.updateStatus(state.StateAnalyze, issueNum, "analyzing requirements")
	var labels []string
	for _, l := range issue.Labels {
		labels = append(labels, l.GetName())
	}
	issueContext := claude.FormatIssueContext(issueNum, issueTitle, issueBody, labels)
	plan, tooComplex, err := d.analyze(ctx, issueContext)
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("analysis failed: %w", err))
		return err
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

	// Step 2.5: Proactive decomposition
	if tooComplex && d.Deps.Config.Decomposition.Enabled {
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

		_ = d.processChildIssues(ctx, childNums, issueNum)
		d.updateStatus(state.StateIdle, 0, "waiting for issues")
		return nil
	}

	// Step 3: Setup workspace
	d.updateStatus(state.StateWorkspace, issueNum, "setting up workspace")
	branchName := fmt.Sprintf("agent/issue-%d", issueNum)
	ws.BranchName = branchName
	ws.State = state.StateWorkspace
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git",
		d.Deps.Config.GitHub.Owner, d.Deps.Config.GitHub.Repo)
	workDir := fmt.Sprintf("%s/issue-%d", d.Deps.Config.Agents.Developer.WorkspaceDir, issueNum)
	ws.WorkspaceDir = workDir

	repo, err := gitops.Clone(repoURL, workDir, d.Deps.Config.GitHub.Token)
	if err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("cloning repo: %w", err))
		return err
	}

	if err := repo.CheckoutBranch(branchName, true); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("creating branch: %w", err))
		return err
	}

	// Step 4: Implement
	d.updateStatus(state.StateImplement, issueNum, "implementing changes")
	ws.State = state.StateImplement
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	if err := d.implement(ctx, repo, issueContext, plan); err != nil {
		// Reactive decomposition: if iteration limit hit and decomposition enabled.
		if claude.IsMaxIterationsError(err) && d.Deps.Config.Decomposition.Enabled {
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

			_ = d.processChildIssues(ctx, childNums, issueNum)
			d.updateStatus(state.StateIdle, 0, "waiting for issues")
			return nil
		}
		d.failIssue(ctx, ws, fmt.Errorf("implementation failed: %w", err))
		return err
	}

	// Step 5: Commit
	d.updateStatus(state.StateCommit, issueNum, "committing changes")
	ws.State = state.StateCommit
	ws.UpdatedAt = time.Now()
	_ = d.Deps.Store.Save(ctx, ws)

	if err := repo.StageAll(); err != nil {
		d.failIssue(ctx, ws, fmt.Errorf("staging: %w", err))
		return err
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

	// Step 7: Update labels and complete
	d.updateStatus(state.StateReview, issueNum, "awaiting review")
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:ready")
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

func (d *DeveloperAgent) claimIssue(ctx context.Context, number int) error {
	if err := d.Deps.GitHub.AddLabels(ctx, number, []string{"agent:claimed"}); err != nil {
		return err
	}
	if err := d.Deps.GitHub.CreateComment(ctx, number, "🤖 Developer agent claiming this issue. Starting analysis..."); err != nil {
		return err
	}
	return nil
}

func (d *DeveloperAgent) analyze(ctx context.Context, issueContext string) (plan string, tooComplex bool, err error) {
	conv := claude.NewConversation(
		d.Deps.Claude,
		SystemPrompt,
		nil, // no tools for analysis
		nil,
		d.Deps.Logger,
		0, // no tools, single-turn — limit doesn't apply
	)

	prompt := fmt.Sprintf(AnalyzePrompt, issueContext)

	// When decomposition is enabled, append complexity estimation to the prompt.
	if d.Deps.Config.Decomposition.Enabled {
		budget := d.Deps.Config.Decomposition.MaxIterationBudget
		threshold := int(float64(budget) * 0.7) // 70% of budget
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

func (d *DeveloperAgent) implement(ctx context.Context, repo *gitops.Repo, issueContext, plan string) error {
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

		case "write_file":
			if err := repo.WriteFile(params["path"], params["content"]); err != nil {
				return "", err
			}
			return "file written successfully", nil

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

