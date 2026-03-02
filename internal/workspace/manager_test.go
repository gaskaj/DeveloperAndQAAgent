package workspace

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_CreateWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 3,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Test creating a new workspace
	workspace, err := manager.CreateWorkspace(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, 123, workspace.ID)
	assert.Equal(t, filepath.Join(tempDir, "issue-123"), workspace.Path)
	assert.Equal(t, WorkspaceStateActive, workspace.State)
	assert.True(t, workspace.CreatedAt.After(time.Time{}))

	// Verify directory was created
	_, err = os.Stat(workspace.Path)
	assert.NoError(t, err)

	// Test creating duplicate workspace returns existing
	workspace2, err := manager.CreateWorkspace(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, workspace.ID, workspace2.ID)
	assert.Equal(t, workspace.Path, workspace2.Path)
}

func TestManager_CleanupWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 3,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create workspace
	workspace, err := manager.CreateWorkspace(ctx, 456)
	require.NoError(t, err)

	// Create a test file in workspace
	testFile := filepath.Join(workspace.Path, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Verify workspace exists
	_, err = os.Stat(workspace.Path)
	require.NoError(t, err)

	// Cleanup workspace
	err = manager.CleanupWorkspace(ctx, 456)
	require.NoError(t, err)

	// Verify workspace is gone
	_, err = os.Stat(workspace.Path)
	assert.True(t, os.IsNotExist(err))
}

func TestManager_GetWorkspaceStats(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 3,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Initially no workspaces
	stats, err := manager.GetWorkspaceStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.TotalWorkspaces)
	assert.Equal(t, 0, stats.ActiveWorkspaces)

	// Create some workspaces
	_, err = manager.CreateWorkspace(ctx, 1)
	require.NoError(t, err)
	_, err = manager.CreateWorkspace(ctx, 2)
	require.NoError(t, err)

	// Check stats
	stats, err = manager.GetWorkspaceStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalWorkspaces)
	assert.Equal(t, 2, stats.ActiveWorkspaces)

	// Update one workspace state
	err = manager.UpdateWorkspaceState(ctx, 1, WorkspaceStateStale)
	require.NoError(t, err)

	// Check stats again
	stats, err = manager.GetWorkspaceStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalWorkspaces)
	assert.Equal(t, 1, stats.ActiveWorkspaces)
	assert.Equal(t, 1, stats.StaleWorkspaces)
}

func TestManager_CleanupStaleWorkspaces(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:        tempDir,
		MaxSizeMB:      100,
		MinFreeDiskMB:  50,
		MaxConcurrent:  3,
		CleanupEnabled: true,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create workspaces with different states
	workspace1, err := manager.CreateWorkspace(ctx, 1)
	require.NoError(t, err)
	workspace2, err := manager.CreateWorkspace(ctx, 2)
	require.NoError(t, err)

	// Mark one as stale with old update time
	err = manager.UpdateWorkspaceState(ctx, 1, WorkspaceStateStale)
	require.NoError(t, err)

	// Manually set old update time
	impl := manager.(*managerImpl)
	impl.workspaces[1].UpdatedAt = time.Now().Add(-2 * time.Hour)

	// Cleanup stale workspaces older than 1 hour
	err = manager.CleanupStaleWorkspaces(ctx, 1*time.Hour)
	require.NoError(t, err)

	// Verify stale workspace is gone
	_, err = os.Stat(workspace1.Path)
	assert.True(t, os.IsNotExist(err))

	// Verify active workspace remains
	_, err = os.Stat(workspace2.Path)
	assert.NoError(t, err)
}

func TestManager_CheckDiskSpace(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 1024 * 1024 * 1024, // Very large requirement (1TB) to trigger failure
		MaxConcurrent: 3,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// This should fail due to insufficient disk space (minimum requirement)
	err = manager.CheckDiskSpace(ctx, 100)
	if err != nil {
		assert.Contains(t, err.Error(), "insufficient disk space")
	} else {
		// If the minimum disk space check didn't fail, try with a very large requirement
		err = manager.CheckDiskSpace(ctx, 1024*1024*1024) // 1TB requirement
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient disk space")
	}
}

func TestManager_MaxConcurrent(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 2, // Limit to 2 concurrent workspaces
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Create maximum concurrent workspaces
	_, err = manager.CreateWorkspace(ctx, 1)
	require.NoError(t, err)
	_, err = manager.CreateWorkspace(ctx, 2)
	require.NoError(t, err)

	// Third workspace should fail
	_, err = manager.CreateWorkspace(ctx, 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrent limit exceeded")

	// Mark one workspace as stale
	err = manager.UpdateWorkspaceState(ctx, 1, WorkspaceStateStale)
	require.NoError(t, err)

	// Now third workspace should succeed
	_, err = manager.CreateWorkspace(ctx, 3)
	assert.NoError(t, err)
}

func TestManager_LoadExistingWorkspaces(t *testing.T) {
	tempDir := t.TempDir()

	// Create some workspace directories manually
	workspace1Dir := filepath.Join(tempDir, "issue-100")
	workspace2Dir := filepath.Join(tempDir, "issue-200")
	invalidDir := filepath.Join(tempDir, "not-a-workspace")

	require.NoError(t, os.MkdirAll(workspace1Dir, 0755))
	require.NoError(t, os.MkdirAll(workspace2Dir, 0755))
	require.NoError(t, os.MkdirAll(invalidDir, 0755))

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(workspace1Dir, "file.txt"), []byte("test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(workspace2Dir, "file.txt"), []byte("test content"), 0644))

	config := ManagerConfig{
		BaseDir:       tempDir,
		MaxSizeMB:     100,
		MinFreeDiskMB: 50,
		MaxConcurrent: 3,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Check that existing workspaces were loaded
	stats, err := manager.GetWorkspaceStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalWorkspaces) // Only valid workspace directories

	// Check workspace details
	workspace1, err := manager.GetWorkspace(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, workspace1Dir, workspace1.Path)
	assert.Equal(t, WorkspaceStateStale, workspace1.State) // Loaded workspaces are marked stale

	workspace2, err := manager.GetWorkspace(ctx, 200)
	require.NoError(t, err)
	assert.Equal(t, workspace2Dir, workspace2.Path)
	assert.Equal(t, WorkspaceStateStale, workspace2.State)
}

func TestManagerConfig_Defaults(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "./workspaces", config.BaseDir)
	assert.Equal(t, int64(1024), config.MaxSizeMB)
	assert.Equal(t, int64(2048), config.MinFreeDiskMB)
	assert.Equal(t, 5, config.MaxConcurrent)
	assert.Equal(t, 24*time.Hour, config.SuccessRetention)
	assert.Equal(t, 168*time.Hour, config.FailureRetention)
	assert.Equal(t, 5*time.Minute, config.DiskCheckInterval)
	assert.Equal(t, 1*time.Hour, config.CleanupInterval)
	assert.True(t, config.CleanupEnabled)
}