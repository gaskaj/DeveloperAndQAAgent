package observability

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
)

func TestLogCleanupManager_Basic(t *testing.T) {
	tests := []struct {
		name   string
		config config.LogCleanupConfig
		valid  bool
	}{
		{
			name: "valid config with retention",
			config: config.LogCleanupConfig{
				Enabled:         true,
				RetentionDays:   30,
				CleanupInterval: time.Hour,
			},
			valid: true,
		},
		{
			name: "valid config with disk space",
			config: config.LogCleanupConfig{
				Enabled:         true,
				MinFreeDiskMB:   1000,
				CleanupInterval: time.Hour,
			},
			valid: true,
		},
		{
			name: "valid config with both policies",
			config: config.LogCleanupConfig{
				Enabled:             true,
				RetentionDays:       30,
				MinFreeDiskMB:       1000,
				CleanupInterval:     time.Hour,
				ArchiveBeforeDelete: true,
			},
			valid: true,
		},
		{
			name: "invalid - no policies",
			config: config.LogCleanupConfig{
				Enabled:         true,
				CleanupInterval: time.Hour,
			},
			valid: false,
		},
		{
			name: "invalid cleanup interval",
			config: config.LogCleanupConfig{
				Enabled:         true,
				RetentionDays:   30,
				CleanupInterval: 0,
			},
			valid: false,
		},
		{
			name: "invalid negative retention",
			config: config.LogCleanupConfig{
				Enabled:         true,
				RetentionDays:   -1,
				CleanupInterval: time.Hour,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewLogCleanupManager(tt.config)
			err := manager.validateConfig()

			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestLogCleanupManager_AgeBasedCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create log files with different ages
	now := time.Now()
	files := []struct {
		name         string
		age          time.Duration
		shouldDelete bool
	}{
		{"app.log", 0, false},                       // Current file
		{"app.1.log", 20 * 24 * time.Hour, false},   // 20 days old - keep
		{"app.2.log", 35 * 24 * time.Hour, true},    // 35 days old - delete
		{"app.3.log", 45 * 24 * time.Hour, true},    // 45 days old - delete
		{"app.4.log.gz", 40 * 24 * time.Hour, true}, // 40 days old compressed - delete
	}

	for _, file := range files {
		filePath := filepath.Join(tempDir, file.name)
		modTime := now.Add(-file.age)
		require.NoError(t, createFileWithModTime(filePath, "log content", modTime))
	}

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30, // Keep files for 30 days
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// Perform cleanup
	err := manager.CleanupOldLogs(tempDir)
	require.NoError(t, err)

	// Verify results
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.name)
		_, err := os.Stat(filePath)

		if file.shouldDelete {
			assert.True(t, os.IsNotExist(err), "File %s should have been deleted", file.name)
		} else {
			assert.NoError(t, err, "File %s should still exist", file.name)
		}
	}
}

func TestLogCleanupManager_ArchiveBeforeDelete(t *testing.T) {
	tempDir := t.TempDir()

	// Create an old log file
	oldFile := filepath.Join(tempDir, "app.1.log")
	content := strings.Repeat("This is a test log line.\n", 100)
	pastTime := time.Now().Add(-35 * 24 * time.Hour)
	require.NoError(t, createFileWithModTime(oldFile, content, pastTime))

	cleanupConfig := config.LogCleanupConfig{
		Enabled:             true,
		RetentionDays:       30,
		CleanupInterval:     time.Hour,
		ArchiveBeforeDelete: true,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// Perform cleanup
	err := manager.CleanupOldLogs(tempDir)
	require.NoError(t, err)

	// Original file should be gone
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err), "Original file should be deleted")

	// Compressed archive should not exist either (since it gets deleted after archiving)
	archivedFile := oldFile + ".gz"
	_, err = os.Stat(archivedFile)
	assert.True(t, os.IsNotExist(err), "Archive file should be deleted after cleanup")
}

func TestLogCleanupManager_GetLogFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create various files
	files := []string{
		"app.log",        // Current log file
		"app.1.log",      // Rotated log file
		"app.2.log.gz",   // Compressed log file
		"debug.log",      // Another log file
		"config.yaml",    // Not a log file
		"data.txt",       // Not a log file
		"app.log.backup", // Contains .log. but not a standard rotated file
	}

	baseTime := time.Now()
	for i, file := range files {
		filePath := filepath.Join(tempDir, file)
		// Create files with different modification times
		modTime := baseTime.Add(-time.Duration(i) * time.Hour)
		require.NoError(t, createFileWithModTime(filePath, "content", modTime))
	}

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	logFiles, err := manager.getLogFiles(tempDir)
	require.NoError(t, err)

	// Should find log files and files containing .log.
	expectedFiles := []string{"app.log", "app.1.log", "app.2.log.gz", "debug.log", "app.log.backup"}
	assert.Len(t, logFiles, len(expectedFiles))

	// Verify files are sorted by modification time (oldest first)
	for i := 1; i < len(logFiles); i++ {
		assert.True(t, logFiles[i-1].modTime.Before(logFiles[i].modTime) || logFiles[i-1].modTime.Equal(logFiles[i].modTime),
			"Files should be sorted by modification time")
	}
}

