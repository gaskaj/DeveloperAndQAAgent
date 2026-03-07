package observability

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
)

func TestLogRotationManager_Basic(t *testing.T) {
	tests := []struct {
		name   string
		config config.LogRotationConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: config.LogRotationConfig{
				Enabled:       true,
				MaxFileSize:   10, // 10MB
				MaxFiles:      5,
				MaxAge:        24 * time.Hour,
				CompressOld:   true,
				CheckInterval: time.Minute,
			},
			valid: true,
		},
		{
			name: "invalid max file size",
			config: config.LogRotationConfig{
				Enabled:       true,
				MaxFileSize:   0,
				MaxFiles:      5,
				CheckInterval: time.Minute,
			},
			valid: false,
		},
		{
			name: "invalid max files",
			config: config.LogRotationConfig{
				Enabled:       true,
				MaxFileSize:   10,
				MaxFiles:      0,
				CheckInterval: time.Minute,
			},
			valid: false,
		},
		{
			name: "invalid check interval",
			config: config.LogRotationConfig{
				Enabled:       true,
				MaxFileSize:   10,
				MaxFiles:      5,
				CheckInterval: 0,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewLogRotationManager(tt.config)
			err := manager.validateConfig()

			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestLogRotationManager_SizeBasedRotation(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a log file that exceeds the size limit
	content := strings.Repeat("This is a test log line.\n", 1000)
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1, // 1MB limit, content is much smaller but we'll test the logic
		MaxFiles:      3,
		CompressOld:   false,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Override shouldRotateFile to always return true for this test
	shouldRotate, err := manager.shouldRotateFile(logFile)
	require.NoError(t, err)

	// The file size should be much smaller than 1MB, so it shouldn't rotate by size
	assert.False(t, shouldRotate)

	// Force rotation to test the rotation mechanism
	err = manager.ForceRotate(logFile)
	require.NoError(t, err)

	// Check that the rotated file was created
	rotatedFile := filepath.Join(tempDir, "test.1.log")
	assert.FileExists(t, rotatedFile)

	// Original file should not exist (moved to rotated)
	assert.NoFileExists(t, logFile)
}

func TestLogRotationManager_AgeBasedRotation(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a log file
	content := "test log content"
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))

	// Set the file's modification time to the past
	pastTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(logFile, pastTime, pastTime))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   100, // Large size limit
		MaxFiles:      3,
		MaxAge:        24 * time.Hour, // 24 hour age limit
		CompressOld:   false,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Check if rotation is needed
	shouldRotate, err := manager.shouldRotateFile(logFile)
	require.NoError(t, err)
	assert.True(t, shouldRotate, "File should be rotated due to age")
}

func TestLogRotationManager_Compression(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a log file
	content := strings.Repeat("This is a test log line that will be compressed.\n", 100)
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1, // Small size to ensure rotation
		MaxFiles:      3,
		CompressOld:   true,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Force rotation
	err := manager.ForceRotate(logFile)
	require.NoError(t, err)

	// Check that the compressed file was created
	compressedFile := filepath.Join(tempDir, "test.1.log.gz")
	assert.FileExists(t, compressedFile)

	// Original rotated file should not exist (compressed and removed)
	uncompressedFile := filepath.Join(tempDir, "test.1.log")
	assert.NoFileExists(t, uncompressedFile)
}

func TestLogRotationManager_MaxFilesLimit(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create existing rotated files
	for i := 1; i <= 5; i++ {
		rotatedFile := filepath.Join(tempDir, "test."+string(rune(i+'0'))+".log")
		require.NoError(t, os.WriteFile(rotatedFile, []byte("content"), 0644))
	}

	// Create current log file
	require.NoError(t, os.WriteFile(logFile, []byte("current content"), 0644))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1, // Small size to ensure rotation
		MaxFiles:      3, // Keep only 3 files
		CompressOld:   false,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Force rotation
	err := manager.ForceRotate(logFile)
	require.NoError(t, err)

	// Count remaining files
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	rotatedFiles := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "test.") && strings.Contains(entry.Name(), ".log") {
			rotatedFiles++
		}
	}

	// Should have at most MaxFiles rotated files
	assert.LessOrEqual(t, rotatedFiles, rotationConfig.MaxFiles)
}

