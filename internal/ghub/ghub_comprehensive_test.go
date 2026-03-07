package ghub

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers ---

// newTestClient creates a GitHubClient backed by a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) (*GitHubClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL
	return &GitHubClient{
		client: ghClient,
		owner:  "owner",
		repo:   "repo",
	}, server
}

// --- Client Tests ---

func TestWithErrorHandling(t *testing.T) {
	client := NewClient("token", "owner", "repo")
	assert.Nil(t, client.errorManager)

	client.WithErrorHandling(nil)
	assert.Nil(t, client.errorManager)
}

// --- Issues Tests ---

func TestListIssues_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues", r.URL.Path)
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"number": 1, "title": "Issue 1"},
			{"number": 2, "title": "Issue 2"}
		]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssues(context.Background(), []string{"bug"})
	require.NoError(t, err)
	assert.Len(t, issues, 2)
	assert.Equal(t, 1, issues[0].GetNumber())
}

func TestListIssues_FiltersPullRequests(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Return a mix of issues and pull requests
		w.Write([]byte(`[
			{"number": 1, "title": "Real Issue"},
			{"number": 2, "title": "Pull Request", "pull_request": {"url": "https://api.github.com/repos/owner/repo/pulls/2"}}
		]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssues(context.Background(), []string{"bug"})
	require.NoError(t, err)
	assert.Len(t, issues, 1, "Should filter out pull requests")
	assert.Equal(t, 1, issues[0].GetNumber())
}

func TestListIssuesByState_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "closed", r.URL.Query().Get("state"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"number": 10, "title": "Closed Issue"}]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssuesByState(context.Background(), []string{"done"}, "closed")
	require.NoError(t, err)
	assert.Len(t, issues, 1)
}

func TestListIssuesByState_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "server error"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssuesByState(context.Background(), []string{"bug"}, "open")
	assert.Error(t, err)
	assert.Nil(t, issues)
	assert.Contains(t, err.Error(), "listing issues")
}

func TestListIssuesByState_AllState(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "all", r.URL.Query().Get("state"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"number": 1}, {"number": 2}]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssuesByState(context.Background(), []string{}, "all")
	require.NoError(t, err)
	assert.Len(t, issues, 2)
}

func TestGetIssue_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/42", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number": 42, "title": "Test Issue", "body": "Issue body"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issue, err := client.GetIssue(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, 42, issue.GetNumber())
	assert.Equal(t, "Test Issue", issue.GetTitle())
}

func TestGetIssue_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issue, err := client.GetIssue(context.Background(), 999)
	assert.Error(t, err)
	assert.Nil(t, issue)
	assert.Contains(t, err.Error(), "getting issue #999")
}

func TestAssignIssue_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/42/assignees", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number": 42}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.AssignIssue(context.Background(), 42, []string{"user1", "user2"})
	assert.NoError(t, err)
}

func TestAssignIssue_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "forbidden"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.AssignIssue(context.Background(), 42, []string{"user1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assigning issue #42")
}

func TestAddLabels_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/10/labels", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"name": "bug"}, {"name": "priority"}]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.AddLabels(context.Background(), 10, []string{"bug", "priority"})
	assert.NoError(t, err)
}

func TestAddLabels_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.AddLabels(context.Background(), 10, []string{"bug"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adding labels to issue #10")
}

func TestRemoveLabel_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/5/labels/old-label", r.URL.Path)
		assert.Equal(t, "DELETE", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.RemoveLabel(context.Background(), 5, "old-label")
	assert.NoError(t, err)
}

func TestRemoveLabel_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Label not found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.RemoveLabel(context.Background(), 5, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `removing label "nonexistent" from issue #5`)
}

func TestCreateIssue_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number": 100, "title": "New Issue", "body": "Description"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issue, err := client.CreateIssue(context.Background(), "New Issue", "Description", []string{"feature"})
	require.NoError(t, err)
	assert.Equal(t, 100, issue.GetNumber())
	assert.Equal(t, "New Issue", issue.GetTitle())
}

func TestCreateIssue_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message": "Validation Failed"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issue, err := client.CreateIssue(context.Background(), "Title", "Body", []string{})
	assert.Error(t, err)
	assert.Nil(t, issue)
	assert.Contains(t, err.Error(), "creating issue")
}