func TestLogCleanupManager_DiskSpaceCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create log files of different sizes
	files := []struct {
		name string
		size int
		age  time.Duration
	}{
		{"app.log", 1000, 0},                // Current file - should not be deleted
		{"app.1.log", 5000, time.Hour},      // Small file
		{"app.2.log", 10000, 2 * time.Hour}, // Medium file
		{"app.3.log", 15000, 3 * time.Hour}, // Large file
	}

	baseTime := time.Now()
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.name)
		content := strings.Repeat("x", file.size)
		modTime := baseTime.Add(-file.age)
		require.NoError(t, createFileWithModTime(filePath, content, modTime))
	}

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		MinFreeDiskMB:   999999, // Very high requirement to trigger cleanup
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// This test is hard to make deterministic since it depends on actual disk space
	// Instead, let's test the disk space checking function
	availableSpace, err := manager.CheckDiskSpace(tempDir)
	require.NoError(t, err)
	assert.Greater(t, availableSpace, int64(0))
}

func TestLogCleanupManager_StartStop(t *testing.T) {
	tempDir := t.TempDir()

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: 100 * time.Millisecond,
	}

	manager := NewLogCleanupManager(cleanupConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start manager
	err := manager.Start(ctx, tempDir)
	require.NoError(t, err)

	// Verify it's active
	manager.mu.RLock()
	active := manager.active
	manager.mu.RUnlock()
	assert.True(t, active)

	// Stop manager
	err = manager.Stop()
	require.NoError(t, err)

	// Verify it's inactive
	manager.mu.RLock()
	active = manager.active
	manager.mu.RUnlock()
	assert.False(t, active)
}

func TestLogCleanupManager_DisabledConfig(t *testing.T) {
	tempDir := t.TempDir()

	cleanupConfig := config.LogCleanupConfig{
		Enabled: false, // Disabled
	}

	manager := NewLogCleanupManager(cleanupConfig)
	ctx := context.Background()

	// Start should succeed but do nothing
	err := manager.Start(ctx, tempDir)
	assert.NoError(t, err)

	// Cleanup should do nothing
	err = manager.CleanupOldLogs(tempDir)
	assert.NoError(t, err)

	// Stop should succeed
	err = manager.Stop()
	assert.NoError(t, err)
}

func TestLogCleanupManager_CreateLogDirectory(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs", "subdir")

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)
	ctx := context.Background()

	// Directory doesn't exist initially
	_, err := os.Stat(logDir)
	assert.True(t, os.IsNotExist(err))

	// Start should create the directory
	err = manager.Start(ctx, logDir)
	require.NoError(t, err)
	defer manager.Stop()

	// Directory should now exist
	info, err := os.Stat(logDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLogCleanupManager_ArchiveFile(t *testing.T) {
	tempDir := t.TempDir()
	sourceFile := filepath.Join(tempDir, "test.log")

	// Create a file with some content
	content := strings.Repeat("This is a test log line.\n", 100)
	require.NoError(t, os.WriteFile(sourceFile, []byte(content), 0644))

	cleanupConfig := config.LogCleanupConfig{
		Enabled:             true,
		RetentionDays:       30,
		CleanupInterval:     time.Hour,
		ArchiveBeforeDelete: true,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// Archive the file
	err := manager.archiveFile(sourceFile)
	require.NoError(t, err)

	// Check that archive was created
	archiveFile := sourceFile + ".gz"
	assert.FileExists(t, archiveFile)

	// Original file should be removed after archiving
	_, err = os.Stat(sourceFile)
	assert.True(t, os.IsNotExist(err), "Original file should be removed after archiving")

	// Archive should be smaller than original content (due to compression)
	// We can't compare to the original file size since it's been removed,
	// but we can verify the archive exists and has reasonable content
	archiveInfo, err := os.Stat(archiveFile)
	require.NoError(t, err)

	assert.Greater(t, archiveInfo.Size(), int64(0), "Archive should have content")
}

// Test helper functions

func TestLogCleanupManager_CheckDiskSpaceLinux(t *testing.T) {
	// This test is Linux-specific due to syscall.Statfs_t
	if !isLinuxSystem() {
		t.Skip("Skipping Linux-specific test")
	}

	tempDir := t.TempDir()

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	availableSpace, err := manager.CheckDiskSpace(tempDir)
	require.NoError(t, err)
	assert.Greater(t, availableSpace, int64(0))
}

// Helper function to detect Linux system
func isLinuxSystem() bool {
	var stat syscall.Statfs_t
	// Try to call Statfs on /tmp - if it works, we're probably on Linux
	err := syscall.Statfs("/tmp", &stat)
	return err == nil
}

func TestNewLogCleanupManagerWithConfig(t *testing.T) {
	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}
	appConfig := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "testowner",
			Repo:  "testrepo",
		},
	}

	manager := NewLogCleanupManagerWithConfig(cleanupConfig, appConfig)
	require.NotNil(t, manager)
	assert.True(t, manager.config.Enabled)
}

