package developer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests: parsing ---

func TestParseComplexityResult_No(t *testing.T) {
	response := "Some analysis...\n\n**Fits within budget**: no\n\n## Decomposition Plan..."
	assert.True(t, parseComplexityResult(response))
}

func TestParseComplexityResult_Yes(t *testing.T) {
	response := "Some analysis...\n\n**Fits within budget**: yes"
	assert.False(t, parseComplexityResult(response))
}

func TestParseComplexityResult_Missing(t *testing.T) {
	response := "Some analysis with no budget line at all."
	assert.False(t, parseComplexityResult(response))
}

func TestParseComplexityResult_CaseInsensitive(t *testing.T) {
	response := "**Fits Within Budget**: No"
	assert.True(t, parseComplexityResult(response))
}

func TestParseComplexityResult_PlainText(t *testing.T) {
	response := "Fits within budget: no"
	assert.True(t, parseComplexityResult(response))
}

func TestParseSubtasks(t *testing.T) {
	text := `## Decomposition Plan

### Subtask 1: Add user model
Create the User struct in models.go with basic fields.

### Subtask 2: Add user handler
Create the HTTP handler for user CRUD operations.

### Subtask 3: Add user tests
Write unit tests for the user model and handler.`

	subtasks := parseSubtasks(text)
	require.Len(t, subtasks, 3)

	assert.Equal(t, "Add user model", subtasks[0].Title)
	assert.Contains(t, subtasks[0].Body, "Create the User struct")

	assert.Equal(t, "Add user handler", subtasks[1].Title)
	assert.Contains(t, subtasks[1].Body, "HTTP handler")

	assert.Equal(t, "Add user tests", subtasks[2].Title)
	assert.Contains(t, subtasks[2].Body, "unit tests")
}

func TestParseSubtasks_Empty(t *testing.T) {
	text := "No subtask markers here."
	subtasks := parseSubtasks(text)
	assert.Nil(t, subtasks)
}

func TestParseSubtasks_Malformed(t *testing.T) {
	text := `## Decomposition Plan

### Subtask 1: Only one subtask
This is the only subtask body.`

	subtasks := parseSubtasks(text)
	require.Len(t, subtasks, 1)
	assert.Equal(t, "Only one subtask", subtasks[0].Title)
	assert.Contains(t, subtasks[0].Body, "only subtask body")
}

func TestParseParentIssue_Found(t *testing.T) {
	body := "Parent issue: #42\n\nSome description here."
	assert.Equal(t, 42, parseParentIssue(body))
}

func TestParseParentIssue_NotFound(t *testing.T) {
	body := "No parent reference here."
	assert.Equal(t, 0, parseParentIssue(body))
}

func TestParseParentIssue_MultipleDigits(t *testing.T) {
	body := "Parent issue: #123\n\nDescription."
	assert.Equal(t, 123, parseParentIssue(body))
}

func TestFormatIssueLinks(t *testing.T) {
	assert.Equal(t, "#1, #2, #3", formatIssueLinks([]int{1, 2, 3}))
}

func TestFormatIssueLinks_Single(t *testing.T) {
	assert.Equal(t, "#42", formatIssueLinks([]int{42}))
}

func TestFormatIssueLinks_Empty(t *testing.T) {
	assert.Equal(t, "", formatIssueLinks([]int{}))
}

// --- parseEstimatedIterations tests ---

func TestParseEstimatedIterations_Found(t *testing.T) {
	response := "Some analysis...\n\n**Estimated iterations**: 12\n\n**Fits within budget**: yes"
	assert.Equal(t, 12, parseEstimatedIterations(response))
}

func TestParseEstimatedIterations_NotFound(t *testing.T) {
	response := "Some analysis without an estimate."
	assert.Equal(t, 0, parseEstimatedIterations(response))
}

func TestParseEstimatedIterations_PlainText(t *testing.T) {
	response := "Estimated iterations: 25"
	assert.Equal(t, 25, parseEstimatedIterations(response))
}

