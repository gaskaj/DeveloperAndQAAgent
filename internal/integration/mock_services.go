package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/google/go-github/v68/github"
)

// MockGitHubClient implements ghub.Client interface for testing
type MockGitHubClient struct {
	mu             sync.RWMutex
	issues         map[int]*MockIssue
	comments       map[int][]*MockComment
	prs            []*MockPR
	simulatedError error
	assignees      map[int][]string
}

// MockIssue represents a mock GitHub issue
type MockIssue struct {
	Number    int
	Title     string
	Body      string
	Labels    []string
	Assignees []string
	State     string
}

// MockComment represents a mock GitHub comment
type MockComment struct {
	Body      string
	CreatedAt time.Time
}

// MockPR represents a mock GitHub pull request
type MockPR struct {
	Number  int
	Title   string
	Body    string
	Head    string
	Base    string
	State   string
	HeadSHA string
}

func (mc *MockComment) Contains(substring string) bool {
	return strings.Contains(mc.Body, substring)
}

// NewMockGitHubClient creates a new mock GitHub client
func NewMockGitHubClient() *MockGitHubClient {
	return &MockGitHubClient{
		issues:    make(map[int]*MockIssue),
		comments:  make(map[int][]*MockComment),
		assignees: make(map[int][]string),
	}
}

func (m *MockGitHubClient) AddIssue(issue *MockIssue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issues[issue.Number] = issue
}

func (m *MockGitHubClient) GetMockIssue(number int) *MockIssue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.issues[number]
}

func (m *MockGitHubClient) GetIssueLabels(number int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if issue, ok := m.issues[number]; ok {
		return issue.Labels
	}
	return nil
}

func (m *MockGitHubClient) GetIssueComments(number int) []*MockComment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.comments[number]
}

func (m *MockGitHubClient) GetCreatedPRs() []*MockPR {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prs
}

func (m *MockGitHubClient) SimulateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulatedError = err
}

func (m *MockGitHubClient) ClearError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulatedError = nil
}

// Implement ghub.Client interface
// ListIssues is an alias for ListIssuesByLabels for interface compatibility
func (m *MockGitHubClient) ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error) {
	return m.ListIssuesByLabels(ctx, labels)
}

func (m *MockGitHubClient) ListIssuesByState(_ context.Context, _ []string, _ string) ([]*github.Issue, error) {
	return nil, nil
}

func (m *MockGitHubClient) ListIssuesByLabels(ctx context.Context, labels []string) ([]*github.Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	var result []*github.Issue
	for _, issue := range m.issues {
		// Check if issue has all required labels
		hasAllLabels := true
		for _, requiredLabel := range labels {
			found := false
			for _, issueLabel := range issue.Labels {
				if issueLabel == requiredLabel {
					found = true
					break
				}
			}
			if !found {
				hasAllLabels = false
				break
			}
		}

		if hasAllLabels {
			// Convert MockIssue to github.Issue
			number := issue.Number
			title := issue.Title
			body := issue.Body

			var githubLabels []*github.Label
			for _, labelName := range issue.Labels {
				name := labelName
				githubLabels = append(githubLabels, &github.Label{Name: &name})
			}

			var githubAssignees []*github.User
			for _, assignee := range issue.Assignees {
				login := assignee
				githubAssignees = append(githubAssignees, &github.User{Login: &login})
			}

			state := issue.State
			if state == "" {
				state = "open"
			}

			result = append(result, &github.Issue{
				Number:    &number,
				Title:     &title,
				Body:      &body,
				Labels:    githubLabels,
				Assignees: githubAssignees,
				State:     &state,
			})
		}
	}

	return result, nil
}

func (m *MockGitHubClient) AddLabels(ctx context.Context, number int, labels []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	if issue, ok := m.issues[number]; ok {
		// Add labels if they don't exist
		for _, newLabel := range labels {
			found := false
			for _, existingLabel := range issue.Labels {
				if existingLabel == newLabel {
					found = true
					break
				}
			}
			if !found {
				issue.Labels = append(issue.Labels, newLabel)
			}
		}
	}

	return nil
}

func (m *MockGitHubClient) RemoveLabel(ctx context.Context, number int, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	if issue, ok := m.issues[number]; ok {
		// Remove label if it exists
		var newLabels []string
		for _, existingLabel := range issue.Labels {
			if existingLabel != label {
				newLabels = append(newLabels, existingLabel)
			}
		}
		issue.Labels = newLabels
	}

	return nil
}

