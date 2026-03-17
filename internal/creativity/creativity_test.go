package creativity

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGitHub implements GitHubClient for testing.
type mockGitHub struct {
	issuesByLabel map[string][]*Issue
	createdIssues []createdIssue
	createErr     error
}

type createdIssue struct {
	title  string
	body   string
	labels []string
}

func newMockGitHub() *mockGitHub {
	return &mockGitHub{
		issuesByLabel: make(map[string][]*Issue),
	}
}

func (m *mockGitHub) ListIssuesByLabel(_ context.Context, label string) ([]*Issue, error) {
	return m.issuesByLabel[label], nil
}

func (m *mockGitHub) ListClosedIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return nil, nil
}

func (m *mockGitHub) ListAllClosedIssues(_ context.Context) ([]*Issue, error) {
	return nil, nil
}

func (m *mockGitHub) CreateIssue(_ context.Context, title, body string, labels []string) (int, error) {
	if m.createErr != nil {
		return 0, m.createErr
	}
	m.createdIssues = append(m.createdIssues, createdIssue{title: title, body: body, labels: labels})
	return len(m.createdIssues), nil
}

func (m *mockGitHub) AddLabels(_ context.Context, _ int, _ []string) error { return nil }
func (m *mockGitHub) RemoveLabel(_ context.Context, _ int, _ string) error { return nil }

// mockAI implements AIClient for testing.
type mockAI struct {
	suggestion *Suggestion
	err        error
}

func (m *mockAI) GenerateSuggestion(_ context.Context, _ string) (*Suggestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.suggestion, nil
}

func testConfig() *config.CreativityConfig {
	return &config.CreativityConfig{
		Enabled:                   true,
		IdleThresholdSeconds:      1,
		SuggestionCooldownSeconds: 1,
		MaxPendingSuggestions:     1,
		MaxRejectionHistory:       50,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestCreativityEngine_ExitsWhenWorkAvailable(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelReady] = []*Issue{
		{Number: 1, Title: "Real work", State: "open"},
	}

	ai := &mockAI{suggestion: &Suggestion{Title: "test", Body: "test"}}
	engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues, "should not create issues when work is available")
}

func TestCreativityEngine_SkipsWhenPendingSuggestionExists(t *testing.T) {
	gh := newMockGitHub()

	// Flow:
	// 1. checkWork → nil (no work)
	// 2. hasPending → [{pending}] (at max) → sleep cooldown
	// 3. checkWork → [{work}] → exit
	counterGH := &counterMockGitHub{
		inner:        gh,
		readyResults: [][]*Issue{nil, {{Number: 99, Title: "Work appeared"}}},
		suggResults:  [][]*Issue{{{Number: 10, Title: "Pending suggestion"}}},
	}

	ai := &mockAI{suggestion: &Suggestion{Title: "test", Body: "test"}}
	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues, "should not create issues when pending suggestion exists")
}

func TestCreativityEngine_CreatesSuggestionWhenIdle(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelSuggestionRejected] = nil

	// Flow:
	// 1. checkWork → readyResults[0] = nil
	// 2. hasPending → suggResults[0] = nil
	// 3. gatherContext → readyResults[1] = nil, suggResults[1] = nil
	// 4. generateSuggestion → suggestion
	// 5. createSuggestionIssue → success
	// 6. sleep cooldown
	// 7. checkWork → readyResults[2] = [{work}] → exit
	counterGH := &counterMockGitHub{
		inner:        gh,
		readyResults: [][]*Issue{nil, nil, {{Number: 99, Title: "Work appeared"}}},
		suggResults:  [][]*Issue{nil, nil},
	}

	ai := &mockAI{
		suggestion: &Suggestion{
			Title: "Add unit test coverage for config package",
			Body:  "The config package lacks test coverage for edge cases.",
		},
	}

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)
	err := engine.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, gh.createdIssues, 1)
	assert.Equal(t, "Add unit test coverage for config package", gh.createdIssues[0].title)
	assert.Contains(t, gh.createdIssues[0].labels, labelSuggestion)
}