func TestListIssues_EmptyResult(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	issues, err := client.ListIssues(context.Background(), []string{"nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, issues)
}

// --- Branch Tests ---

func TestCreateBranch_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/git/ref/heads/main" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"ref": "refs/heads/main",
				"object": {"sha": "abc123", "type": "commit"}
			}`))
		case r.URL.Path == "/repos/owner/repo/git/refs" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{
				"ref": "refs/heads/feature-branch",
				"object": {"sha": "abc123", "type": "commit"}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.CreateBranch(context.Background(), "feature-branch", "main")
	assert.NoError(t, err)
}

func TestCreateBranch_SourceRefNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.CreateBranch(context.Background(), "feature", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `getting ref "nonexistent"`)
}

func TestCreateBranch_CreateRefError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/git/ref/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"ref": "refs/heads/main",
				"object": {"sha": "abc123", "type": "commit"}
			}`))
		case r.URL.Path == "/repos/owner/repo/git/refs" && r.Method == "POST":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"message": "Reference already exists"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.CreateBranch(context.Background(), "existing-branch", "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `creating branch "existing-branch"`)
}

// --- Pull Request Tests ---

func TestCreatePR_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"number": 42,
			"title": "Test PR",
			"body": "PR body",
			"html_url": "https://github.com/owner/repo/pull/42"
		}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	pr, err := client.CreatePR(context.Background(), PROptions{
		Title: "Test PR",
		Body:  "PR body",
		Head:  "feature",
		Base:  "main",
	})
	require.NoError(t, err)
	assert.Equal(t, 42, pr.GetNumber())
	assert.Equal(t, "Test PR", pr.GetTitle())
}

func TestCreatePR_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message": "Validation Failed"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	pr, err := client.CreatePR(context.Background(), PROptions{
		Title: "Test PR",
		Body:  "Body",
		Head:  "feature",
		Base:  "main",
	})
	assert.Error(t, err)
	assert.Nil(t, pr)
	assert.Contains(t, err.Error(), "creating pull request")
}

func TestListPRs_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls", r.URL.Path)
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"number": 1, "title": "PR 1"},
			{"number": 2, "title": "PR 2"}
		]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	prs, err := client.ListPRs(context.Background(), "open")
	require.NoError(t, err)
	assert.Len(t, prs, 2)
}

func TestListPRs_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal Server Error"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	prs, err := client.ListPRs(context.Background(), "open")
	assert.Error(t, err)
	assert.Nil(t, prs)
	assert.Contains(t, err.Error(), "listing pull requests")
}

func TestGetPR_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/7", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number": 7, "title": "My PR", "state": "open"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	pr, err := client.GetPR(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, 7, pr.GetNumber())
	assert.Equal(t, "My PR", pr.GetTitle())
}

func TestGetPR_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	pr, err := client.GetPR(context.Background(), 999)
	assert.Error(t, err)
	assert.Nil(t, pr)
	assert.Contains(t, err.Error(), "getting pull request 999")
}

func TestListPRs_EmptyResult(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	prs, err := client.ListPRs(context.Background(), "closed")
	require.NoError(t, err)
	assert.Empty(t, prs)
}

// --- Comment Tests ---

func TestCreateComment_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/15/comments", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1, "body": "Test comment"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.CreateComment(context.Background(), 15, "Test comment")
	assert.NoError(t, err)
}

func TestCreateComment_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Forbidden"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	err := client.CreateComment(context.Background(), 15, "Test comment")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating comment on #15")
}

func TestListComments_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/issues/20/comments", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"id": 1, "body": "First comment"},
			{"id": 2, "body": "Second comment"}
		]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	comments, err := client.ListComments(context.Background(), 20)
	require.NoError(t, err)
	assert.Len(t, comments, 2)
	assert.Equal(t, "First comment", comments[0].GetBody())
}

func TestListComments_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	comments, err := client.ListComments(context.Background(), 999)
	assert.Error(t, err)
	assert.Nil(t, comments)
	assert.Contains(t, err.Error(), "listing comments on #999")
}

func TestListComments_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	client, server := newTestClient(t, handler)
	defer server.Close()

	comments, err := client.ListComments(context.Background(), 5)
	require.NoError(t, err)
	assert.Empty(t, comments)
}

// --- Poller Tests ---

// mockClient implements the Client interface for testing the Poller.
type mockClient struct {
	listIssuesFunc func(ctx context.Context, labels []string) ([]*github.Issue, error)
}