func TestParseEstimatedIterations_CaseInsensitive(t *testing.T) {
	response := "**estimated iterations**: 8"
	assert.Equal(t, 8, parseEstimatedIterations(response))
}

// --- formatSubtaskBreakdown tests ---

func TestFormatSubtaskBreakdown(t *testing.T) {
	sts := []subtask{
		{Title: "Add model", Body: "Create the User struct in models.go."},
		{Title: "Add handler", Body: "Create HTTP handler for CRUD."},
	}
	result := formatSubtaskBreakdown(sts, []int{101, 102})
	assert.Contains(t, result, "#101")
	assert.Contains(t, result, "**Add model**")
	assert.Contains(t, result, "#102")
	assert.Contains(t, result, "**Add handler**")
}

func TestFormatSubtaskBreakdown_MoreSubtasksThanNums(t *testing.T) {
	sts := []subtask{
		{Title: "First", Body: "Do first thing."},
		{Title: "Second", Body: "Do second thing."},
	}
	result := formatSubtaskBreakdown(sts, []int{101})
	assert.Contains(t, result, "#101")
	assert.Contains(t, result, "**Second**")
	// Second subtask should not have an issue number.
	assert.NotContains(t, result, "#102")
}

func TestFormatSubtaskBreakdown_Empty(t *testing.T) {
	result := formatSubtaskBreakdown(nil, nil)
	assert.Equal(t, "", result)
}

// --- firstLine tests ---

func TestFirstLine_Normal(t *testing.T) {
	assert.Equal(t, "Hello world", firstLine("Hello world\nSecond line"))
}

func TestFirstLine_Empty(t *testing.T) {
	assert.Equal(t, "", firstLine(""))
}

func TestFirstLine_LeadingNewlines(t *testing.T) {
	assert.Equal(t, "Content here", firstLine("\n\n  Content here\nMore"))
}

// --- extractFilePaths tests ---

func TestExtractFilePaths_GoFiles(t *testing.T) {
	plan := `## Files to modify
1. internal/developer/workflow.go - add self-assign logic
2. internal/ghub/client.go - update interface
3. internal/developer/workflow_test.go - add tests`
	paths := extractFilePaths(plan)
	assert.Contains(t, paths, "internal/developer/workflow.go")
	assert.Contains(t, paths, "internal/ghub/client.go")
	assert.Contains(t, paths, "internal/developer/workflow_test.go")
}