func TestCreativityEngine_SkipsDuplicateSuggestion(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelSuggestionRejected] = nil

	// Flow:
	// 1. checkWork → readyResults[0] = nil
	// 2. hasPending → suggResults[0] = nil (below max)
	// 3. gatherContext → readyResults[1] = nil, suggResults[1] = [{pending "Add logging"}]
	// 4. generateSuggestion → "Add logging"
	// 5. isDuplicate → true (matches pending in projectCtx)
	// 6. Loop back
	// 7. checkWork → readyResults[2] = [{work}] → exit
	counterGH := &counterMockGitHub{
		inner: gh,
		readyResults: [][]*Issue{
			nil,
			nil,
			{{Number: 99, Title: "Work appeared"}},
		},
		suggResults: [][]*Issue{
			nil,
			{{Number: 5, Title: "Add logging"}},
		},
	}

	ai := &mockAI{
		suggestion: &Suggestion{
			Title: "Add logging",
			Body:  "We should add structured logging.",
		},
	}

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)
	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues, "should skip duplicate suggestion")
}

func TestCreativityEngine_SkipsRejectedSuggestion(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelSuggestionRejected] = []*Issue{
		{Number: 3, Title: "Add caching layer"},
	}

	// Flow:
	// 1. loadRejectionHistory → loads "Add caching layer" into cache
	// 2. checkWork → readyResults[0] = nil
	// 3. hasPending → suggResults[0] = nil
	// 4. gatherContext → readyResults[1] = nil, suggResults[1] = nil
	// 5. generateSuggestion → "Add caching layer"
	// 6. isDuplicate → true (matches rejection cache)
	// 7. Loop back
	// 8. checkWork → readyResults[2] = [{work}] → exit
	counterGH := &counterMockGitHub{
		inner: gh,
		readyResults: [][]*Issue{
			nil,
			nil,
			{{Number: 99, Title: "Work appeared"}},
		},
		suggResults: [][]*Issue{nil, nil},
	}

	ai := &mockAI{
		suggestion: &Suggestion{
			Title: "Add caching layer",
			Body:  "We should add a caching layer.",
		},
	}

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)
	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues, "should skip rejected suggestion")
}

func TestCreativityEngine_DisabledByConfig(t *testing.T) {
	cfg := testConfig()
	cfg.Enabled = false

	gh := newMockGitHub()
	ai := &mockAI{}

	engine := NewCreativityEngine(gh, ai, cfg, RepoConfig{}, "test-agent", testLogger(), nil)
	assert.NotNil(t, engine)
	// When disabled, the engine is simply never called by the poller.
	// We verify the engine can be constructed even with disabled config.
}

func TestCreativityEngine_ContextIncludesClosedIssues(t *testing.T) {
	closedIssues := []*Issue{
		{Number: 50, Title: "Implemented feature X", State: "closed"},
		{Number: 51, Title: "Fixed bug Y", State: "closed"},
	}

	gh := &closedIssueMockGitHub{
		mockGitHub:   newMockGitHub(),
		closedIssues: closedIssues,
	}
	gh.issuesByLabel[labelSuggestionRejected] = nil

	counterGH := &counterMockGitHubWithClosed{
		inner:        gh,
		readyResults: [][]*Issue{nil, nil, {{Number: 99, Title: "Work appeared"}}},
		suggResults:  [][]*Issue{nil, nil},
	}

	capturedPrompt := ""
	ai := &capturingMockAI{
		suggestion: &Suggestion{
			Title: "New suggestion",
			Body:  "Something new.",
		},
		capturePrompt: &capturedPrompt,
	}

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)
	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, capturedPrompt, "Implemented feature X")
	assert.Contains(t, capturedPrompt, "Fixed bug Y")
	assert.Contains(t, capturedPrompt, "Closed Issues")
}