func (m *mockClient) ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error) {
	return m.listIssuesFunc(ctx, labels)
}
func (m *mockClient) ListIssuesByState(ctx context.Context, labels []string, state string) ([]*github.Issue, error) {
	return nil, nil
}
func (m *mockClient) GetIssue(ctx context.Context, number int) (*github.Issue, error) {
	return nil, nil
}
func (m *mockClient) AssignIssue(ctx context.Context, number int, assignees []string) error {
	return nil
}
func (m *mockClient) AssignSelfIfNoAssignees(ctx context.Context, number int) error {
	return nil
}
func (m *mockClient) AddLabels(ctx context.Context, number int, labels []string) error {
	return nil
}
func (m *mockClient) RemoveLabel(ctx context.Context, number int, label string) error {
	return nil
}
func (m *mockClient) CreateBranch(ctx context.Context, name string, fromRef string) error {
	return nil
}
func (m *mockClient) CreatePR(ctx context.Context, opts PROptions) (*github.PullRequest, error) {
	return nil, nil
}
func (m *mockClient) ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error) {
	return nil, nil
}
func (m *mockClient) GetPR(ctx context.Context, number int) (*github.PullRequest, error) {
	return nil, nil
}
func (m *mockClient) ValidatePR(ctx context.Context, prNumber int, opts PRValidationOptions) (*PRValidationResult, error) {
	return nil, nil
}
func (m *mockClient) GetPRCheckStatus(ctx context.Context, prNumber int) (*PRValidationResult, error) {
	return nil, nil
}
func (m *mockClient) MergePR(ctx context.Context, prNumber int, commitMessage string) error {
	return nil
}
func (m *mockClient) CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error) {
	return nil, nil
}
func (m *mockClient) CreateComment(ctx context.Context, number int, body string) error {
	return nil
}
func (m *mockClient) ListComments(ctx context.Context, number int) ([]*github.IssueComment, error) {
	return nil, nil
}

func TestNewPoller(t *testing.T) {
	mc := &mockClient{}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"agent:ready"}, 30*time.Second, handler, logger)
	require.NotNil(t, poller)
	assert.Equal(t, 30*time.Second, poller.interval)
	assert.Equal(t, []string{"agent:ready"}, poller.labels)
	assert.NotNil(t, poller.handler)
	assert.Nil(t, poller.IdleHandler)
}

func TestPoller_Run_ContextCancellation(t *testing.T) {
	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return nil, nil
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"label"}, 100*time.Millisecond, handler, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err, "Run should return nil on context cancellation")
}

func TestPoller_Run_HandlesIssues(t *testing.T) {
	var handledIssues int32

	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return []*github.Issue{
				{Number: github.Int(1)},
			}, nil
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error {
		atomic.AddInt32(&handledIssues, int32(len(issues)))
		return nil
	}

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&handledIssues), int32(1), "Handler should have been called at least once")
}

func TestPoller_Run_CallsIdleHandler(t *testing.T) {
	var idleCalled int32

	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return nil, nil // No issues found
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)
	poller.IdleHandler = func(ctx context.Context) error {
		atomic.AddInt32(&idleCalled, 1)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&idleCalled), int32(1), "IdleHandler should have been called")
}

func TestPoller_Run_IdleHandlerError(t *testing.T) {
	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return nil, nil
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)
	poller.IdleHandler = func(ctx context.Context) error {
		return fmt.Errorf("idle error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err, "IdleHandler error should be logged but not returned")
}

func TestPoller_Run_HandlerError(t *testing.T) {
	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return []*github.Issue{{Number: github.Int(1)}}, nil
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error {
		return fmt.Errorf("handler error")
	}

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err, "Handler errors are logged, Run returns nil on context cancel")
}

func TestPoller_Run_ListIssuesError(t *testing.T) {
	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return nil, fmt.Errorf("API error")
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err, "ListIssues errors are logged, Run returns nil on context cancel")
}

func TestPoller_Run_NoIdleHandler(t *testing.T) {
	mc := &mockClient{
		listIssuesFunc: func(ctx context.Context, labels []string) ([]*github.Issue, error) {
			return nil, nil
		},
	}
	logger := slog.Default()
	handler := func(ctx context.Context, issues []*github.Issue) error { return nil }

	poller := NewPoller(mc, []string{"label"}, 50*time.Millisecond, handler, logger)
	// Explicitly no IdleHandler set

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx)
	assert.NoError(t, err, "Should handle no IdleHandler gracefully")
}

