package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/stretchr/testify/assert"
)

// --- AgeBasedStrategy tests ---

func TestAgeBasedStrategy_ShouldCleanup_Active(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 168 * time.Hour,
		StaleRetention:   168 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateActive,
		UpdatedAt: time.Now().Add(-48 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "active workspace", reason)
}

func TestAgeBasedStrategy_ShouldCleanup_FailedOld(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 1 * time.Hour,
		StaleRetention:   1 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Contains(t, reason, "failed workspace older than")
}

func TestAgeBasedStrategy_ShouldCleanup_FailedRecent(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 24 * time.Hour,
		StaleRetention:   24 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "within retention period", reason)
}

func TestAgeBasedStrategy_ShouldCleanup_StaleOld(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 24 * time.Hour,
		StaleRetention:   1 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateStale,
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Contains(t, reason, "stale workspace older than")
}

func TestAgeBasedStrategy_ShouldCleanup_StaleRecent(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 24 * time.Hour,
		StaleRetention:   24 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateStale,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "within retention period", reason)
}

func TestAgeBasedStrategy_ShouldCleanup_AlreadyCleaned(t *testing.T) {
	strategy := &AgeBasedStrategy{
		SuccessRetention: 24 * time.Hour,
		FailureRetention: 24 * time.Hour,
		StaleRetention:   24 * time.Hour,
	}

	ws := &Workspace{
		State:     WorkspaceStateCleaned,
		UpdatedAt: time.Now(),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Equal(t, "workspace already marked as cleaned", reason)
}

// --- SizeBasedStrategy tests ---

func TestSizeBasedStrategy_ShouldCleanup_ExceedsLimit(t *testing.T) {
	strategy := &SizeBasedStrategy{MaxSizeMB: 100}

	ws := &Workspace{
		SizeMB: 200,
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Contains(t, reason, "exceeds limit")
}

func TestSizeBasedStrategy_ShouldCleanup_WithinLimit(t *testing.T) {
	strategy := &SizeBasedStrategy{MaxSizeMB: 100}

	ws := &Workspace{
		SizeMB: 50,
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "within size limit", reason)
}

func TestSizeBasedStrategy_ShouldCleanup_ExactLimit(t *testing.T) {
	strategy := &SizeBasedStrategy{MaxSizeMB: 100}

	ws := &Workspace{
		SizeMB: 100,
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "within size limit", reason)
}

// --- CompositeStrategy tests ---

func TestCompositeStrategy_AnyMode_OneMatches(t *testing.T) {
	strategy := &CompositeStrategy{
		Mode: "any",
		Strategies: []CleanupStrategy{
			&SizeBasedStrategy{MaxSizeMB: 100},
			&AgeBasedStrategy{
				SuccessRetention: 24 * time.Hour,
				FailureRetention: 24 * time.Hour,
				StaleRetention:   24 * time.Hour,
			},
		},
	}

	ws := &Workspace{
		SizeMB:    200, // Exceeds size limit
		State:     WorkspaceStateActive,
		UpdatedAt: time.Now(), // Recent (won't match age)
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Contains(t, reason, "matches 1 strategies")
}

func TestCompositeStrategy_AnyMode_NoneMatch(t *testing.T) {
	strategy := &CompositeStrategy{
		Mode: "any",
		Strategies: []CleanupStrategy{
			&SizeBasedStrategy{MaxSizeMB: 100},
			&AgeBasedStrategy{
				SuccessRetention: 24 * time.Hour,
				FailureRetention: 24 * time.Hour,
				StaleRetention:   24 * time.Hour,
			},
		},
	}

	ws := &Workspace{
		SizeMB:    50,
		State:     WorkspaceStateActive,
		UpdatedAt: time.Now(),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "cleanup criteria not met", reason)
}

func TestCompositeStrategy_AllMode_AllMatch(t *testing.T) {
	strategy := &CompositeStrategy{
		Mode: "all",
		Strategies: []CleanupStrategy{
			&SizeBasedStrategy{MaxSizeMB: 100},
			&AgeBasedStrategy{
				SuccessRetention: 1 * time.Hour,
				FailureRetention: 1 * time.Hour,
				StaleRetention:   1 * time.Hour,
			},
		},
	}

	ws := &Workspace{
		SizeMB:    200,
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.True(t, shouldCleanup)
	assert.Contains(t, reason, "matches all strategies")
}

func TestCompositeStrategy_AllMode_OnlyOneMatches(t *testing.T) {
	strategy := &CompositeStrategy{
		Mode: "all",
		Strategies: []CleanupStrategy{
			&SizeBasedStrategy{MaxSizeMB: 100},
			&AgeBasedStrategy{
				SuccessRetention: 24 * time.Hour,
				FailureRetention: 24 * time.Hour,
				StaleRetention:   24 * time.Hour,
			},
		},
	}

	ws := &Workspace{
		SizeMB:    200, // Exceeds size
		State:     WorkspaceStateActive,
		UpdatedAt: time.Now(), // Not old
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "cleanup criteria not met", reason)
}

func TestCompositeStrategy_UnknownMode(t *testing.T) {
	strategy := &CompositeStrategy{
		Mode: "unknown",
		Strategies: []CleanupStrategy{
			&SizeBasedStrategy{MaxSizeMB: 100},
		},
	}

	ws := &Workspace{
		SizeMB: 200,
	}

	shouldCleanup, reason := strategy.ShouldCleanup(context.Background(), ws)
	assert.False(t, shouldCleanup)
	assert.Equal(t, "cleanup criteria not met", reason)
}

// --- mockManager for Scheduler tests ---

type mockManager struct {
	workspaces    []*Workspace
	cleanupCalls  []int
	cleanupErrors map[int]error
	listError     error
}

func (m *mockManager) CreateWorkspace(_ context.Context, _ int) (*Workspace, error) {
	return nil, nil
}
func (m *mockManager) CleanupWorkspace(_ context.Context, issueID int) error {
	m.cleanupCalls = append(m.cleanupCalls, issueID)
	if err, ok := m.cleanupErrors[issueID]; ok {
		return err
	}
	return nil
}
func (m *mockManager) CleanupStaleWorkspaces(_ context.Context, _ time.Duration) error { return nil }
func (m *mockManager) GetWorkspaceStats(_ context.Context) (*WorkspaceStats, error)    { return nil, nil }
func (m *mockManager) CheckDiskSpace(_ context.Context, _ int64) error                 { return nil }
func (m *mockManager) GetWorkspace(_ context.Context, _ int) (*Workspace, error)       { return nil, nil }
func (m *mockManager) UpdateWorkspaceState(_ context.Context, _ int, _ WorkspaceState) error {
	return nil
}
func (m *mockManager) ListWorkspaces(_ context.Context, _ WorkspaceState) ([]*Workspace, error) {
	if m.listError != nil {
		return nil, m.listError
	}
	return m.workspaces, nil
}
func (m *mockManager) CreateWorkspaceSnapshot(_ context.Context, _ int, _ *state.AgentWorkState) (*WorkspaceSnapshot, error) {
	return nil, nil
}
func (m *mockManager) RestoreWorkspaceSnapshot(_ context.Context, _ int) (*WorkspaceSnapshot, error) {
	return nil, nil
}
func (m *mockManager) GetWorkspaceSnapshots(_ context.Context, _ int) ([]*WorkspaceSnapshot, error) {
	return nil, nil
}

// --- Scheduler tests ---

func TestNewScheduler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr := &mockManager{}
	cfg := DefaultConfig()

	scheduler := NewScheduler(mgr, cfg, logger)
	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.strategy)
	assert.Equal(t, mgr, scheduler.manager)
}

func TestNewSchedulerWithAppConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr := &mockManager{}
	cfg := DefaultConfig()

	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)
	assert.NotNil(t, scheduler)
}