func TestCreativityEngine_RepoCloneFailureGraceful(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelSuggestionRejected] = nil

	counterGH := &counterMockGitHub{
		inner:        gh,
		readyResults: [][]*Issue{nil, nil, {{Number: 99, Title: "Work appeared"}}},
		suggResults:  [][]*Issue{nil, nil},
	}

	ai := &mockAI{
		suggestion: &Suggestion{
			Title: "Improvement without repo context",
			Body:  "This suggestion was made without codebase awareness.",
		},
	}

	// Use an invalid repo URL to trigger clone failure.
	repoCfg := RepoConfig{
		URL:          "https://invalid.example.com/nonexistent.git",
		Token:        "fake-token",
		WorkspaceDir: t.TempDir(),
	}
	engine := NewCreativityEngine(counterGH, ai, testConfig(), repoCfg, "test-agent", testLogger(), nil)

	err := engine.Run(context.Background())
	require.NoError(t, err)
	// Engine should still create the suggestion despite repo clone failure.
	require.Len(t, gh.createdIssues, 1)
	assert.Equal(t, "Improvement without repo context", gh.createdIssues[0].title)
}

func TestCreativityEngine_AIError(t *testing.T) {
	gh := newMockGitHub()
	gh.issuesByLabel[labelSuggestionRejected] = nil

	// Flow:
	// 1. checkWork → readyResults[0] = nil
	// 2. hasPending → suggResults[0] = nil
	// 3. gatherContext → readyResults[1] = nil, suggResults[1] = nil
	// 4. generateSuggestion → error → sleep cooldown
	// 5. checkWork → readyResults[2] = [{work}] → exit
	counterGH := &counterMockGitHub{
		inner: gh,
		readyResults: [][]*Issue{
			nil,
			nil,
			{{Number: 99, Title: "Work appeared"}},
		},
		suggResults: [][]*Issue{nil, nil},
	}

	ai := &mockAI{err: fmt.Errorf("API error")}
	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues)
}

// counterMockGitHub wraps mockGitHub and returns different results per call.
type counterMockGitHub struct {
	inner        *mockGitHub
	readyCallNum int
	suggCallNum  int
	readyResults [][]*Issue
	suggResults  [][]*Issue
}

func (c *counterMockGitHub) ListClosedIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return nil, nil
}

func (c *counterMockGitHub) ListAllClosedIssues(_ context.Context) ([]*Issue, error) {
	return nil, nil
}

func (c *counterMockGitHub) ListIssuesByLabel(_ context.Context, label string) ([]*Issue, error) {
	switch label {
	case labelReady:
		idx := c.readyCallNum
		c.readyCallNum++
		if idx < len(c.readyResults) {
			return c.readyResults[idx], nil
		}
		return nil, nil
	case labelSuggestion:
		idx := c.suggCallNum
		c.suggCallNum++
		if idx < len(c.suggResults) {
			return c.suggResults[idx], nil
		}
		return nil, nil
	case labelSuggestionRejected:
		return c.inner.issuesByLabel[labelSuggestionRejected], nil
	default:
		return nil, nil
	}
}

func (c *counterMockGitHub) CreateIssue(ctx context.Context, title, body string, labels []string) (int, error) {
	return c.inner.CreateIssue(ctx, title, body, labels)
}

func (c *counterMockGitHub) AddLabels(ctx context.Context, number int, labels []string) error {
	return c.inner.AddLabels(ctx, number, labels)
}

func (c *counterMockGitHub) RemoveLabel(ctx context.Context, number int, label string) error {
	return c.inner.RemoveLabel(ctx, number, label)
}

// closedIssueMockGitHub extends mockGitHub with closed issues support.
type closedIssueMockGitHub struct {
	*mockGitHub
	closedIssues []*Issue
}

func (m *closedIssueMockGitHub) ListClosedIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return m.closedIssues, nil
}

func (m *closedIssueMockGitHub) ListAllClosedIssues(_ context.Context) ([]*Issue, error) {
	return m.closedIssues, nil
}

// counterMockGitHubWithClosed wraps closedIssueMockGitHub with counter semantics.
type counterMockGitHubWithClosed struct {
	inner        *closedIssueMockGitHub
	readyCallNum int
	suggCallNum  int
	readyResults [][]*Issue
	suggResults  [][]*Issue
}

func (c *counterMockGitHubWithClosed) ListIssuesByLabel(_ context.Context, label string) ([]*Issue, error) {
	switch label {
	case labelReady:
		idx := c.readyCallNum
		c.readyCallNum++
		if idx < len(c.readyResults) {
			return c.readyResults[idx], nil
		}
		return nil, nil
	case labelSuggestion:
		idx := c.suggCallNum
		c.suggCallNum++
		if idx < len(c.suggResults) {
			return c.suggResults[idx], nil
		}
		return nil, nil
	case labelSuggestionRejected:
		return c.inner.issuesByLabel[labelSuggestionRejected], nil
	default:
		return nil, nil
	}
}