// --- PR Validation Additional Tests ---

func TestAnalyzeCheckResults_NoChecks(t *testing.T) {
	client := &GitHubClient{}
	result := client.analyzeCheckResults(nil, &github.CombinedStatus{
		Statuses: []*github.RepoStatus{},
	})

	assert.Equal(t, PRCheckStatusSuccess, result.Status)
	assert.True(t, result.AllChecksPassing)
	assert.Empty(t, result.FailedChecks)
	assert.Empty(t, result.PendingChecks)
	assert.Equal(t, 0, result.TotalChecks)
}

func TestAnalyzeCheckResults_NeutralAndSkipped(t *testing.T) {
	client := &GitHubClient{}
	result := client.analyzeCheckResults(
		[]*github.CheckRun{
			{
				Name:       github.Ptr("neutral-check"),
				Status:     github.Ptr("completed"),
				Conclusion: github.Ptr("neutral"),
			},
			{
				Name:       github.Ptr("skipped-check"),
				Status:     github.Ptr("completed"),
				Conclusion: github.Ptr("skipped"),
			},
		},
		&github.CombinedStatus{Statuses: []*github.RepoStatus{}},
	)

	assert.Equal(t, PRCheckStatusSuccess, result.Status)
	assert.True(t, result.AllChecksPassing)
	assert.Equal(t, 2, result.TotalChecks)
}

func TestAnalyzeCheckResults_CancelledAndTimedOut(t *testing.T) {
	client := &GitHubClient{}
	result := client.analyzeCheckResults(
		[]*github.CheckRun{
			{
				Name:       github.Ptr("cancelled-check"),
				Status:     github.Ptr("completed"),
				Conclusion: github.Ptr("cancelled"),
				Output:     &github.CheckRunOutput{},
			},
			{
				Name:       github.Ptr("timed-out-check"),
				Status:     github.Ptr("completed"),
				Conclusion: github.Ptr("timed_out"),
				Output:     &github.CheckRunOutput{},
			},
		},
		&github.CombinedStatus{Statuses: []*github.RepoStatus{}},
	)

	assert.Equal(t, PRCheckStatusFailed, result.Status)
	assert.False(t, result.AllChecksPassing)
	assert.Len(t, result.FailedChecks, 2)
}

func TestAnalyzeCheckResults_ActionRequired(t *testing.T) {
	client := &GitHubClient{}
	result := client.analyzeCheckResults(
		[]*github.CheckRun{
			{
				Name:       github.Ptr("action-check"),
				Status:     github.Ptr("completed"),
				Conclusion: github.Ptr("action_required"),
			},
		},
		&github.CombinedStatus{Statuses: []*github.RepoStatus{}},
	)

	assert.Equal(t, PRCheckStatusRunning, result.Status)
	assert.False(t, result.AllChecksPassing)
	assert.Len(t, result.PendingChecks, 1)
	assert.Contains(t, result.PendingChecks[0], "action required")
}

func TestAnalyzeCheckResults_LegacyStatusError(t *testing.T) {
	client := &GitHubClient{}
	result := client.analyzeCheckResults(
		[]*github.CheckRun{},
		&github.CombinedStatus{
			Statuses: []*github.RepoStatus{
				{
					Context:     github.Ptr("ci/pipeline"),
					State:       github.Ptr("error"),
					Description: github.Ptr("Error occurred"),
					TargetURL:   github.Ptr("https://example.com/logs"),
				},
			},
		},
	)

	assert.Equal(t, PRCheckStatusFailed, result.Status)
	assert.False(t, result.AllChecksPassing)
	assert.Len(t, result.FailedChecks, 1)
	assert.Equal(t, "ci/pipeline", result.FailedChecks[0].Name)
	assert.Equal(t, "error", result.FailedChecks[0].Conclusion)
	assert.Equal(t, "Error occurred", result.FailedChecks[0].Summary)
}

func TestPRValidationResult_GenerateFixPrompt_NoFailures(t *testing.T) {
	result := &PRValidationResult{
		FailedChecks: []CheckFailure{},
	}

	prompt := result.GenerateFixPrompt("context", "plan")
	assert.Empty(t, prompt)
}