func TestScheduler_Start_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr := &mockManager{}
	cfg := DefaultConfig()
	cfg.CleanupEnabled = false

	scheduler := NewScheduler(mgr, cfg, logger)
	scheduler.Start(context.Background())

	// stoppedCh should be closed immediately when disabled
	select {
	case <-scheduler.stoppedCh:
		// Expected behavior
	case <-time.After(1 * time.Second):
		t.Fatal("stoppedCh not closed for disabled scheduler")
	}
}

func TestScheduler_Start_Stop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr := &mockManager{workspaces: []*Workspace{}}
	cfg := DefaultConfig()
	cfg.CleanupEnabled = true
	cfg.CleanupInterval = 50 * time.Millisecond

	scheduler := NewScheduler(mgr, cfg, logger)
	scheduler.Start(context.Background())

	// Wait a bit for at least one cycle to run
	time.Sleep(100 * time.Millisecond)

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete in time")
	}
}

func TestScheduler_ForceCleanup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	oldWorkspace := &Workspace{
		ID:        1,
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-200 * time.Hour),
		SizeMB:    10,
	}

	mgr := &mockManager{
		workspaces:    []*Workspace{oldWorkspace},
		cleanupErrors: make(map[int]error),
	}

	cfg := DefaultConfig()
	cfg.CleanupEnabled = true
	cfg.FailureRetention = 1 * time.Hour
	cfg.CleanupInterval = 1 * time.Hour

	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)

	err := scheduler.ForceCleanup(context.Background())
	assert.NoError(t, err)
	assert.Contains(t, mgr.cleanupCalls, 1)
}

