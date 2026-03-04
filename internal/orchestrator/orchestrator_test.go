package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStructuredLogger() *observability.StructuredLogger {
	return observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
}

func newTestMetrics() *observability.Metrics {
	return observability.NewMetrics(newTestStructuredLogger())
}

// mockAgent implements agent.Agent for testing.
type mockAgent struct {
	mu        sync.Mutex
	agentType agent.AgentType
	runFn     func(ctx context.Context) error
	status    agent.StatusReport
}

func newMockAgent(t agent.AgentType) *mockAgent {
	return &mockAgent{
		agentType: t,
		status: agent.StatusReport{
			Type:  t,
			State: "idle",
		},
	}
}

func (m *mockAgent) Type() agent.AgentType { return m.agentType }

func (m *mockAgent) Run(ctx context.Context) error {
	if m.runFn != nil {
		return m.runFn(ctx)
	}
	// Default: wait for context cancellation
	<-ctx.Done()
	return nil
}

func (m *mockAgent) Status() agent.StatusReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockAgent) setStatus(s agent.StatusReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = s
}

func TestNew(t *testing.T) {
	agents := []agent.Agent{newMockAgent(agent.TypeDeveloper)}
	logger := slog.Default()

	o := New(agents, logger)
	require.NotNil(t, o)
	assert.Len(t, o.agents, 1)
	assert.Same(t, logger, o.logger)
}

func TestNew_NoAgents(t *testing.T) {
	o := New(nil, slog.Default())
	require.NotNil(t, o)
	assert.Empty(t, o.agents)
}

func TestNew_MultipleAgents(t *testing.T) {
	agents := []agent.Agent{
		newMockAgent(agent.TypeDeveloper),
		newMockAgent(agent.TypeQA),
	}
	o := New(agents, slog.Default())
	assert.Len(t, o.agents, 2)
}

func TestWithObservability(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithObservability(nil, nil)
	assert.Same(t, o, result, "should return same orchestrator for chaining")
}

func TestWithWorkspaceCleanup(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithWorkspaceCleanup(nil)
	assert.Same(t, o, result)
}

func TestWithLogRotation(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithLogRotation(nil)
	assert.Same(t, o, result)
}

func TestWithLogCleanup(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithLogCleanup(nil)
	assert.Same(t, o, result)
}

func TestWithLogFilePath(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithLogFilePath("/var/log/agent.log")
	assert.Same(t, o, result)
	assert.Equal(t, "/var/log/agent.log", o.logFilePath)
}

func TestWithConfigManager(t *testing.T) {
	o := New(nil, slog.Default())
	result := o.WithConfigManager(nil)
	assert.Same(t, o, result)
}

func TestBuilderChaining(t *testing.T) {
	o := New(nil, slog.Default()).
		WithObservability(nil, nil).
		WithWorkspaceCleanup(nil).
		WithLogRotation(nil).
		WithLogCleanup(nil).
		WithLogFilePath("/tmp/test.log").
		WithConfigManager(nil)
	require.NotNil(t, o)
	assert.Equal(t, "/tmp/test.log", o.logFilePath)
}

func TestRun_SingleAgent_ContextCancellation(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	o := New([]agent.Agent{ma}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "context cancellation should not produce an error")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete within timeout")
	}
}

func TestRun_MultipleAgents_AllStop(t *testing.T) {
	agents := []agent.Agent{
		newMockAgent(agent.TypeDeveloper),
		newMockAgent(agent.TypeQA),
	}
	o := New(agents, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete within timeout")
	}
}

func TestRun_AgentReturnsError(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	expectedErr := errors.New("agent crashed")
	ma.runFn = func(ctx context.Context) error {
		return expectedErr
	}

	o := New([]agent.Agent{ma}, slog.Default())
	err := o.Run(context.Background())
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestRun_AgentReturnsNilImmediately(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return nil
	}

	o := New([]agent.Agent{ma}, slog.Default())
	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_NoAgents(t *testing.T) {
	o := New(nil, slog.Default())
	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestStatus_SingleAgent(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   "idle",
		Message: "waiting for issues",
	})

	o := New([]agent.Agent{ma}, slog.Default())
	reports := o.Status()
	require.Len(t, reports, 1)
	assert.Equal(t, agent.TypeDeveloper, reports[0].Type)
	assert.Equal(t, "idle", reports[0].State)
	assert.Equal(t, "waiting for issues", reports[0].Message)
}

func TestStatus_MultipleAgents(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{Type: agent.TypeDeveloper, State: "implement"})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{Type: agent.TypeQA, State: "idle"})

	o := New([]agent.Agent{ma1, ma2}, slog.Default())
	reports := o.Status()
	require.Len(t, reports, 2)
	assert.Equal(t, "implement", reports[0].State)
	assert.Equal(t, "idle", reports[1].State)
}

func TestStatus_NoAgents(t *testing.T) {
	o := New(nil, slog.Default())
	reports := o.Status()
	assert.Empty(t, reports)
}

func TestStatus_WithIssueID(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   "implement",
		IssueID: 42,
	})

	o := New([]agent.Agent{ma}, slog.Default())
	reports := o.Status()
	require.Len(t, reports, 1)
	assert.Equal(t, 42, reports[0].IssueID)
}