func TestPRValidationResult_AnalyzeFailures_NoSummary(t *testing.T) {
	result := &PRValidationResult{
		FailedChecks: []CheckFailure{
			{
				Name:       "check",
				Conclusion: "failure",
				// No Summary
				// No DetailsURL
				// No Annotations
			},
		},
	}

	analysis := result.AnalyzeFailures()
	assert.Contains(t, analysis, "### check (failure)")
	assert.NotContains(t, analysis, "Summary")
	assert.NotContains(t, analysis, "View detailed logs")
}

func TestValidatePR_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 1, "head": {"sha": "abc"}}`))
		case strings.Contains(r.URL.Path, "check-runs"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"check_runs": [{"name": "t", "status": "in_progress"}]}`))
		case strings.Contains(r.URL.Path, "status"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"state": "pending", "statuses": []}`))
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := PRValidationOptions{
		MaxWaitTime:   5 * time.Second,
		PollInterval:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	}

	_, err := client.ValidatePR(ctx, 1, opts)
	assert.Error(t, err)
}

func TestValidatePR_DefaultOptions(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 1, "head": {"sha": "abc"}}`))
		case strings.Contains(r.URL.Path, "check-runs"):
			callCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"check_runs": [{"name": "test", "status": "completed", "conclusion": "failure", "output": {}}]}`))
		case strings.Contains(r.URL.Path, "status"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"state": "failure", "statuses": []}`))
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	// Pass zero opts to trigger default
	opts := PRValidationOptions{
		MaxWaitTime: 0, // Will trigger defaults
	}

	result, err := client.ValidatePR(context.Background(), 1, opts)
	// The default timeout is 30 min which would hang, but a failure result returns immediately
	require.NoError(t, err)
	assert.Equal(t, PRCheckStatusFailed, result.Status)
}

func TestValidatePR_GetPRCheckStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/1":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "error"}`))
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	opts := PRValidationOptions{
		MaxWaitTime:   1 * time.Second,
		PollInterval:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	}

	_, err := client.ValidatePR(context.Background(), 1, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting PR check status")
}

func TestGetPRCheckStatus_EmptyHeadSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number": 1, "head": {"sha": ""}}`))
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	_, err := client.GetPRCheckStatus(context.Background(), 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no head commit SHA")
}

func TestGetPRCheckStatus_CheckRunsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 1, "head": {"sha": "abc123"}}`))
		case strings.Contains(r.URL.Path, "check-runs"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "error"}`))
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	_, err := client.GetPRCheckStatus(context.Background(), 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting check runs")
}

func TestGetPRCheckStatus_CombinedStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 1, "head": {"sha": "abc123"}}`))
		case strings.Contains(r.URL.Path, "check-runs"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"check_runs": []}`))
		case strings.Contains(r.URL.Path, "status"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "error"}`))
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	_, err := client.GetPRCheckStatus(context.Background(), 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting commit status")
}

// --- AssignSelfIfNoAssignees with mock server ---

func TestAssignSelfIfNoAssignees_WithAssignees(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/issues/5" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 5, "assignees": [{"login": "user1"}]}`))
		default:
			t.Fatalf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	err := client.AssignSelfIfNoAssignees(context.Background(), 5)
	assert.NoError(t, err)
}

func TestAssignSelfIfNoAssignees_GetIssueError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "error"}`))
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	err := client.AssignSelfIfNoAssignees(context.Background(), 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting issue #5")
}

func TestAssignSelfIfNoAssignees_NoAssignees_AssignsSelf(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/issues/5" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 5, "assignees": []}`))
		case r.URL.Path == "/user" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"login": "bot-user"}`))
		case r.URL.Path == "/repos/owner/repo/issues/5/assignees" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 5}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	err := client.AssignSelfIfNoAssignees(context.Background(), 5)
	assert.NoError(t, err)
}

func TestAssignSelfIfNoAssignees_GetUserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/issues/5" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 5, "assignees": []}`))
		case r.URL.Path == "/user":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message": "unauthorized"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ghClient := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL + "/")
	ghClient.BaseURL = serverURL

	client := &GitHubClient{client: ghClient, owner: "owner", repo: "repo"}

	err := client.AssignSelfIfNoAssignees(context.Background(), 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting authenticated user")
}