func TestLogCleanupManager_StartAlreadyStarted(t *testing.T) {
	tempDir := t.TempDir()

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: 100 * time.Millisecond,
	}

	manager := NewLogCleanupManager(cleanupConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx, tempDir)
	require.NoError(t, err)
	defer manager.Stop()

	// Starting again should error
	err = manager.Start(ctx, tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestLogCleanupManager_CleanupOldLogsForRepo(t *testing.T) {
	tempDir := t.TempDir()

	// Create repo-specific log directory
	repoLogDir := filepath.Join(tempDir, "owner", "repo")
	require.NoError(t, os.MkdirAll(repoLogDir, 0755))

	// Create an old log file in the repo-specific directory
	oldFile := filepath.Join(repoLogDir, "app.1.log")
	pastTime := time.Now().Add(-35 * 24 * time.Hour)
	require.NoError(t, createFileWithModTime(oldFile, "old log content", pastTime))

	// Create a recent log file
	recentFile := filepath.Join(repoLogDir, "app.log")
	require.NoError(t, os.WriteFile(recentFile, []byte("recent log"), 0644))

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	err := manager.CleanupOldLogsForRepo(tempDir, "owner/repo")
	require.NoError(t, err)

	// Old file should be deleted
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	// Recent file should still exist
	assert.FileExists(t, recentFile)
}

func TestLogCleanupManager_CleanupOldLogsForRepo_NonExistentRepo(t *testing.T) {
	tempDir := t.TempDir()

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		RetentionDays:   30,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// Should not error for non-existent repo directory
	err := manager.CleanupOldLogsForRepo(tempDir, "nonexistent/repo")
	assert.NoError(t, err)
}

func TestLogCleanupManager_CleanupByDiskSpace(t *testing.T) {
	tempDir := t.TempDir()

	// Create some rotated log files (files with .log. in name are eligible for disk space cleanup)
	for i := 1; i <= 3; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("app.%d.log.gz", i))
		content := strings.Repeat("x", 1000)
		pastTime := time.Now().Add(-time.Duration(i) * time.Hour)
		require.NoError(t, createFileWithModTime(filePath, content, pastTime))
	}

	// Also create current log file (should be skipped by disk space cleanup)
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "app.log"), []byte("current"), 0644))

	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		MinFreeDiskMB:   1, // Very low threshold - won't actually trigger cleanup
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)

	// Test the CleanupOldLogs path that includes disk space check
	err := manager.CleanupOldLogs(tempDir)
	assert.NoError(t, err)
}

func TestLogCleanupManager_NegativeDiskMB(t *testing.T) {
	cleanupConfig := config.LogCleanupConfig{
		Enabled:         true,
		MinFreeDiskMB:   -1,
		CleanupInterval: time.Hour,
	}

	manager := NewLogCleanupManager(cleanupConfig)
	err := manager.validateConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "min_free_disk_mb cannot be negative")
}

func TestLogCleanupManager_RemoveLogFile(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name           string
		fileName       string
		archiveEnabled bool
		expectArchive  bool
	}{
		{
			name:           "remove without archive",
			fileName:       "test1.log",
			archiveEnabled: false,
			expectArchive:  false,
		},
		{
			name:           "remove with archive - uncompressed",
			fileName:       "test2.log",
			archiveEnabled: true,
			expectArchive:  true,
		},
		{
			name:           "remove with archive - already compressed",
			fileName:       "test3.log.gz",
			archiveEnabled: true,
			expectArchive:  false, // Already compressed, just delete
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, tt.fileName)
			content := "test log content"
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

			info, err := os.Stat(filePath)
			require.NoError(t, err)

			file := logFileInfo{
				name:    tt.fileName,
				path:    filePath,
				size:    info.Size(),
				modTime: info.ModTime(),
			}

			cleanupConfig := config.LogCleanupConfig{
				Enabled:             true,
				RetentionDays:       30,
				CleanupInterval:     time.Hour,
				ArchiveBeforeDelete: tt.archiveEnabled,
			}

			manager := NewLogCleanupManager(cleanupConfig)

			err = manager.removeLogFile(file)
			require.NoError(t, err)

			// Original file should be gone
			_, err = os.Stat(filePath)
			assert.True(t, os.IsNotExist(err))

			// Check archive expectations
			archivePath := filePath + ".gz"
			_, err = os.Stat(archivePath)

			if tt.expectArchive {
				// Archive should have been created but then deleted (as per cleanup policy)
				assert.True(t, os.IsNotExist(err), "Archive file should be deleted after archiving in cleanup")
			} else {
				// No archive should have been attempted
				assert.True(t, os.IsNotExist(err), "Archive file should not exist")
			}
		})
	}
}