func (c *counterMockGitHubWithClosed) ListClosedIssuesByLabel(ctx context.Context, label string) ([]*Issue, error) {
	return c.inner.ListClosedIssuesByLabel(ctx, label)
}

func (c *counterMockGitHubWithClosed) ListAllClosedIssues(ctx context.Context) ([]*Issue, error) {
	return c.inner.ListAllClosedIssues(ctx)
}

func (c *counterMockGitHubWithClosed) CreateIssue(ctx context.Context, title, body string, labels []string) (int, error) {
	return c.inner.CreateIssue(ctx, title, body, labels)
}

func (c *counterMockGitHubWithClosed) AddLabels(ctx context.Context, number int, labels []string) error {
	return c.inner.AddLabels(ctx, number, labels)
}

func (c *counterMockGitHubWithClosed) RemoveLabel(ctx context.Context, number int, label string) error {
	return c.inner.RemoveLabel(ctx, number, label)
}

// capturingMockAI captures the prompt passed to GenerateSuggestion.
type capturingMockAI struct {
	suggestion    *Suggestion
	capturePrompt *string
}

func (m *capturingMockAI) GenerateSuggestion(_ context.Context, prompt string) (*Suggestion, error) {
	*m.capturePrompt = prompt
	return m.suggestion, nil
}

// --- Additional tests for uncovered functions ---

func TestCreativityEngine_CheckForAvailableWork(t *testing.T) {
	t.Run("no work available", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		hasWork, err := engine.checkForAvailableWork(context.Background())
		require.NoError(t, err)
		assert.False(t, hasWork)
	})

	t.Run("work available", func(t *testing.T) {
		gh := newMockGitHub()
		gh.issuesByLabel[labelReady] = []*Issue{{Number: 1, Title: "Work"}}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		hasWork, err := engine.checkForAvailableWork(context.Background())
		require.NoError(t, err)
		assert.True(t, hasWork)
	})

	t.Run("error checking work", func(t *testing.T) {
		gh := &errorMockGitHub{err: fmt.Errorf("network error")}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		_, err := engine.checkForAvailableWork(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checking for available work")
	})
}

func TestCreativityEngine_HasPendingSuggestion(t *testing.T) {
	t.Run("no pending suggestions", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		pending, err := engine.hasPendingSuggestion(context.Background())
		require.NoError(t, err)
		assert.False(t, pending)
	})

	t.Run("at max pending suggestions", func(t *testing.T) {
		gh := newMockGitHub()
		gh.issuesByLabel[labelSuggestion] = []*Issue{{Number: 1, Title: "Suggestion"}}
		ai := &mockAI{}
		cfg := testConfig()
		cfg.MaxPendingSuggestions = 1
		engine := NewCreativityEngine(gh, ai, cfg, RepoConfig{}, "test-agent", testLogger(), nil)

		pending, err := engine.hasPendingSuggestion(context.Background())
		require.NoError(t, err)
		assert.True(t, pending)
	})

	t.Run("error checking pending", func(t *testing.T) {
		gh := &errorMockGitHub{err: fmt.Errorf("network error")}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		_, err := engine.hasPendingSuggestion(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "checking pending suggestions")
	})
}

func TestCreativityEngine_IsDuplicate(t *testing.T) {
	t.Run("matches pending idea", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		projectCtx := &ProjectContext{
			PendingIdeas: []*Issue{{Number: 1, Title: "Add logging"}},
		}

		assert.True(t, engine.isDuplicate(&Suggestion{Title: "add logging", Body: "details"}, projectCtx))
	})

	t.Run("matches open issue", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		projectCtx := &ProjectContext{
			OpenIssues: []*Issue{{Number: 1, Title: "Improve error handling"}},
		}

		assert.True(t, engine.isDuplicate(&Suggestion{Title: "improve error handling", Body: "details"}, projectCtx))
	})

	t.Run("matches rejection cache", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)
		engine.rejectionCache.Add("Add caching layer")

		projectCtx := &ProjectContext{}
		assert.True(t, engine.isDuplicate(&Suggestion{Title: "Add caching layer", Body: "details"}, projectCtx))
	})

	t.Run("no match", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		projectCtx := &ProjectContext{
			PendingIdeas: []*Issue{{Number: 1, Title: "Add logging"}},
			OpenIssues:   []*Issue{{Number: 2, Title: "Fix bug"}},
		}

		assert.False(t, engine.isDuplicate(&Suggestion{Title: "Add monitoring", Body: "details"}, projectCtx))
	})
}

