package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements state.Store for testing.
type mockStore struct {
	saved  []*state.AgentWorkState
	loadFn func(ctx context.Context, agentType string) (*state.AgentWorkState, error)
	saveFn func(ctx context.Context, s *state.AgentWorkState) error
	states map[string]*state.AgentWorkState
}

func newMockStore() *mockStore {
	return &mockStore{
		states: make(map[string]*state.AgentWorkState),
	}
}

func (m *mockStore) Save(ctx context.Context, s *state.AgentWorkState) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, s)
	}
	m.saved = append(m.saved, s)
	m.states[s.AgentType] = s
	return nil
}

func (m *mockStore) Load(ctx context.Context, agentType string) (*state.AgentWorkState, error) {
	if m.loadFn != nil {
		return m.loadFn(ctx, agentType)
	}
	s, ok := m.states[agentType]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockStore) Delete(ctx context.Context, agentType string) error {
	delete(m.states, agentType)
	return nil
}

func (m *mockStore) List(ctx context.Context) ([]*state.AgentWorkState, error) {
	var result []*state.AgentWorkState
	for _, s := range m.states {
		result = append(result, s)
	}
	return result, nil
}

func defaultShutdownConfig() *config.Config {
	return &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: false,
		},
	}
}

func TestNewShutdownManager(t *testing.T) {
	agents := []agent.Agent{newMockAgent(agent.TypeDeveloper)}
	store := newMockStore()
	cfg := defaultShutdownConfig()

	sm := NewShutdownManager(agents, store, cfg, slog.Default())
	require.NotNil(t, sm)
	assert.Len(t, sm.agents, 1)
	assert.Equal(t, 10*time.Second, sm.shutdownTimeout)
}

func TestNewShutdownManager_DefaultTimeout(t *testing.T) {
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout: 0,
		},
	}
	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	assert.Equal(t, 30*time.Second, sm.shutdownTimeout)
}

func TestNewShutdownManager_CustomTimeout(t *testing.T) {
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout: 5 * time.Second,
		},
	}
	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	assert.Equal(t, 5*time.Second, sm.shutdownTimeout)
}

func TestWithObservability_Shutdown(t *testing.T) {
	sm := NewShutdownManager(nil, newMockStore(), defaultShutdownConfig(), slog.Default())
	result := sm.WithObservability(nil)
	assert.Same(t, sm, result)
}

func TestAddCleanupHandler(t *testing.T) {
	sm := NewShutdownManager(nil, newMockStore(), defaultShutdownConfig(), slog.Default())
	assert.Len(t, sm.cleanupHandlers, 0)

	sm.AddCleanupHandler(func(ctx context.Context) error { return nil })
	assert.Len(t, sm.cleanupHandlers, 1)

	sm.AddCleanupHandler(func(ctx context.Context) error { return nil })
	assert.Len(t, sm.cleanupHandlers, 2)
}

func TestShutdown_Success_IdleAgents(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{Type: agent.TypeDeveloper, State: string(state.StateIdle)})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
	// Idle agents should not save checkpoints
	assert.Empty(t, store.saved)
}

func TestShutdown_SavesCheckpoints_ActiveAgents(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(state.StateImplement),
		IssueID: 42,
	})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "developer", store.saved[0].AgentType)
	assert.Equal(t, 42, store.saved[0].IssueNumber)
	assert.Equal(t, state.StateImplement, store.saved[0].State)
	assert.Equal(t, "graceful_shutdown", store.saved[0].InterruptedBy)
}

func TestShutdown_SaveCheckpointFails(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:  agent.TypeDeveloper,
		State: string(state.StateImplement),
	})

	store := newMockStore()
	store.saveFn = func(ctx context.Context, s *state.AgentWorkState) error {
		return errors.New("disk full")
	}

	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())
	// Shutdown should not fail even if checkpoint saving fails
	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_RunsCleanupHandlers(t *testing.T) {
	store := newMockStore()
	sm := NewShutdownManager(nil, store, defaultShutdownConfig(), slog.Default())

	called := false
	sm.AddCleanupHandler(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
	assert.True(t, called, "cleanup handler should have been called")
}

func TestShutdown_CleanupHandlerError(t *testing.T) {
	store := newMockStore()
	sm := NewShutdownManager(nil, store, defaultShutdownConfig(), slog.Default())

	sm.AddCleanupHandler(func(ctx context.Context) error {
		return errors.New("cleanup failed")
	})

	err := sm.Shutdown(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup failed")
}

func TestShutdown_MultipleCleanupHandlers(t *testing.T) {
	sm := NewShutdownManager(nil, newMockStore(), defaultShutdownConfig(), slog.Default())

	order := make([]int, 0)
	sm.AddCleanupHandler(func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	sm.AddCleanupHandler(func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})
	sm.AddCleanupHandler(func(ctx context.Context) error {
		order = append(order, 3)
		return nil
	})

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, order)
}

func TestShutdown_CleanupWorkspaces(t *testing.T) {
	workDir := t.TempDir()

	// Create issue directories
	issueDir := filepath.Join(workDir, "issue-42")
	err := os.MkdirAll(issueDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(issueDir, "file.go"), []byte("test"), 0o644)
	require.NoError(t, err)

	// Create a non-issue directory that should not be removed
	otherDir := filepath.Join(workDir, "config")
	err = os.MkdirAll(otherDir, 0o755)
	require.NoError(t, err)

	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: true,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: workDir,
			},
		},
	}

	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	err = sm.Shutdown(context.Background())
	assert.NoError(t, err)

	// issue-42 directory should be removed
	_, err = os.Stat(issueDir)
	assert.True(t, os.IsNotExist(err), "issue directory should be removed")

	// config directory should still exist
	_, err = os.Stat(otherDir)
	assert.NoError(t, err, "non-issue directory should remain")
}

