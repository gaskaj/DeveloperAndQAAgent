package observability

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"compress/gzip"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
)

// LogRotationManager handles log file rotation based on size, count, and age limits
type LogRotationManager struct {
	config config.LogRotationConfig
	ticker *time.Ticker
	done   chan struct{}
	mu     sync.RWMutex
	active bool
}

// NewLogRotationManager creates a new log rotation manager with the given configuration
func NewLogRotationManager(rotationConfig config.LogRotationConfig) *LogRotationManager {
	return &LogRotationManager{
		config: rotationConfig,
		done:   make(chan struct{}),
	}
}

// Start begins log rotation monitoring for the specified log file
func (m *LogRotationManager) Start(ctx context.Context, logFilePath string) error {
	if !m.config.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active {
		return fmt.Errorf("rotation manager already started")
	}

	// Validate configuration
	if err := m.validateConfig(); err != nil {
		return fmt.Errorf("invalid rotation config: %w", err)
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", logDir, err)
	}

	m.ticker = time.NewTicker(m.config.CheckInterval)
	m.active = true

	go m.rotationLoop(ctx, logFilePath)

	return nil
}

// Stop gracefully stops the log rotation manager
func (m *LogRotationManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active {
		return nil
	}

	close(m.done)
	if m.ticker != nil {
		m.ticker.Stop()
	}
	m.active = false

	return nil
}

// ForceRotate performs immediate log rotation regardless of schedule
func (m *LogRotationManager) ForceRotate(logFilePath string) error {
	if !m.config.Enabled {
		return nil
	}

	return m.performRotation(logFilePath)
}

// rotationLoop runs the periodic rotation check
func (m *LogRotationManager) rotationLoop(ctx context.Context, logFilePath string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case <-m.ticker.C:
			if err := m.checkAndRotate(logFilePath); err != nil {
				// Log rotation failures should not crash the system
				// TODO: Consider adding metrics/alerts for rotation failures
				continue
			}
		}
	}
}

// checkAndRotate examines the log file and rotates if necessary
func (m *LogRotationManager) checkAndRotate(logFilePath string) error {
	// Check if rotation is needed
	shouldRotate, err := m.shouldRotateFile(logFilePath)
	if err != nil {
		return fmt.Errorf("checking rotation necessity: %w", err)
	}

	if !shouldRotate {
		return nil
	}

	return m.performRotation(logFilePath)
}

// shouldRotateFile determines if the log file needs rotation
func (m *LogRotationManager) shouldRotateFile(logFilePath string) (bool, error) {
	info, err := os.Stat(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // No file to rotate
		}
		return false, fmt.Errorf("stating log file: %w", err)
	}

	// Check file size (convert MB to bytes)
	maxSizeBytes := m.config.MaxFileSize * 1024 * 1024
	if info.Size() >= maxSizeBytes {
		return true, nil
	}

	// Check file age
	if m.config.MaxAge > 0 {
		if time.Since(info.ModTime()) >= m.config.MaxAge {
			return true, nil
		}
	}

	return false, nil
}

// performRotation executes the actual log rotation
func (m *LogRotationManager) performRotation(logFilePath string) error {
	logDir := filepath.Dir(logFilePath)
	baseName := filepath.Base(logFilePath)
	baseNameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	ext := filepath.Ext(baseName)

	// Get list of existing rotated files
	rotatedFiles, err := m.getRotatedFiles(logDir, baseNameWithoutExt, ext)
	if err != nil {
		return fmt.Errorf("getting rotated files: %w", err)
	}

	// Remove excess files if we're at the limit
	if len(rotatedFiles) >= m.config.MaxFiles {
		filesToRemove := len(rotatedFiles) - m.config.MaxFiles + 1
		for i := 0; i < filesToRemove; i++ {
			if err := os.Remove(filepath.Join(logDir, rotatedFiles[i].name)); err != nil {
				return fmt.Errorf("removing old rotated file %s: %w", rotatedFiles[i].name, err)
			}
		}
		rotatedFiles = rotatedFiles[filesToRemove:]
	}

	// Shift existing files
	if err := m.shiftRotatedFiles(logDir, baseNameWithoutExt, ext, rotatedFiles); err != nil {
		return fmt.Errorf("shifting rotated files: %w", err)
	}

	// Rotate the current log file
	rotatedName := fmt.Sprintf("%s.1%s", baseNameWithoutExt, ext)
	rotatedPath := filepath.Join(logDir, rotatedName)

	if err := m.moveFile(logFilePath, rotatedPath); err != nil {
		return fmt.Errorf("moving log file: %w", err)
	}

	// Compress the rotated file if configured
	if m.config.CompressOld {
		if err := m.compressFile(rotatedPath); err != nil {
			return fmt.Errorf("compressing rotated file: %w", err)
		}
	}

	return nil
}