func TestRun_WithObservability_NilLoggerAndMetrics(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return nil
	}

	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(nil, nil)

	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_WithRealObservability_Success(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return nil
	}

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m)

	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_WithRealObservability_AgentError(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return errors.New("agent failed")
	}

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m)

	err := o.Run(context.Background())
	assert.Error(t, err)
}

func TestRun_WithRealObservability_ContextCancel(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	// Default: blocks until ctx done

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRun_AgentError_WithContextActive(t *testing.T) {
	// Test agent returning error while context is still active
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return errors.New("runtime error")
	}

	o := New([]agent.Agent{ma}, slog.Default())
	err := o.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "runtime error")
}

func TestRun_MultipleAgents_OneErrors(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.runFn = func(ctx context.Context) error {
		return errors.New("agent 1 failed")
	}
	ma2 := newMockAgent(agent.TypeQA)
	// agent 2 blocks until context cancellation (which errgroup does when one fails)
	ma2.runFn = func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}

	o := New([]agent.Agent{ma1, ma2}, slog.Default())
	err := o.Run(context.Background())
	assert.Error(t, err)
}

func TestRun_WithLogFilePath(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error { return nil }

	o := New([]agent.Agent{ma}, slog.Default()).
		WithLogFilePath("/tmp/test.log")

	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_ContextAlreadyCancelled(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	o := New([]agent.Agent{ma}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := o.Run(ctx)
	assert.NoError(t, err)
}

// mockWorkspaceManager implements workspace.Manager for testing.
type mockWorkspaceManager struct{}

func (m *mockWorkspaceManager) CreateWorkspace(ctx context.Context, issueID int) (*workspace.Workspace, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) CleanupWorkspace(ctx context.Context, issueID int) error { return nil }
func (m *mockWorkspaceManager) CleanupStaleWorkspaces(ctx context.Context, olderThan time.Duration) error {
	return nil
}
func (m *mockWorkspaceManager) GetWorkspaceStats(ctx context.Context) (*workspace.WorkspaceStats, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) CheckDiskSpace(ctx context.Context, requiredMB int64) error {
	return nil
}
func (m *mockWorkspaceManager) GetWorkspace(ctx context.Context, issueID int) (*workspace.Workspace, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) UpdateWorkspaceState(ctx context.Context, issueID int, s workspace.WorkspaceState) error {
	return nil
}
func (m *mockWorkspaceManager) ListWorkspaces(ctx context.Context, s workspace.WorkspaceState) ([]*workspace.Workspace, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) CreateWorkspaceSnapshot(ctx context.Context, issueID int, agentState *state.AgentWorkState) (*workspace.WorkspaceSnapshot, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) RestoreWorkspaceSnapshot(ctx context.Context, issueID int) (*workspace.WorkspaceSnapshot, error) {
	return nil, nil
}
func (m *mockWorkspaceManager) GetWorkspaceSnapshots(ctx context.Context, issueID int) ([]*workspace.WorkspaceSnapshot, error) {
	return nil, nil
}

func TestRun_WithAllOptionalComponents(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return nil
	}

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	scheduler := workspace.NewScheduler(&mockWorkspaceManager{}, workspace.ManagerConfig{
		CleanupInterval: 1 * time.Hour,
	}, slog.Default())
	rotationMgr := observability.NewLogRotationManager(config.LogRotationConfig{
		Enabled:       true,
		CheckInterval: 1 * time.Hour,
	})
	cleanupMgr := observability.NewLogCleanupManager(config.LogCleanupConfig{
		Enabled:         true,
		CleanupInterval: 1 * time.Hour,
	})

	logDir := t.TempDir()

	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m).
		WithWorkspaceCleanup(scheduler).
		WithLogRotation(rotationMgr).
		WithLogCleanup(cleanupMgr).
		WithLogFilePath(logDir + "/agent.log")

	err := o.Run(context.Background())
	assert.NoError(t, err)
}

func TestRun_WithAllOptionalComponents_AgentError(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.runFn = func(ctx context.Context) error {
		return errors.New("fatal error")
	}

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	scheduler := workspace.NewScheduler(&mockWorkspaceManager{}, workspace.ManagerConfig{
		CleanupInterval: 1 * time.Hour,
	}, slog.Default())

	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m).
		WithWorkspaceCleanup(scheduler)

	err := o.Run(context.Background())
	assert.Error(t, err)
}

func TestRun_WithAllOptionalComponents_ContextCancel(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	// Default: blocks until ctx cancellation

	sl := newTestStructuredLogger()
	m := newTestMetrics()
	scheduler := workspace.NewScheduler(&mockWorkspaceManager{}, workspace.ManagerConfig{
		CleanupInterval: 1 * time.Hour,
	}, slog.Default())
	rotationMgr := observability.NewLogRotationManager(config.LogRotationConfig{
		Enabled:       true,
		CheckInterval: 1 * time.Hour,
	})
	cleanupMgr := observability.NewLogCleanupManager(config.LogCleanupConfig{
		Enabled:         true,
		CleanupInterval: 1 * time.Hour,
	})

	logDir := t.TempDir()

	o := New([]agent.Agent{ma}, slog.Default()).
		WithObservability(sl, m).
		WithWorkspaceCleanup(scheduler).
		WithLogRotation(rotationMgr).
		WithLogCleanup(cleanupMgr).
		WithLogFilePath(logDir + "/agent.log")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- o.Run(ctx) }()

	// Give agents time to start
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