func TestShutdown_CleanupWorkspaces_EmptyDir(t *testing.T) {
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: true,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: "",
			},
		},
	}

	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_CleanupWorkspaces_NonExistentDir(t *testing.T) {
	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: true,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: "/nonexistent/workspace/path",
			},
		},
	}

	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_ContextTimeout(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{Type: agent.TypeDeveloper, State: string(state.StateIdle)})

	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout: 100 * time.Millisecond,
		},
	}

	sm := NewShutdownManager([]agent.Agent{ma}, newMockStore(), cfg, slog.Default())

	// Add slow cleanup handler
	sm.AddCleanupHandler(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	err := sm.Shutdown(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup")
}

func TestRunCleanupHandlers_Empty(t *testing.T) {
	sm := NewShutdownManager(nil, newMockStore(), defaultShutdownConfig(), slog.Default())
	err := sm.runCleanupHandlers(context.Background())
	assert.NoError(t, err)
}

func TestRunCleanupHandlers_CancelledContext(t *testing.T) {
	sm := NewShutdownManager(nil, newMockStore(), defaultShutdownConfig(), slog.Default())

	sm.AddCleanupHandler(func(ctx context.Context) error {
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sm.runCleanupHandlers(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup timeout")
}

func TestSaveCheckpoints_MultipleAgents(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(state.StateImplement),
		IssueID: 10,
	})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{
		Type:  agent.TypeQA,
		State: string(state.StateIdle), // idle -> no checkpoint
	})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma1, ma2}, store, defaultShutdownConfig(), slog.Default())

	err := sm.saveCheckpoints(context.Background())
	assert.NoError(t, err)
	// Only the non-idle agent should have a checkpoint
	assert.Len(t, store.saved, 1)
	assert.Equal(t, "developer", store.saved[0].AgentType)
}

func TestSaveCheckpoints_CancelledContext(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:  agent.TypeDeveloper,
		State: string(state.StateImplement),
	})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sm.saveCheckpoints(ctx)
	assert.Error(t, err)
}

func TestRunEmergencyCleanup(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(state.StateImplement),
		IssueID: 99,
	})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())

	// runEmergencyCleanup should not panic and should save checkpoints
	sm.runEmergencyCleanup()
	assert.Len(t, store.saved, 1)
}

func TestRunEmergencyCleanup_IdleAgents(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:  agent.TypeDeveloper,
		State: string(state.StateIdle),
	})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())

	sm.runEmergencyCleanup()
	assert.Empty(t, store.saved)
}

func TestCleanupWorkspaces_WithContextCancellation(t *testing.T) {
	workDir := t.TempDir()

	// Create multiple issue directories
	for i := 0; i < 5; i++ {
		dir := filepath.Join(workDir, "issue-"+string(rune('0'+i)))
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
	}

	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: true,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: workDir,
			},
		},
	}

	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sm.cleanupWorkspaces(ctx)
	assert.Error(t, err)
}

func TestShutdown_WithObservability_Nil(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{Type: agent.TypeDeveloper, State: string(state.StateIdle)})

	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())
	sm.WithObservability(nil)

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_WithRealObservability(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(state.StateImplement),
		IssueID: 10,
	})

	sl := observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
	store := newMockStore()
	sm := NewShutdownManager([]agent.Agent{ma}, store, defaultShutdownConfig(), slog.Default())
	sm.WithObservability(sl)

	err := sm.Shutdown(context.Background())
	assert.NoError(t, err)
	require.Len(t, store.saved, 1)
}

func TestShutdown_WithCleanupWorkspaces_AndObservability(t *testing.T) {
	workDir := t.TempDir()
	issueDir := filepath.Join(workDir, "issue-55")
	err := os.MkdirAll(issueDir, 0o755)
	require.NoError(t, err)

	cfg := &config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:           10 * time.Second,
			CleanupWorkspaces: true,
		},
		Agents: config.AgentsConfig{
			Developer: config.DeveloperAgentConfig{
				WorkspaceDir: workDir,
			},
		},
	}

	sl := observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
	sm := NewShutdownManager(nil, newMockStore(), cfg, slog.Default())
	sm.WithObservability(sl)

	err = sm.Shutdown(context.Background())
	assert.NoError(t, err)
}
