package ghub

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
)

// PRCheckStatus represents the status of PR checks
type PRCheckStatus string

const (
	PRCheckStatusPending   PRCheckStatus = "pending"
	PRCheckStatusRunning   PRCheckStatus = "running"
	PRCheckStatusCompleted PRCheckStatus = "completed"
	PRCheckStatusFailed    PRCheckStatus = "failed"
	PRCheckStatusSuccess   PRCheckStatus = "success"
)

// PRValidationResult contains the result of PR validation
type PRValidationResult struct {
	Status           PRCheckStatus
	AllChecksPassing bool
	FailedChecks     []CheckFailure
	PendingChecks    []string
	TotalChecks      int
}

// CheckFailure contains details about a failed check
type CheckFailure struct {
	Name        string
	Conclusion  string
	Summary     string
	DetailsURL  string
	Annotations []CheckAnnotation
}

// CheckAnnotation contains details about specific failure locations
type CheckAnnotation struct {
	Filename string
	Line     int
	Column   int
	Message  string
	Level    string
}

// PRValidationOptions configures PR validation behavior
type PRValidationOptions struct {
	MaxWaitTime   time.Duration // Maximum time to wait for checks to complete
	PollInterval  time.Duration // Initial polling interval
	MaxRetries    int           // Maximum number of fix attempts
	BackoffFactor float64       // Exponential backoff multiplier
	MaxPollTime   time.Duration // Maximum polling interval
}

// DefaultPRValidationOptions returns sensible defaults for PR validation
func DefaultPRValidationOptions() PRValidationOptions {
	return PRValidationOptions{
		MaxWaitTime:   30 * time.Minute, // Wait up to 30 minutes for checks
		PollInterval:  30 * time.Second, // Start with 30 second polls
		MaxRetries:    3,                // Try up to 3 times to fix failures
		BackoffFactor: 1.5,              // Increase poll time by 1.5x each time
		MaxPollTime:   5 * time.Minute,  // Cap polling at 5 minutes
	}
}

// ValidatePR monitors a pull request until all checks complete successfully
func (c *GitHubClient) ValidatePR(ctx context.Context, prNumber int, opts PRValidationOptions) (*PRValidationResult, error) {
	if opts.MaxWaitTime == 0 {
		opts = DefaultPRValidationOptions()
	}

	timeout := time.After(opts.MaxWaitTime)
	pollInterval := opts.PollInterval

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for PR checks to complete after %v", opts.MaxWaitTime)
		default:
		}

		result, err := c.GetPRCheckStatus(ctx, prNumber)
		if err != nil {
			return nil, fmt.Errorf("getting PR check status: %w", err)
		}

		// If all checks are completed and passing, we're done
		if result.AllChecksPassing && result.Status == PRCheckStatusSuccess {
			return result, nil
		}

		// If checks have failed, return the failure details
		if result.Status == PRCheckStatusFailed {
			return result, nil
		}

		// Still pending or running, wait and try again
		time.Sleep(pollInterval)

		// Exponential backoff with cap
		pollInterval = time.Duration(float64(pollInterval) * opts.BackoffFactor)
		if pollInterval > opts.MaxPollTime {
			pollInterval = opts.MaxPollTime
		}
	}
}

// GetPRCheckStatus retrieves the current status of all checks for a PR
func (c *GitHubClient) GetPRCheckStatus(ctx context.Context, prNumber int) (*PRValidationResult, error) {
	// Get the PR to find the head commit SHA
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("getting PR %d: %w", prNumber, err)
	}

	headSHA := pr.GetHead().GetSHA()
	if headSHA == "" {
		return nil, fmt.Errorf("PR %d has no head commit SHA", prNumber)
	}

	// Get check runs for the head commit
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, headSHA, &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("getting check runs for commit %s: %w", headSHA, err)
	}

	// Get commit status (for legacy status checks)
	commitStatus, _, err := c.client.Repositories.GetCombinedStatus(ctx, c.owner, c.repo, headSHA, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting commit status for %s: %w", headSHA, err)
	}

	return c.analyzeCheckResults(checkRuns.CheckRuns, commitStatus), nil
}