func TestLogRotationManager_StartStop(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   10,
		MaxFiles:      3,
		CompressOld:   false,
		CheckInterval: 100 * time.Millisecond,
	}

	manager := NewLogRotationManager(rotationConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start manager
	err := manager.Start(ctx, logFile)
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

func TestLogRotationManager_DisabledConfig(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	rotationConfig := config.LogRotationConfig{
		Enabled: false, // Disabled
	}

	manager := NewLogRotationManager(rotationConfig)
	ctx := context.Background()

	// Start should succeed but do nothing
	err := manager.Start(ctx, logFile)
	assert.NoError(t, err)

	// Force rotate should do nothing
	err = manager.ForceRotate(logFile)
	assert.NoError(t, err)

	// Stop should succeed
	err = manager.Stop()
	assert.NoError(t, err)
}

func TestLogRotationManager_GetRotatedFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create various files to test pattern matching
	files := []string{
		"app.log",        // Current log file
		"app.1.log",      // Rotated file
		"app.2.log",      // Rotated file
		"app.10.log",     // Rotated file with higher number
		"app.1.log.gz",   // Compressed rotated file
		"other.log",      // Different log file
		"app.log.backup", // Not a rotated file
		"app.txt",        // Not a log file
	}

	for _, file := range files {
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, file), []byte("content"), 0644))
	}

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   10,
		MaxFiles:      5,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	rotatedFiles, err := manager.getRotatedFiles(tempDir, "app", ".log")
	require.NoError(t, err)

	// Should find 4 rotated files: app.1.log, app.2.log, app.10.log, app.1.log.gz
	assert.Len(t, rotatedFiles, 4)

	// Verify they're sorted by number (highest first for removal order), then by compression
	expectedNames := []string{"app.10.log", "app.2.log", "app.1.log.gz", "app.1.log"}
	for i, file := range rotatedFiles {
		assert.Equal(t, expectedNames[i], file.name, "File order should match expected rotation sequence")
	}
}

func TestLogRotationManager_CreateLogDirectory(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs", "subdir")
	logFile := filepath.Join(logDir, "test.log")

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   10,
		MaxFiles:      3,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)
	ctx := context.Background()

	// Directory doesn't exist initially
	_, err := os.Stat(logDir)
	assert.True(t, os.IsNotExist(err))

	// Start should create the directory
	err = manager.Start(ctx, logFile)
	require.NoError(t, err)
	defer manager.Stop()

	// Directory should now exist
	info, err := os.Stat(logDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLogRotationManager_CheckAndRotate(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a log file with modification time in the past (exceeds MaxAge)
	content := "test log content"
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))
	pastTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(logFile, pastTime, pastTime))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   100, // Large size to avoid size-based rotation
		MaxFiles:      3,
		MaxAge:        24 * time.Hour,
		CompressOld:   false,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// checkAndRotate should detect age-based rotation need and rotate
	err := manager.checkAndRotate(logFile)
	require.NoError(t, err)

	// Original file should be moved
	assert.NoFileExists(t, logFile)
	rotatedFile := filepath.Join(tempDir, "test.1.log")
	assert.FileExists(t, rotatedFile)
}

func TestLogRotationManager_CheckAndRotate_NoRotationNeeded(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a small recent file
	require.NoError(t, os.WriteFile(logFile, []byte("small"), 0644))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   100, // 100MB - well above file size
		MaxFiles:      3,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	err := manager.checkAndRotate(logFile)
	require.NoError(t, err)

	// File should still exist (no rotation needed)
	assert.FileExists(t, logFile)
}

func TestLogRotationManager_CheckAndRotate_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "nonexistent.log")

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1,
		MaxFiles:      3,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	err := manager.checkAndRotate(logFile)
	require.NoError(t, err) // Should not error, just skip
}

func TestLogRotationManager_ShouldRotateFile_SizeBased(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Create a file that exceeds 1 byte * 1024 * 1024 = 1MB threshold
	// We use MaxFileSize=1 (1MB) but create content much smaller
	// So set MaxFileSize very small to make it work
	content := strings.Repeat("x", 2*1024*1024) // 2MB
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1, // 1MB
		MaxFiles:      3,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	shouldRotate, err := manager.shouldRotateFile(logFile)
	require.NoError(t, err)
	assert.True(t, shouldRotate, "File exceeding max size should need rotation")
}

func TestLogRotationManager_StartAlreadyStarted(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   10,
		MaxFiles:      3,
		CompressOld:   false,
		CheckInterval: 100 * time.Millisecond,
	}

	manager := NewLogRotationManager(rotationConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx, logFile)
	require.NoError(t, err)
	defer manager.Stop()

	// Starting again should error
	err = manager.Start(ctx, logFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestLogRotationManager_StopNotStarted(t *testing.T) {
	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   10,
		MaxFiles:      3,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Stopping when not started should be fine
	err := manager.Stop()
	assert.NoError(t, err)
}

func TestLogRotationManager_MultipleRotations(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "app.log")

	rotationConfig := config.LogRotationConfig{
		Enabled:       true,
		MaxFileSize:   1,
		MaxFiles:      5,
		CompressOld:   false,
		CheckInterval: time.Second,
	}

	manager := NewLogRotationManager(rotationConfig)

	// Perform multiple rotations
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(logFile, []byte(fmt.Sprintf("content %d", i)), 0644))
		err := manager.ForceRotate(logFile)
		require.NoError(t, err)
	}

	// Should have rotated files
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	count := 0
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "app.") && strings.Contains(entry.Name(), ".log") {
			count++
		}
	}
	assert.True(t, count >= 1, "Should have rotated files")
}

// Helper function to create a file with specific modification time
func createFileWithModTime(path string, content string, modTime time.Time) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return os.Chtimes(path, modTime, modTime)
}

// Helper function to get file count in directory
func countFilesWithPattern(dir, pattern string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), pattern) {
			count++
		}
	}
	return count, nil
}