func TestScheduler_RunCleanupCycle_ListError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := &mockManager{
		listError: fmt.Errorf("list error"),
	}

	cfg := DefaultConfig()
	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)

	// Should not panic even if ListWorkspaces fails
	scheduler.runCleanupCycle(context.Background())
}

func TestScheduler_RunCleanupCycle_CleanupError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ws := &Workspace{
		ID:        1,
		State:     WorkspaceStateCleaned,
		UpdatedAt: time.Now(),
		SizeMB:    10,
	}

	mgr := &mockManager{
		workspaces:    []*Workspace{ws},
		cleanupErrors: map[int]error{1: fmt.Errorf("cleanup failed")},
	}

	cfg := DefaultConfig()
	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)

	// Should not panic even if CleanupWorkspace fails
	scheduler.runCleanupCycle(context.Background())
	assert.Contains(t, mgr.cleanupCalls, 1)
}

func TestScheduler_RunCleanupCycle_ContextCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	workspaces := []*Workspace{
		{ID: 1, State: WorkspaceStateCleaned, UpdatedAt: time.Now(), SizeMB: 10},
		{ID: 2, State: WorkspaceStateCleaned, UpdatedAt: time.Now(), SizeMB: 10},
	}

	mgr := &mockManager{
		workspaces:    workspaces,
		cleanupErrors: make(map[int]error),
	}

	cfg := DefaultConfig()
	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	scheduler.runCleanupCycle(ctx)
	// First workspace might be cleaned, second should be skipped due to context cancellation
}

func TestScheduler_Run_ContextCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr := &mockManager{workspaces: []*Workspace{}}
	cfg := DefaultConfig()
	cfg.CleanupInterval = 100 * time.Millisecond

	scheduler := NewSchedulerWithAppConfig(mgr, cfg, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		scheduler.run(ctx)
		close(done)
	}()

	// Let it run one cycle
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not exit after context cancellation")
	}
}

// --- CleanupWorkspacesByAge tests ---

func TestCleanupWorkspacesByAge_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	oldStaleWs := &Workspace{
		ID:        1,
		State:     WorkspaceStateStale,
		UpdatedAt: time.Now().Add(-48 * time.Hour),
	}
	recentActiveWs := &Workspace{
		ID:        2,
		State:     WorkspaceStateActive,
		UpdatedAt: time.Now(),
	}

	mgr := &mockManager{
		workspaces:    []*Workspace{oldStaleWs, recentActiveWs},
		cleanupErrors: make(map[int]error),
	}

	err := CleanupWorkspacesByAge(context.Background(), mgr, 24*time.Hour, logger)
	assert.NoError(t, err)

	// Only workspace 1 should be cleaned (old and stale)
	assert.Contains(t, mgr.cleanupCalls, 1)
}

func TestCleanupWorkspacesByAge_ListError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mgr := &mockManager{
		listError: fmt.Errorf("list error"),
	}

	err := CleanupWorkspacesByAge(context.Background(), mgr, 24*time.Hour, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listing workspaces")
}

func TestCleanupWorkspacesByAge_CleanupError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	oldFailed := &Workspace{
		ID:        1,
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-48 * time.Hour),
	}

	mgr := &mockManager{
		workspaces:    []*Workspace{oldFailed},
		cleanupErrors: map[int]error{1: fmt.Errorf("cleanup error")},
	}

	// Should not return error, just log and continue
	err := CleanupWorkspacesByAge(context.Background(), mgr, 24*time.Hour, logger)
	assert.NoError(t, err)
}

func TestCleanupWorkspacesByAge_ContextCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	workspaces := []*Workspace{
		{ID: 1, State: WorkspaceStateFailed, UpdatedAt: time.Now().Add(-48 * time.Hour)},
		{ID: 2, State: WorkspaceStateFailed, UpdatedAt: time.Now().Add(-48 * time.Hour)},
	}

	mgr := &mockManager{
		workspaces:    workspaces,
		cleanupErrors: make(map[int]error),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := CleanupWorkspacesByAge(ctx, mgr, 24*time.Hour, logger)
	assert.Error(t, err) // Should return context error
}

func TestCleanupWorkspacesByAge_SkipsRecentWorkspaces(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	recentFailed := &Workspace{
		ID:        1,
		State:     WorkspaceStateFailed,
		UpdatedAt: time.Now().Add(-1 * time.Hour), // Within cutoff
	}

	mgr := &mockManager{
		workspaces:    []*Workspace{recentFailed},
		cleanupErrors: make(map[int]error),
	}

	err := CleanupWorkspacesByAge(context.Background(), mgr, 24*time.Hour, logger)
	assert.NoError(t, err)
	assert.Empty(t, mgr.cleanupCalls) // Should not clean recent workspace
}
