package creativity

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
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

func testConfig() config.CreativityConfig {
	return config.CreativityConfig{
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
	engine := NewCreativityEngine(gh, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

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
	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

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

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())
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

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())
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

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())
	err := engine.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, gh.createdIssues, "should skip rejected suggestion")
}

func TestCreativityEngine_DisabledByConfig(t *testing.T) {
	cfg := testConfig()
	cfg.Enabled = false

	gh := newMockGitHub()
	ai := &mockAI{}

	engine := NewCreativityEngine(gh, ai, cfg, RepoConfig{}, "test-agent", testLogger())
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

	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())
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
	engine := NewCreativityEngine(counterGH, ai, testConfig(), repoCfg, "test-agent", testLogger())

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
	engine := NewCreativityEngine(counterGH, ai, testConfig(), RepoConfig{}, "test-agent", testLogger())

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
