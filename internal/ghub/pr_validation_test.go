package ghub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRValidationResult_AnalyzeFailures(t *testing.T) {
	tests := []struct {
		name     string
		result   *PRValidationResult
		expected string
	}{
		{
			name: "no failures",
			result: &PRValidationResult{
				FailedChecks: []CheckFailure{},
			},
			expected: "",
		},
		{
			name: "single failure with annotations",
			result: &PRValidationResult{
				FailedChecks: []CheckFailure{
					{
						Name:       "build",
						Conclusion: "failure",
						Summary:    "Build failed due to compilation errors",
						DetailsURL: "https://example.com/build/123",
						Annotations: []CheckAnnotation{
							{
								Filename: "main.go",
								Line:     10,
								Message:  "undefined: fmt",
							},
						},
					},
				},
			},
			expected: "## Check Failures Analysis\n\n### build (failure)\n\n**Summary:** Build failed due to compilation errors\n\n**Issues found:**\n\n- `main.go:10`: undefined: fmt\n\n[View detailed logs](https://example.com/build/123)\n",
		},
		{
			name: "multiple failures",
			result: &PRValidationResult{
				FailedChecks: []CheckFailure{
					{
						Name:       "test",
						Conclusion: "failure",
						Summary:    "Tests failed",
					},
					{
						Name:       "lint",
						Conclusion: "failure",
						Summary:    "Linting issues found",
					},
				},
			},
			expected: "## Check Failures Analysis\n\n### test (failure)\n\n**Summary:** Tests failed\n\n\n---\n\n### lint (failure)\n\n**Summary:** Linting issues found\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result.AnalyzeFailures()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPRValidationResult_GenerateFixPrompt(t *testing.T) {
	result := &PRValidationResult{
		FailedChecks: []CheckFailure{
			{
				Name:       "build",
				Conclusion: "failure",
				Summary:    "Build failed",
				Annotations: []CheckAnnotation{
					{
						Filename: "main.go",
						Line:     5,
						Message:  "syntax error",
					},
				},
			},
		},
	}

	issueContext := "Fix the build issue"
	originalPlan := "Add missing imports"

	prompt := result.GenerateFixPrompt(issueContext, originalPlan)

	assert.Contains(t, prompt, "## Pull Request Check Failures")
	assert.Contains(t, prompt, "build (failure)")
	assert.Contains(t, prompt, "Fix the build issue")
	assert.Contains(t, prompt, "Add missing imports")
	assert.Contains(t, prompt, "go build ./... && go test ./...")
}

func TestGitHubClient_analyzeCheckResults(t *testing.T) {
	client := &GitHubClient{}

	tests := []struct {
		name             string
		checkRuns        []*github.CheckRun
		commitStatus     *github.CombinedStatus
		expectedStatus   PRCheckStatus
		expectedPassing  bool
		expectedFailures int
		expectedPending  int
	}{
		{
			name: "all checks passing",
			checkRuns: []*github.CheckRun{
				{
					Name:       github.Ptr("test"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("success"),
				},
				{
					Name:       github.Ptr("build"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("success"),
				},
			},
			commitStatus: &github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						Context: github.Ptr("ci/status"),
						State:   github.Ptr("success"),
					},
				},
			},
			expectedStatus:   PRCheckStatusSuccess,
			expectedPassing:  true,
			expectedFailures: 0,
			expectedPending:  0,
		},
		{
			name: "some checks pending",
			checkRuns: []*github.CheckRun{
				{
					Name:   github.Ptr("test"),
					Status: github.Ptr("in_progress"),
				},
				{
					Name:       github.Ptr("build"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("success"),
				},
			},
			commitStatus: &github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						Context: github.Ptr("ci/status"),
						State:   github.Ptr("pending"),
					},
				},
			},
			expectedStatus:   PRCheckStatusRunning,
			expectedPassing:  false,
			expectedFailures: 0,
			expectedPending:  2,
		},
		{
			name: "some checks failed",
			checkRuns: []*github.CheckRun{
				{
					Name:       github.Ptr("test"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("failure"),
					Output: &github.CheckRunOutput{
						Summary: github.Ptr("Test failures"),
						Annotations: []*github.CheckRunAnnotation{
							{
								Path:            github.Ptr("test.go"),
								StartLine:       github.Ptr(10),
								StartColumn:     github.Ptr(5),
								Message:         github.Ptr("assertion failed"),
								AnnotationLevel: github.Ptr("failure"),
							},
						},
					},
				},
				{
					Name:       github.Ptr("build"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("success"),
				},
			},
			commitStatus: &github.CombinedStatus{
				Statuses: []*github.RepoStatus{
					{
						Context:     github.Ptr("ci/status"),
						State:       github.Ptr("failure"),
						Description: github.Ptr("Build failed"),
						TargetURL:   github.Ptr("https://example.com/build"),
					},
				},
			},
			expectedStatus:   PRCheckStatusFailed,
			expectedPassing:  false,
			expectedFailures: 2,
			expectedPending:  0,
		},
		{
			name: "mixed states",
			checkRuns: []*github.CheckRun{
				{
					Name:       github.Ptr("test"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("success"),
				},
				{
					Name:   github.Ptr("build"),
					Status: github.Ptr("queued"),
				},
				{
					Name:       github.Ptr("lint"),
					Status:     github.Ptr("completed"),
					Conclusion: github.Ptr("failure"),
				},
			},
			commitStatus: &github.CombinedStatus{
				Statuses: []*github.RepoStatus{},
			},
			expectedStatus:   PRCheckStatusFailed,
			expectedPassing:  false,
			expectedFailures: 1,
			expectedPending:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.analyzeCheckResults(tt.checkRuns, tt.commitStatus)

			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedPassing, result.AllChecksPassing)
			assert.Len(t, result.FailedChecks, tt.expectedFailures)
			assert.Len(t, result.PendingChecks, tt.expectedPending)

			totalExpected := len(tt.checkRuns) + len(tt.commitStatus.Statuses)
			assert.Equal(t, totalExpected, result.TotalChecks)

			// Verify failure details
			if tt.expectedFailures > 0 {
				for _, failure := range result.FailedChecks {
					assert.NotEmpty(t, failure.Name)
					assert.NotEmpty(t, failure.Conclusion)
				}
			}
		})
	}
}

