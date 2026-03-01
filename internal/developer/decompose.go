package developer

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/google/go-github/v68/github"
)

// subtask represents a parsed subtask from Claude's decomposition output.
type subtask struct {
	Title string
	Body  string
}

// parseComplexityResult scans the analysis response for "Fits within budget: no".
// Returns true if the issue is too complex (does not fit within budget).
func parseComplexityResult(response string) bool {
	lower := strings.ToLower(response)
	return strings.Contains(lower, "**fits within budget**: no") ||
		strings.Contains(lower, "fits within budget: no")
}

// parseSubtasks extracts subtasks from text formatted with "### Subtask N: <title>" markers.
func parseSubtasks(text string) []subtask {
	re := regexp.MustCompile(`(?m)^###\s+Subtask\s+\d+:\s*(.+)$`)
	matches := re.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		return nil
	}

	var subtasks []subtask
	for i, match := range matches {
		title := strings.TrimSpace(text[match[2]:match[3]])

		// Body extends from end of this header line to start of next header (or end of text).
		bodyStart := match[1]
		var bodyEnd int
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(text)
		}

		body := strings.TrimSpace(text[bodyStart:bodyEnd])
		subtasks = append(subtasks, subtask{Title: title, Body: body})
	}

	return subtasks
}

// parseParentIssue extracts the parent issue number from a "Parent issue: #N" line.
func parseParentIssue(body string) int {
	re := regexp.MustCompile(`Parent issue:\s*#(\d+)`)
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}

// createChildIssues creates GitHub issues for each subtask and returns their issue numbers.
func (d *DeveloperAgent) createChildIssues(ctx context.Context, parentNum int, subtasks []subtask) ([]int, error) {
	var nums []int
	for _, st := range subtasks {
		body := fmt.Sprintf("Parent issue: #%d\n\n%s", parentNum, st.Body)
		issue, err := d.Deps.GitHub.CreateIssue(ctx, st.Title, body, []string{"agent:subtask", "agent:ready"})
		if err != nil {
			return nums, fmt.Errorf("creating child issue %q: %w", st.Title, err)
		}
		nums = append(nums, issue.GetNumber())
	}
	return nums, nil
}

// formatIssueLinks formats issue numbers as "#1, #2, #3".
func formatIssueLinks(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("#%d", n)
	}
	return strings.Join(parts, ", ")
}

// decompose handles proactive decomposition: parses the existing plan for subtasks
// or makes a separate Claude call, then creates child issues.
func (d *DeveloperAgent) decompose(ctx context.Context, issueNum int, issueContext, plan string) ([]int, error) {
	d.updateStatus(state.StateDecompose, issueNum, "breaking issue into subtasks")

	// Try to parse subtasks from the existing plan first.
	subtasks := parseSubtasks(plan)

	// If no subtasks found in the plan, make a separate Claude call.
	if len(subtasks) == 0 {
		conv := claude.NewConversation(
			d.Deps.Claude,
			SystemPrompt,
			nil,
			nil,
			d.Deps.Logger,
		)

		maxSubtasks := d.Deps.Config.Decomposition.MaxSubtasks
		budget := d.Deps.Config.Decomposition.MaxIterationBudget
		prompt := fmt.Sprintf(DecomposePrompt, budget, issueContext, plan, maxSubtasks)
		response, err := conv.Send(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("decomposition call failed: %w", err)
		}
		subtasks = parseSubtasks(response)
	}

	if len(subtasks) == 0 {
		return nil, fmt.Errorf("decomposition produced no subtasks")
	}

	// Cap at MaxSubtasks.
	maxSubtasks := d.Deps.Config.Decomposition.MaxSubtasks
	if len(subtasks) > maxSubtasks {
		subtasks = subtasks[:maxSubtasks]
	}

	// Create child issues.
	childNums, err := d.createChildIssues(ctx, issueNum, subtasks)
	if err != nil {
		return childNums, err
	}

	// Label parent as epic, remove agent:ready.
	_ = d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:epic"})
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:ready")

	// Post summary comment.
	comment := fmt.Sprintf(
		"🤖 This issue has been decomposed into %d subtasks: %s\n\nEach subtask will be processed independently.",
		len(childNums), formatIssueLinks(childNums),
	)
	_ = d.Deps.GitHub.CreateComment(ctx, issueNum, comment)

	return childNums, nil
}

// reactiveDecompose handles decomposition after the iteration limit is hit at runtime.
func (d *DeveloperAgent) reactiveDecompose(ctx context.Context, issueNum int, issueContext, plan string) ([]int, error) {
	d.updateStatus(state.StateDecompose, issueNum, "decomposing remaining work after iteration limit")

	conv := claude.NewConversation(
		d.Deps.Claude,
		SystemPrompt,
		nil,
		nil,
		d.Deps.Logger,
	)

	maxSubtasks := d.Deps.Config.Decomposition.MaxSubtasks
	prompt := fmt.Sprintf(ReactiveDecomposePrompt, issueContext, plan, maxSubtasks)
	response, err := conv.Send(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("reactive decomposition call failed: %w", err)
	}

	subtasks := parseSubtasks(response)
	if len(subtasks) == 0 {
		return nil, fmt.Errorf("reactive decomposition produced no subtasks")
	}

	// Cap at MaxSubtasks.
	if len(subtasks) > maxSubtasks {
		subtasks = subtasks[:maxSubtasks]
	}

	// Create child issues.
	childNums, err := d.createChildIssues(ctx, issueNum, subtasks)
	if err != nil {
		return childNums, err
	}

	// Label parent as epic + failed, remove agent:ready.
	_ = d.Deps.GitHub.AddLabels(ctx, issueNum, []string{"agent:epic", "agent:failed"})
	_ = d.Deps.GitHub.RemoveLabel(ctx, issueNum, "agent:ready")

	// Post summary comment.
	comment := fmt.Sprintf(
		"🤖 Implementation exceeded iteration limit. Remaining work decomposed into %d subtasks: %s",
		len(childNums), formatIssueLinks(childNums),
	)
	_ = d.Deps.GitHub.CreateComment(ctx, issueNum, comment)

	return childNums, nil
}

// processChildIssues fetches and processes each child issue sequentially.
func (d *DeveloperAgent) processChildIssues(ctx context.Context, childNums []int, parentNum int) error {
	var failures []int
	for _, num := range childNums {
		issue, err := d.Deps.GitHub.GetIssue(ctx, num)
		if err != nil {
			d.logger().Error("failed to fetch child issue", "issue", num, "error", err)
			failures = append(failures, num)
			continue
		}

		if !hasLabel(issue, "agent:ready") {
			d.logger().Info("skipping child issue without agent:ready label", "issue", num)
			continue
		}

		if err := d.processIssue(ctx, issue); err != nil {
			d.logger().Error("child issue processing failed", "issue", num, "error", err)
			failures = append(failures, num)
			continue
		}
	}

	// Post summary on parent.
	var comment string
	if len(failures) == 0 {
		comment = fmt.Sprintf("🤖 All %d subtasks completed successfully.", len(childNums))
	} else {
		comment = fmt.Sprintf("🤖 Subtask processing complete. %d/%d succeeded. Failed: %s",
			len(childNums)-len(failures), len(childNums), formatIssueLinks(failures))
	}
	_ = d.Deps.GitHub.CreateComment(ctx, parentNum, comment)

	return nil
}

// hasLabel checks if an issue has a label with the given name.
func hasLabel(issue *github.Issue, name string) bool {
	for _, l := range issue.Labels {
		if l.GetName() == name {
			return true
		}
	}
	return false
}