func (m *MockGitHubClient) CreateComment(ctx context.Context, number int, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	comment := &MockComment{
		Body:      body,
		CreatedAt: time.Now(),
	}
	m.comments[number] = append(m.comments[number], comment)

	return nil
}

func (m *MockGitHubClient) AssignSelfIfNoAssignees(ctx context.Context, number int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	if issue, ok := m.issues[number]; ok {
		if len(issue.Assignees) == 0 {
			issue.Assignees = []string{"test-agent"}
		}
	}

	return nil
}

func (m *MockGitHubClient) AssignIssue(ctx context.Context, number int, assignees []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	if issue, ok := m.issues[number]; ok {
		issue.Assignees = assignees
	}

	return nil
}

func (m *MockGitHubClient) GetIssue(ctx context.Context, number int) (*github.Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	if issue, ok := m.issues[number]; ok {
		// Convert MockIssue to github.Issue
		num := issue.Number
		title := issue.Title
		body := issue.Body
		state := issue.State
		if state == "" {
			state = "open"
		}

		var githubLabels []*github.Label
		for _, labelName := range issue.Labels {
			name := labelName
			githubLabels = append(githubLabels, &github.Label{Name: &name})
		}

		var githubAssignees []*github.User
		for _, assignee := range issue.Assignees {
			login := assignee
			githubAssignees = append(githubAssignees, &github.User{Login: &login})
		}

		return &github.Issue{
			Number:    &num,
			Title:     &title,
			Body:      &body,
			Labels:    githubLabels,
			Assignees: githubAssignees,
			State:     &state,
		}, nil
	}

	return nil, fmt.Errorf("issue %d not found", number)
}

func (m *MockGitHubClient) CreateBranch(ctx context.Context, name string, fromRef string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	// Mock implementation - just track that it was called
	return nil
}

func (m *MockGitHubClient) ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	var result []*github.PullRequest
	for _, pr := range m.prs {
		if state == "" || pr.State == state {
			number := pr.Number
			title := pr.Title
			body := pr.Body
			head := pr.Head
			base := pr.Base
			prState := pr.State

			result = append(result, &github.PullRequest{
				Number: &number,
				Title:  &title,
				Body:   &body,
				Head: &github.PullRequestBranch{
					Ref: &head,
				},
				Base: &github.PullRequestBranch{
					Ref: &base,
				},
				State: &prState,
			})
		}
	}

	return result, nil
}

func (m *MockGitHubClient) CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	number := len(m.issues) + 1000 // Use high numbers to avoid conflicts
	issue := &MockIssue{
		Number: number,
		Title:  title,
		Body:   body,
		Labels: labels,
		State:  "open",
	}
	m.issues[number] = issue

	// Convert to github.Issue
	num := issue.Number
	issueTitle := issue.Title
	issueBody := issue.Body
	state := issue.State

	var githubLabels []*github.Label
	for _, labelName := range issue.Labels {
		name := labelName
		githubLabels = append(githubLabels, &github.Label{Name: &name})
	}

	return &github.Issue{
		Number: &num,
		Title:  &issueTitle,
		Body:   &issueBody,
		Labels: githubLabels,
		State:  &state,
	}, nil
}

func (m *MockGitHubClient) ListComments(ctx context.Context, number int) ([]*github.IssueComment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	comments := m.comments[number]
	var result []*github.IssueComment
	for _, comment := range comments {
		body := comment.Body
		createdAt := comment.CreatedAt
		id := int64(len(result) + 1)

		result = append(result, &github.IssueComment{
			ID:        &id,
			Body:      &body,
			CreatedAt: &github.Timestamp{Time: createdAt},
		})
	}

	return result, nil
}

func (m *MockGitHubClient) CreatePR(ctx context.Context, options ghub.PROptions) (*github.PullRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	prNumber := len(m.prs) + 1
	pr := &MockPR{
		Number: prNumber,
		Title:  options.Title,
		Body:   options.Body,
		Head:   options.Head,
		Base:   options.Base,
		State:  "open",
	}
	m.prs = append(m.prs, pr)

	// Convert to github.PullRequest
	number := pr.Number
	title := pr.Title
	body := pr.Body
	head := pr.Head
	base := pr.Base
	state := pr.State

	return &github.PullRequest{
		Number: &number,
		Title:  &title,
		Body:   &body,
		Head: &github.PullRequestBranch{
			Ref: &head,
			SHA: github.Ptr("mock-sha-" + fmt.Sprintf("%d", number)),
		},
		Base: &github.PullRequestBranch{
			Ref: &base,
		},
		State: &state,
	}, nil
}