// getRotatedFiles returns a sorted list of rotated log files
func (m *LogRotationManager) getRotatedFiles(logDir, baseName, ext string) ([]rotatedFileInfo, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("reading log directory: %w", err)
	}

	var rotatedFiles []rotatedFileInfo
	pattern := baseName + "."

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, pattern) {
			continue
		}

		// Extract rotation number
		suffix := strings.TrimPrefix(name, pattern)
		if strings.HasSuffix(suffix, ".gz") {
			suffix = strings.TrimSuffix(suffix, ".gz")
		}
		if strings.HasSuffix(suffix, ext) {
			suffix = strings.TrimSuffix(suffix, ext)
		}

		if rotationNum, err := strconv.Atoi(suffix); err == nil && rotationNum > 0 {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			rotatedFiles = append(rotatedFiles, rotatedFileInfo{
				name:       name,
				number:     rotationNum,
				modTime:    info.ModTime(),
				compressed: strings.HasSuffix(name, ".gz"),
			})
		}
	}

	// Sort by rotation number (highest first), then by compression status for deterministic order
	sort.Slice(rotatedFiles, func(i, j int) bool {
		if rotatedFiles[i].number != rotatedFiles[j].number {
			return rotatedFiles[i].number > rotatedFiles[j].number
		}
		// For same rotation number, sort compressed files first
		return rotatedFiles[i].compressed && !rotatedFiles[j].compressed
	})

	return rotatedFiles, nil
}

// shiftRotatedFiles renumbers existing rotated files to make room for the new one
func (m *LogRotationManager) shiftRotatedFiles(logDir, baseName, ext string, rotatedFiles []rotatedFileInfo) error {
	// Shift files in reverse order to avoid conflicts
	for i := len(rotatedFiles) - 1; i >= 0; i-- {
		file := rotatedFiles[i]
		newNumber := file.number + 1

		var newName string
		if file.compressed {
			newName = fmt.Sprintf("%s.%d%s.gz", baseName, newNumber, ext)
		} else {
			newName = fmt.Sprintf("%s.%d%s", baseName, newNumber, ext)
		}

		oldPath := filepath.Join(logDir, file.name)
		newPath := filepath.Join(logDir, newName)

		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("renaming %s to %s: %w", file.name, newName, err)
		}
	}

	return nil
}

// moveFile safely moves a file using atomic operations when possible
func (m *LogRotationManager) moveFile(src, dst string) error {
	// Try atomic rename first (works if src and dst are on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + delete for cross-filesystem moves
	return m.copyAndRemoveFile(src, dst)
}

// copyAndRemoveFile copies a file and then removes the original
func (m *LogRotationManager) copyAndRemoveFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst) // Clean up partial file
		return fmt.Errorf("copying file content: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("syncing destination file: %w", err)
	}

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("removing source file: %w", err)
	}

	return nil
}

// compressFile compresses a file using gzip and removes the original
func (m *LogRotationManager) compressFile(filePath string) error {
	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for compression: %w", err)
	}
	defer srcFile.Close()

	compressedPath := filePath + ".gz"
	dstFile, err := os.OpenFile(compressedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating compressed file: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		os.Remove(compressedPath) // Clean up partial file
		return fmt.Errorf("compressing file: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("finalizing compressed file: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("syncing compressed file: %w", err)
	}

	// Remove original file after successful compression
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("removing original file after compression: %w", err)
	}

	return nil
}

// validateConfig validates the rotation configuration
func (m *LogRotationManager) validateConfig() error {
	if m.config.MaxFileSize <= 0 {
		return fmt.Errorf("max_file_size_mb must be greater than 0")
	}

	if m.config.MaxFiles <= 0 {
		return fmt.Errorf("max_files must be greater than 0")
	}

	if m.config.CheckInterval <= 0 {
		return fmt.Errorf("check_interval must be greater than 0")
	}

	return nil
}

// rotatedFileInfo holds information about a rotated log file
type rotatedFileInfo struct {
	name       string
	number     int
	modTime    time.Time
	compressed bool
}
