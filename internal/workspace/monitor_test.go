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

func TestMonitor_GetDiskStats(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MinFreeDiskMB: 100,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	stats, err := monitor.GetDiskStats(tempDir)
	require.NoError(t, err)

	assert.Greater(t, stats.TotalMB, int64(0))
	assert.Greater(t, stats.AvailableMB, int64(0))
	assert.GreaterOrEqual(t, stats.UsedMB, int64(0))
	assert.GreaterOrEqual(t, stats.UsagePercent, 0.0)
	assert.LessOrEqual(t, stats.UsagePercent, 100.0)
}

func TestMonitor_CheckDiskSpace(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MinFreeDiskMB: 100, // Reasonable minimum
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	ctx := context.Background()

	// Test with reasonable requirement - should pass
	err := monitor.CheckDiskSpace(ctx, 10)
	assert.NoError(t, err)

	// Test with very large requirement - should fail
	err = monitor.CheckDiskSpace(ctx, 1024*1024*1024) // 1TB requirement
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient disk space")
}

func TestMonitor_CheckDiskSpace_MinimumRequirement(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MinFreeDiskMB: 1024*1024*1024, // Very large minimum to trigger failure
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	ctx := context.Background()

	// Should fail due to minimum free disk requirement
	err := monitor.CheckDiskSpace(ctx, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minimum required")
}

func TestMonitor_ValidateWorkspaceSize(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:   tempDir,
		MaxSizeMB: 1, // 1MB limit
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	ctx := context.Background()

	// Create test workspace
	workspaceDir := filepath.Join(tempDir, "test-workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0755))

	// Create small file - should pass
	smallFile := filepath.Join(workspaceDir, "small.txt")
	require.NoError(t, os.WriteFile(smallFile, []byte("small content"), 0644))

	err := monitor.ValidateWorkspaceSize(ctx, workspaceDir)
	assert.NoError(t, err)

	// Create large file - should fail
	largeFile := filepath.Join(workspaceDir, "large.txt")
	largeContent := make([]byte, 2*1024*1024) // 2MB content
	require.NoError(t, os.WriteFile(largeFile, largeContent, 0644))

	err = monitor.ValidateWorkspaceSize(ctx, workspaceDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestMonitor_GetResourceUsage(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MinFreeDiskMB: 100,
		MaxSizeMB:     500,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	ctx := context.Background()

	usage, err := monitor.GetResourceUsage(ctx)
	require.NoError(t, err)

	// Check structure
	assert.Contains(t, usage, "disk")
	assert.Contains(t, usage, "thresholds")
	assert.Contains(t, usage, "timestamp")

	diskUsage, ok := usage["disk"].(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, diskUsage, "total_mb")
	assert.Contains(t, diskUsage, "used_mb")
	assert.Contains(t, diskUsage, "available_mb")
	assert.Contains(t, diskUsage, "usage_percent")

	thresholds, ok := usage["thresholds"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, int64(100), thresholds["min_free_mb"])
	assert.Equal(t, int64(500), thresholds["max_size_mb"])

	timestamp, ok := usage["timestamp"].(time.Time)
	require.True(t, ok)
	assert.True(t, timestamp.After(time.Now().Add(-time.Minute)))
}

func TestMonitor_MonitorDiskSpace(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:           tempDir,
		MinFreeDiskMB:     100,
		DiskCheckInterval: 10 * time.Millisecond, // Very short for testing
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should run a few monitoring cycles before context is cancelled
	monitor.MonitorDiskSpace(ctx)

	// Test passes if no panic occurs and context cancellation is handled gracefully
}

func TestMonitor_CheckAndLogDiskUsage(t *testing.T) {
	tempDir := t.TempDir()
	config := ManagerConfig{
		BaseDir:       tempDir,
		MinFreeDiskMB: 100,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	// This should not error under normal conditions
	err := monitor.checkAndLogDiskUsage()
	assert.NoError(t, err)
}

func TestNewMonitor(t *testing.T) {
	config := ManagerConfig{
		BaseDir:           "./test",
		MinFreeDiskMB:     200,
		DiskCheckInterval: 5 * time.Minute,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	monitor := NewMonitor(config, logger)

	assert.NotNil(t, monitor)
	assert.Equal(t, config, monitor.config)
	assert.Equal(t, logger, monitor.logger)
}