func TestCreativityEngine_CreateSuggestionIssue(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		err := engine.createSuggestionIssue(context.Background(), &Suggestion{
			Title: "Test suggestion",
			Body:  "Test body",
		})
		require.NoError(t, err)

		require.Len(t, gh.createdIssues, 1)
		assert.Equal(t, "Test suggestion", gh.createdIssues[0].title)
		assert.Contains(t, gh.createdIssues[0].body, "Test body")
		assert.Contains(t, gh.createdIssues[0].body, "test-agent")
		assert.Contains(t, gh.createdIssues[0].labels, labelSuggestion)
	})

	t.Run("error creating issue", func(t *testing.T) {
		gh := newMockGitHub()
		gh.createErr = fmt.Errorf("API error")
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		err := engine.createSuggestionIssue(context.Background(), &Suggestion{
			Title: "Test",
			Body:  "Body",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "creating suggestion issue")
	})
}

func TestCreativityEngine_GenerateSuggestion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{suggestion: &Suggestion{Title: "Test", Body: "Body"}}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		suggestion, err := engine.generateSuggestion(context.Background(), &ProjectContext{})
		require.NoError(t, err)
		assert.Equal(t, "Test", suggestion.Title)
	})

	t.Run("AI error", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{err: fmt.Errorf("AI error")}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		_, err := engine.generateSuggestion(context.Background(), &ProjectContext{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "generating suggestion")
	})
}

func TestCreativityEngine_Sleep(t *testing.T) {
	t.Run("sleeps for duration", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		err := engine.sleep(context.Background(), 10*time.Millisecond)
		assert.NoError(t, err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		gh := newMockGitHub()
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := engine.sleep(ctx, 1*time.Hour)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestCreativityEngine_LoadRejectionHistory(t *testing.T) {
	t.Run("loads rejected issues", func(t *testing.T) {
		gh := newMockGitHub()
		gh.issuesByLabel[labelSuggestionRejected] = []*Issue{
			{Number: 1, Title: "Rejected idea 1"},
			{Number: 2, Title: "Rejected idea 2"},
		}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		err := engine.loadRejectionHistory(context.Background())
		require.NoError(t, err)

		assert.True(t, engine.rejectionCache.Contains("Rejected idea 1"))
		assert.True(t, engine.rejectionCache.Contains("Rejected idea 2"))
	})

	t.Run("error loading rejection history", func(t *testing.T) {
		gh := &errorMockGitHub{err: fmt.Errorf("API error")}
		ai := &mockAI{}
		engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

		err := engine.loadRejectionHistory(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "loading rejection history")
	})
}

func TestCreativityEngine_RunContextCancelled(t *testing.T) {
	gh := newMockGitHub()
	ai := &mockAI{}
	engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := engine.Run(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// errorMockGitHub returns an error for all operations.
type errorMockGitHub struct {
	err error
}

func (m *errorMockGitHub) ListIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return nil, m.err
}
func (m *errorMockGitHub) ListClosedIssuesByLabel(_ context.Context, _ string) ([]*Issue, error) {
	return nil, m.err
}
func (m *errorMockGitHub) ListAllClosedIssues(_ context.Context) ([]*Issue, error) {
	return nil, m.err
}
func (m *errorMockGitHub) CreateIssue(_ context.Context, _, _ string, _ []string) (int, error) {
	return 0, m.err
}
func (m *errorMockGitHub) AddLabels(_ context.Context, _ int, _ []string) error { return m.err }
func (m *errorMockGitHub) RemoveLabel(_ context.Context, _ int, _ string) error { return m.err }