func (m *MockGitHubClient) GetPR(ctx context.Context, number int) (*github.PullRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	for _, pr := range m.prs {
		if pr.Number == number {
			num := pr.Number
			title := pr.Title
			body := pr.Body
			head := pr.Head
			base := pr.Base
			state := pr.State
			headSHA := pr.HeadSHA
			if headSHA == "" {
				headSHA = "mock-sha-" + fmt.Sprintf("%d", number)
			}

			return &github.PullRequest{
				Number: &num,
				Title:  &title,
				Body:   &body,
				Head: &github.PullRequestBranch{
					Ref: &head,
					SHA: &headSHA,
				},
				Base: &github.PullRequestBranch{
					Ref: &base,
				},
				State: &state,
			}, nil
		}
	}

	return nil, fmt.Errorf("PR %d not found", number)
}

func (m *MockGitHubClient) ValidatePR(ctx context.Context, prNumber int, opts ghub.PRValidationOptions) (*ghub.PRValidationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	// Mock successful validation by default
	return &ghub.PRValidationResult{
		Status:           ghub.PRCheckStatusSuccess,
		AllChecksPassing: true,
		FailedChecks:     []ghub.CheckFailure{},
		PendingChecks:    []string{},
		TotalChecks:      3,
	}, nil
}

func (m *MockGitHubClient) GetPRCheckStatus(ctx context.Context, prNumber int) (*ghub.PRValidationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	return &ghub.PRValidationResult{
		Status:           ghub.PRCheckStatusSuccess,
		AllChecksPassing: true,
		FailedChecks:     []ghub.CheckFailure{},
		PendingChecks:    []string{},
		TotalChecks:      3,
	}, nil
}

// SimpleClaudeClient is a minimal mock that focuses on testing agent behavior.
// It is still available for test-level configuration (e.g. setting responses),
// but the actual HTTP mock is provided by MockClaudeServer.
type SimpleClaudeClient struct {
	mu             sync.RWMutex
	responses      map[string]string
	simulatedError error
	callCount      int
	maxIterations  int
}

func NewSimpleClaudeClient() *SimpleClaudeClient {
	client := &SimpleClaudeClient{
		responses:     make(map[string]string),
		maxIterations: 10,
	}

	// Set default responses
	client.responses["analyze"] = "## Analysis Complete\n\nThis issue requires implementing a new feature.\n\n## Implementation Plan\n\n1. Create new file\n2. Add tests\n3. Update docs"
	client.responses["too_complex"] = "COMPLEXITY_ASSESSMENT: TOO_COMPLEX\nThis issue requires decomposition."
	client.responses["implement"] = "Implementation completed successfully."

	return client
}

func (s *SimpleClaudeClient) SetResponse(input, response string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[input] = response
}

func (s *SimpleClaudeClient) SimulateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulatedError = err
}

func (s *SimpleClaudeClient) ClearError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.simulatedError = nil
}

func (s *SimpleClaudeClient) SetMaxIterations(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxIterations = max
}

func (s *SimpleClaudeClient) GetCallCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.callCount
}

// MockClaudeServer is an httptest.Server that mimics the Anthropic Messages API.
// It detects whether the request includes tool definitions (implement step) and
// returns an appropriate tool_use or text response. A response queue allows tests
// to override responses for specific calls (e.g. decompose analysis).
type MockClaudeServer struct {
	server       *httptest.Server
	callCount    atomic.Int64
	toolUseCount atomic.Int64
	mu           sync.RWMutex
	responseText string
	// responseQueue is a FIFO queue of text responses. When non-empty, the
	// front element is consumed instead of responseText for non-tool requests.
	responseQueue []string
	httpErrorCode int
	httpErrorBody string
}