func TestExtractFilePaths_NoDuplicates(t *testing.T) {
	plan := "Modify internal/developer/workflow.go and also update internal/developer/workflow.go"
	paths := extractFilePaths(plan)
	count := 0
	for _, p := range paths {
		if p == "internal/developer/workflow.go" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestExtractFilePaths_NoMatches(t *testing.T) {
	plan := "This plan mentions no Go files at all."
	paths := extractFilePaths(plan)
	assert.Empty(t, paths)
}

func TestExtractFilePaths_BacktickPaths(t *testing.T) {
	plan := "Update `internal/config/config.go` with the new field."
	paths := extractFilePaths(plan)
	assert.Contains(t, paths, "internal/config/config.go")
}

func TestHasLabel_Match(t *testing.T) {
	issue := &github.Issue{
		Labels: []*github.Label{
			{Name: github.Ptr("bug")},
			{Name: github.Ptr("agent:ready")},
		},
	}
	assert.True(t, hasLabel(issue, "agent:ready"))
}

func TestHasLabel_NoMatch(t *testing.T) {
	issue := &github.Issue{
		Labels: []*github.Label{
			{Name: github.Ptr("bug")},
		},
	}
	assert.False(t, hasLabel(issue, "agent:ready"))
}

func TestHasLabel_NoLabels(t *testing.T) {
	issue := &github.Issue{}
	assert.False(t, hasLabel(issue, "agent:ready"))
}

// --- Enhanced mock for integration tests ---

type trackingMockGitHub struct {
	createdIssues   []*github.Issue
	addedLabels     map[int][]string
	removedLabels   map[int][]string
	comments        map[int][]string
	issueCounter    int
	issues          map[int]*github.Issue
	failProcessNums map[int]bool // issue numbers that should fail when fetched
}

func newTrackingMock() *trackingMockGitHub {
	return &trackingMockGitHub{
		addedLabels:     make(map[int][]string),
		removedLabels:   make(map[int][]string),
		comments:        make(map[int][]string),
		issues:          make(map[int]*github.Issue),
		failProcessNums: make(map[int]bool),
		issueCounter:    100,
	}
}

func (m *trackingMockGitHub) ListIssues(_ context.Context, _ []string) ([]*github.Issue, error) {
	return nil, nil
}

func (m *trackingMockGitHub) GetIssue(_ context.Context, number int) (*github.Issue, error) {
	if m.failProcessNums[number] {
		return nil, fmt.Errorf("mock fetch error for issue %d", number)
	}
	if issue, ok := m.issues[number]; ok {
		return issue, nil
	}
	return nil, fmt.Errorf("issue %d not found", number)
}

func (m *trackingMockGitHub) AssignIssue(_ context.Context, _ int, _ []string) error { return nil }

func (m *trackingMockGitHub) AddLabels(_ context.Context, number int, labels []string) error {
	m.addedLabels[number] = append(m.addedLabels[number], labels...)
	return nil
}

func (m *trackingMockGitHub) RemoveLabel(_ context.Context, number int, label string) error {
	m.removedLabels[number] = append(m.removedLabels[number], label)
	return nil
}

func (m *trackingMockGitHub) CreateIssue(_ context.Context, title, body string, labels []string) (*github.Issue, error) {
	m.issueCounter++
	num := m.issueCounter
	issue := &github.Issue{
		Number: github.Ptr(num),
		Title:  github.Ptr(title),
		Body:   github.Ptr(body),
	}
	for _, l := range labels {
		issue.Labels = append(issue.Labels, &github.Label{Name: github.Ptr(l)})
	}
	m.createdIssues = append(m.createdIssues, issue)
	m.issues[num] = issue
	return issue, nil
}

func (m *trackingMockGitHub) CreateBranch(_ context.Context, _ string, _ string) error { return nil }

func (m *trackingMockGitHub) CreatePR(_ context.Context, _ ghub.PROptions) (*github.PullRequest, error) {
	return &github.PullRequest{Number: github.Ptr(999)}, nil
}

func (m *trackingMockGitHub) ListPRs(_ context.Context, _ string) ([]*github.PullRequest, error) {
	return nil, nil
}

func (m *trackingMockGitHub) CreateComment(_ context.Context, number int, body string) error {
	m.comments[number] = append(m.comments[number], body)
	return nil
}

func (m *trackingMockGitHub) ListComments(_ context.Context, _ int) ([]*github.IssueComment, error) {
	return nil, nil
}

// --- Integration helpers ---

func newTestAgent(t *testing.T, gh ghub.Client, decompEnabled bool) *DeveloperAgent {
	t.Helper()
	dir := t.TempDir()
	store, err := state.NewFileStore(dir)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	deps := agent.Dependencies{
		Config: &config.Config{
			GitHub: config.GitHubConfig{
				Token:        "test-token",
				Owner:        "testowner",
				Repo:         "testrepo",
				PollInterval: 30 * time.Second,
				WatchLabels:  []string{"agent:ready"},
			},
			Claude: config.ClaudeConfig{
				APIKey:    "test-key",
				Model:     "claude-sonnet-4-20250514",
				MaxTokens: 4096,
			},
			Agents: config.AgentsConfig{
				Developer: config.DeveloperAgentConfig{
					Enabled:       true,
					MaxConcurrent: 1,
					WorkspaceDir:  dir,
				},
			},
			Decomposition: config.DecompositionConfig{
				Enabled:            decompEnabled,
				MaxIterationBudget: 15,
				MaxSubtasks:        5,
			},
		},
		GitHub: gh,
		Claude: claude.NewClient("test-key", "claude-sonnet-4-20250514", 4096),
		Store:  store,
		Logger: logger,
	}

	da := &DeveloperAgent{
		BaseAgent: agent.NewBaseAgent(deps),
		status: agent.StatusReport{
			Type:    agent.TypeDeveloper,
			State:   string(state.StateIdle),
			Message: "waiting for issues",
		},
	}
	return da
}

// --- Integration tests ---

func TestCreateChildIssues(t *testing.T) {
	mock := newTrackingMock()
	da := newTestAgent(t, mock, true)

	subtasks := []subtask{
		{Title: "First task", Body: "Do first thing"},
		{Title: "Second task", Body: "Do second thing"},
	}

	nums, err := da.createChildIssues(context.Background(), 42, subtasks)
	require.NoError(t, err)
	require.Len(t, nums, 2)

	// Verify issues were created with correct content.
	require.Len(t, mock.createdIssues, 2)
	assert.Equal(t, "First task", mock.createdIssues[0].GetTitle())
	assert.Contains(t, mock.createdIssues[0].GetBody(), "Parent issue: #42")
	assert.Contains(t, mock.createdIssues[0].GetBody(), "Do first thing")
}

func TestProcessChildIssues_ContinuesOnFailure(t *testing.T) {
	mock := newTrackingMock()

	// Pre-populate issues. Issue 101 will fail to fetch, 102 is normal.
	mock.issueCounter = 100
	mock.issues[101] = &github.Issue{
		Number: github.Ptr(101),
		Title:  github.Ptr("Child 1"),
		Labels: []*github.Label{{Name: github.Ptr("agent:ready")}},
	}
	mock.failProcessNums[101] = true // simulate fetch failure

	mock.issues[102] = &github.Issue{
		Number: github.Ptr(102),
		Title:  github.Ptr("Child 2"),
		Labels: []*github.Label{{Name: github.Ptr("agent:ready")}},
	}

	da := newTestAgent(t, mock, true)

	// processChildIssues should not return an error even when children fail.
	err := da.processChildIssues(context.Background(), []int{101, 102}, 10)
	assert.NoError(t, err)

	// A summary comment should be posted on the parent.
	require.NotEmpty(t, mock.comments[10])
}

func TestProcessChildIssues_SkipsWithoutReadyLabel(t *testing.T) {
	mock := newTrackingMock()

	mock.issues[101] = &github.Issue{
		Number: github.Ptr(101),
		Title:  github.Ptr("Child without ready"),
		Labels: []*github.Label{{Name: github.Ptr("bug")}}, // no agent:ready
	}

	da := newTestAgent(t, mock, true)

	err := da.processChildIssues(context.Background(), []int{101}, 10)
	assert.NoError(t, err)

	// Summary should still be posted.
	require.NotEmpty(t, mock.comments[10])
}

func TestProcessChildIssues_PostsProgressComments(t *testing.T) {
	mock := newTrackingMock()

	// Pre-populate two child issues (both will fail to process since they trigger
	// full processIssue, but we only care about the progress comments).
	mock.issues[101] = &github.Issue{
		Number: github.Ptr(101),
		Title:  github.Ptr("Child 1"),
		Labels: []*github.Label{{Name: github.Ptr("bug")}}, // no agent:ready, will be skipped
	}
	mock.issues[102] = &github.Issue{
		Number: github.Ptr(102),
		Title:  github.Ptr("Child 2"),
		Labels: []*github.Label{{Name: github.Ptr("bug")}}, // no agent:ready, will be skipped
	}

	da := newTestAgent(t, mock, true)

	err := da.processChildIssues(context.Background(), []int{101, 102}, 10)
	assert.NoError(t, err)

	// Should have progress comments for each child + final summary = 3 comments on parent.
	require.Len(t, mock.comments[10], 3)
	assert.Contains(t, mock.comments[10][0], "Processing subtask 1/2: #101")
	assert.Contains(t, mock.comments[10][1], "Processing subtask 2/2: #102")
}