func TestDefaultPRValidationOptions(t *testing.T) {
	opts := DefaultPRValidationOptions()

	assert.Equal(t, 30*time.Minute, opts.MaxWaitTime)
	assert.Equal(t, 30*time.Second, opts.PollInterval)
	assert.Equal(t, 3, opts.MaxRetries)
	assert.Equal(t, 1.5, opts.BackoffFactor)
	assert.Equal(t, 5*time.Minute, opts.MaxPollTime)
}

func TestGitHubClient_GetPRCheckStatus_Integration(t *testing.T) {
	// Mock server to simulate GitHub API responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/123":
			// Mock PR response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"number": 123,
				"head": {
					"sha": "abc123"
				}
			}`))
		case r.URL.Path == "/repos/owner/repo/commits/abc123/check-runs":
			// Mock check runs response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"check_runs": [
					{
						"name": "test",
						"status": "completed",
						"conclusion": "success"
					},
					{
						"name": "build",
						"status": "completed",
						"conclusion": "failure",
						"output": {
							"summary": "Build failed",
							"annotations": [
								{
									"path": "main.go",
									"start_line": 10,
									"start_column": 5,
									"message": "syntax error",
									"annotation_level": "failure"
								}
							]
						},
						"details_url": "https://example.com/build/123"
					}
				]
			}`))
		case r.URL.Path == "/repos/owner/repo/commits/abc123/status":
			// Mock commit status response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"state": "failure",
				"statuses": [
					{
						"context": "ci/pipeline",
						"state": "failure",
						"description": "Pipeline failed",
						"target_url": "https://example.com/pipeline/123"
					}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL)
	if !strings.HasSuffix(serverURL.Path, "/") {
		serverURL.Path += "/"
	}
	client.BaseURL = serverURL

	ghClient := &GitHubClient{
		client: client,
		owner:  "owner",
		repo:   "repo",
	}

	ctx := context.Background()
	result, err := ghClient.GetPRCheckStatus(ctx, 123)

	require.NoError(t, err)
	assert.Equal(t, PRCheckStatusFailed, result.Status)
	assert.False(t, result.AllChecksPassing)
	assert.Len(t, result.FailedChecks, 2)  // One from check runs, one from status
	assert.Equal(t, 3, result.TotalChecks) // 2 check runs + 1 status
}

func TestGitHubClient_ValidatePR_Timeout(t *testing.T) {
	// Create a client that will always return pending status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/123":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 123, "head": {"sha": "abc123"}}`))
		case r.URL.Path == "/repos/owner/repo/commits/abc123/check-runs":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"check_runs": [
					{
						"name": "test",
						"status": "in_progress"
					}
				]
			}`))
		case r.URL.Path == "/repos/owner/repo/commits/abc123/status":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"state": "pending", "statuses": []}`))
		}
	}))
	defer server.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL)
	if !strings.HasSuffix(serverURL.Path, "/") {
		serverURL.Path += "/"
	}
	client.BaseURL = serverURL

	ghClient := &GitHubClient{
		client: client,
		owner:  "owner",
		repo:   "repo",
	}

	opts := PRValidationOptions{
		MaxWaitTime:   100 * time.Millisecond, // Very short timeout for testing
		PollInterval:  10 * time.Millisecond,
		BackoffFactor: 1.0, // No backoff for predictable timing
	}

	ctx := context.Background()
	_, err := ghClient.ValidatePR(ctx, 123, opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for PR checks")
}

func TestGitHubClient_ValidatePR_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/123":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number": 123, "head": {"sha": "abc123"}}`))
		case r.URL.Path == "/repos/owner/repo/commits/abc123/check-runs":
			callCount++
			w.WriteHeader(http.StatusOK)
			if callCount == 1 {
				// First call - checks still running
				w.Write([]byte(`{
					"check_runs": [
						{
							"name": "test",
							"status": "in_progress"
						}
					]
				}`))
			} else {
				// Second call - checks completed successfully
				w.Write([]byte(`{
					"check_runs": [
						{
							"name": "test",
							"status": "completed",
							"conclusion": "success"
						}
					]
				}`))
			}
		case r.URL.Path == "/repos/owner/repo/commits/abc123/status":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"state": "success", "statuses": []}`))
		}
	}))
	defer server.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(server.URL)
	if !strings.HasSuffix(serverURL.Path, "/") {
		serverURL.Path += "/"
	}
	client.BaseURL = serverURL

	ghClient := &GitHubClient{
		client: client,
		owner:  "owner",
		repo:   "repo",
	}

	opts := PRValidationOptions{
		MaxWaitTime:   5 * time.Second,
		PollInterval:  10 * time.Millisecond,
		BackoffFactor: 1.0,
	}

	ctx := context.Background()
	result, err := ghClient.ValidatePR(ctx, 123, opts)

	require.NoError(t, err)
	assert.Equal(t, PRCheckStatusSuccess, result.Status)
	assert.True(t, result.AllChecksPassing)
	assert.Empty(t, result.FailedChecks)
	assert.Empty(t, result.PendingChecks)
}