// NewMockClaudeServer starts a mock HTTP server that handles /v1/messages.
func NewMockClaudeServer() *MockClaudeServer {
	m := &MockClaudeServer{
		responseText: "## Analysis Complete\n\nThis issue requires implementing a new feature.\n\n## Implementation Plan\n\n1. Create new file\n2. Add tests\n3. Update docs\n\n**Estimated iterations**: 5\n**Fits within budget**: yes",
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.callCount.Add(1)

		// Parse request body to detect tools and tool_result messages.
		var reqBody struct {
			Tools    []json.RawMessage `json:"tools"`
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)

		m.mu.RLock()
		errCode := m.httpErrorCode
		errBody := m.httpErrorBody
		m.mu.RUnlock()

		if errCode != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errCode)
			errResp := map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": errBody,
				},
			}
			json.NewEncoder(w).Encode(errResp)
			return
		}

		// Detect whether tools are present (implement step) and whether the
		// conversation already contains a tool_result (follow-up after tool execution).
		hasTools := len(reqBody.Tools) > 0
		hasToolResult := false
		for _, msg := range reqBody.Messages {
			var blocks []struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(msg.Content, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "tool_result" {
						hasToolResult = true
					}
				}
			}
		}

		// First request with tools (no tool_result yet): return a tool_use
		// response that writes a file so the commit step has something to stage.
		if hasTools && !hasToolResult {
			count := m.toolUseCount.Add(1)
			resp := map[string]interface{}{
				"id":    fmt.Sprintf("msg_test_%d", m.callCount.Load()),
				"type":  "message",
				"role":  "assistant",
				"model": "claude-3-haiku-20240307",
				"content": []map[string]interface{}{
					{
						"type": "tool_use",
						"id":   fmt.Sprintf("toolu_test_%d", count),
						"name": "write_file",
						"input": map[string]interface{}{
							"path":    "implementation.go",
							"content": "package main\n\n// Auto-generated implementation\nfunc main() {}\n",
						},
					},
				},
				"stop_reason": "tool_use",
				"usage": map[string]interface{}{
					"input_tokens":  100,
					"output_tokens": 50,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// All other requests: return a text response from queue or default.
		m.mu.Lock()
		var text string
		if len(m.responseQueue) > 0 {
			text = m.responseQueue[0]
			m.responseQueue = m.responseQueue[1:]
		} else {
			text = m.responseText
		}
		m.mu.Unlock()

		resp := map[string]interface{}{
			"id":    fmt.Sprintf("msg_test_%d", m.callCount.Load()),
			"type":  "message",
			"role":  "assistant",
			"model": "claude-3-haiku-20240307",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return m
}

// SetResponse changes the text returned by the mock server.
func (m *MockClaudeServer) SetResponse(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseText = text
	m.httpErrorCode = 0
	m.httpErrorBody = ""
}

// SetHTTPError makes the mock server return the given HTTP error code.
func (m *MockClaudeServer) SetHTTPError(code int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpErrorCode = code
	m.httpErrorBody = message
}

// EnqueueResponse adds a text response to the FIFO queue. Queued responses
// are consumed before the default responseText for non-tool requests.
func (m *MockClaudeServer) EnqueueResponse(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseQueue = append(m.responseQueue, text)
}

// ClearHTTPError removes any configured HTTP error so subsequent requests succeed.
func (m *MockClaudeServer) ClearHTTPError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpErrorCode = 0
	m.httpErrorBody = ""
}

// RequestOption returns an option.WithBaseURL pointing at the mock server.
func (m *MockClaudeServer) RequestOption() option.RequestOption {
	return option.WithBaseURL(m.server.URL)
}

// Close shuts down the mock server.
func (m *MockClaudeServer) Close() {
	m.server.Close()
}

// CallCount returns how many requests the mock server has received.
func (m *MockClaudeServer) CallCount() int64 {
	return m.callCount.Load()
}

// MockStore implements state.Store interface for testing
type MockStore struct {
	mu             sync.RWMutex
	states         map[string]*state.AgentWorkState
	simulatedError error
}

func NewMockStore() *MockStore {
	return &MockStore{
		states: make(map[string]*state.AgentWorkState),
	}
}

func (m *MockStore) SimulateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulatedError = err
}

func (m *MockStore) ClearError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulatedError = nil
}

// Implement state.Store interface
func (m *MockStore) Save(ctx context.Context, agentState *state.AgentWorkState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	// Create a copy to avoid race conditions
	stateCopy := *agentState
	m.states[agentState.AgentType] = &stateCopy
	return nil
}

func (m *MockStore) Load(ctx context.Context, agentType string) (*state.AgentWorkState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	if state, ok := m.states[agentType]; ok {
		// Return a copy to avoid race conditions
		stateCopy := *state
		return &stateCopy, nil
	}

	return nil, nil
}

func (m *MockStore) Delete(ctx context.Context, agentType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simulatedError != nil {
		return m.simulatedError
	}

	delete(m.states, agentType)
	return nil
}

func (m *MockStore) List(ctx context.Context) ([]*state.AgentWorkState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.simulatedError != nil {
		return nil, m.simulatedError
	}

	var result []*state.AgentWorkState
	for _, state := range m.states {
		// Return copies to avoid race conditions
		stateCopy := *state
		result = append(result, &stateCopy)
	}

	return result, nil
}