// analyzeCheckResults processes check runs and commit status to determine overall status
func (c *GitHubClient) analyzeCheckResults(checkRuns []*github.CheckRun, commitStatus *github.CombinedStatus) *PRValidationResult {
	result := &PRValidationResult{
		Status:           PRCheckStatusPending,
		AllChecksPassing: true,
		FailedChecks:     []CheckFailure{},
		PendingChecks:    []string{},
		TotalChecks:      len(checkRuns) + len(commitStatus.Statuses),
	}

	// Analyze check runs
	for _, check := range checkRuns {
		checkName := check.GetName()

		switch check.GetStatus() {
		case "queued", "in_progress":
			result.PendingChecks = append(result.PendingChecks, checkName)
			result.Status = PRCheckStatusRunning
			result.AllChecksPassing = false
		case "completed":
			switch check.GetConclusion() {
			case "success", "neutral", "skipped":
				// These are considered passing
			case "failure", "cancelled", "timed_out":
				result.AllChecksPassing = false
				result.Status = PRCheckStatusFailed

				failure := CheckFailure{
					Name:       checkName,
					Conclusion: check.GetConclusion(),
					Summary:    check.GetOutput().GetSummary(),
					DetailsURL: check.GetDetailsURL(),
				}

				// Get annotations if available
				if check.GetOutput() != nil {
					for _, annotation := range check.GetOutput().Annotations {
						failure.Annotations = append(failure.Annotations, CheckAnnotation{
							Filename: annotation.GetPath(),
							Line:     annotation.GetStartLine(),
							Column:   annotation.GetStartColumn(),
							Message:  annotation.GetMessage(),
							Level:    annotation.GetAnnotationLevel(),
						})
					}
				}

				result.FailedChecks = append(result.FailedChecks, failure)
			case "action_required":
				result.PendingChecks = append(result.PendingChecks, checkName+" (action required)")
				result.Status = PRCheckStatusRunning
				result.AllChecksPassing = false
			}
		}
	}

	// Analyze legacy status checks
	for _, status := range commitStatus.Statuses {
		statusName := status.GetContext()

		switch status.GetState() {
		case "pending":
			result.PendingChecks = append(result.PendingChecks, statusName)
			result.Status = PRCheckStatusRunning
			result.AllChecksPassing = false
		case "success":
			// Passing
		case "failure", "error":
			result.AllChecksPassing = false
			result.Status = PRCheckStatusFailed

			failure := CheckFailure{
				Name:       statusName,
				Conclusion: status.GetState(),
				Summary:    status.GetDescription(),
				DetailsURL: status.GetTargetURL(),
			}
			result.FailedChecks = append(result.FailedChecks, failure)
		}
	}

	// Determine final status
	if result.AllChecksPassing && len(result.PendingChecks) == 0 {
		result.Status = PRCheckStatusSuccess
	} else if len(result.FailedChecks) > 0 {
		result.Status = PRCheckStatusFailed
	} else if len(result.PendingChecks) > 0 {
		if result.Status != PRCheckStatusRunning {
			result.Status = PRCheckStatusPending
		}
	}

	return result
}

// AnalyzeFailures generates a human-readable analysis of check failures
func (r *PRValidationResult) AnalyzeFailures() string {
	if len(r.FailedChecks) == 0 {
		return ""
	}

	var analysis strings.Builder
	analysis.WriteString("## Check Failures Analysis\n\n")

	for i, failure := range r.FailedChecks {
		if i > 0 {
			analysis.WriteString("\n---\n\n")
		}

		analysis.WriteString(fmt.Sprintf("### %s (%s)\n\n", failure.Name, failure.Conclusion))

		if failure.Summary != "" {
			analysis.WriteString(fmt.Sprintf("**Summary:** %s\n\n", failure.Summary))
		}

		if len(failure.Annotations) > 0 {
			analysis.WriteString("**Issues found:**\n\n")
			for _, annotation := range failure.Annotations {
				analysis.WriteString(fmt.Sprintf("- `%s:%d`: %s\n",
					annotation.Filename, annotation.Line, annotation.Message))
			}
			analysis.WriteString("\n")
		}

		if failure.DetailsURL != "" {
			analysis.WriteString(fmt.Sprintf("[View detailed logs](%s)\n", failure.DetailsURL))
		}
	}

	return analysis.String()
}

// GenerateFixPrompt creates a prompt for an AI to fix the failed checks
func (r *PRValidationResult) GenerateFixPrompt(issueContext, originalPlan string) string {
	if len(r.FailedChecks) == 0 {
		return ""
	}

	var prompt strings.Builder
	prompt.WriteString("## Pull Request Check Failures\n\n")
	prompt.WriteString("The pull request has failing checks that need to be fixed. Here are the details:\n\n")

	prompt.WriteString(r.AnalyzeFailures())

	prompt.WriteString("\n\n## Original Issue Context\n\n")
	prompt.WriteString(issueContext)

	prompt.WriteString("\n\n## Original Implementation Plan\n\n")
	prompt.WriteString(originalPlan)

	prompt.WriteString("\n\n## Instructions\n\n")
	prompt.WriteString("Please analyze the check failures and fix the issues. Focus on:\n\n")
	prompt.WriteString("1. **Build errors**: Fix compilation issues, missing imports, syntax errors\n")
	prompt.WriteString("2. **Test failures**: Fix failing tests or update test expectations if the behavior change is correct\n")
	prompt.WriteString("3. **Linting issues**: Address code quality issues flagged by linters\n")
	prompt.WriteString("4. **Security issues**: Fix any security vulnerabilities identified\n\n")
	prompt.WriteString("Use the available tools to read, edit, and test files. ")
	prompt.WriteString("Run `go build ./... && go test ./...` to verify fixes before finishing.\n")

	return prompt.String()
}
