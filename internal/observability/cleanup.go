package observability

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"compress/gzip"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
)

// LogCleanupManager manages log cleanup based on retention policies and disk space
type LogCleanupManager struct {
	config config.LogCleanupConfig
	ticker *time.Ticker
	done   chan struct{}
	mu     sync.RWMutex
	active bool
}

// NewLogCleanupManager creates a new log cleanup manager with the given configuration
func NewLogCleanupManager(cleanupConfig config.LogCleanupConfig) *LogCleanupManager {
	return &LogCleanupManager{
		config: cleanupConfig,
		done:   make(chan struct{}),
	}
}

// NewLogCleanupManagerWithConfig creates a new log cleanup manager with app config for repo-specific paths
func NewLogCleanupManagerWithConfig(cleanupConfig config.LogCleanupConfig, appConfig *config.Config) *LogCleanupManager {
	return &LogCleanupManager{
		config: cleanupConfig,
		done:   make(chan struct{}),
	}
}

// Start begins log cleanup monitoring for the specified log directory
func (m *LogCleanupManager) Start(ctx context.Context, logDir string) error {
	if !m.config.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active {
		return fmt.Errorf("cleanup manager already started")
	}

	// Validate configuration
	if err := m.validateConfig(); err != nil {
		return fmt.Errorf("invalid cleanup config: %w", err)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log directory %s: %w", logDir, err)
	}

	m.ticker = time.NewTicker(m.config.CleanupInterval)
	m.active = true

	go m.cleanupLoop(ctx, logDir)

	return nil
}

// Stop gracefully stops the log cleanup manager
func (m *LogCleanupManager) Stop() error {
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

// CleanupOldLogs performs immediate log cleanup based on retention policies
func (m *LogCleanupManager) CleanupOldLogs(logDir string) error {
	return m.CleanupOldLogsForRepo(logDir, "")
}

// CleanupOldLogsForRepo performs immediate log cleanup for a specific repository
// If repoPath is empty, cleans up the entire logDir (backward compatibility)
// If repoPath is provided, only cleans up logs for that specific owner/repo
func (m *LogCleanupManager) CleanupOldLogsForRepo(logDir, repoPath string) error {
	if !m.config.Enabled {
		return nil
	}

	// Determine actual log directory to clean up
	var targetLogDir string
	if repoPath != "" {
		targetLogDir = filepath.Join(logDir, repoPath)
		// Check if repo-specific directory exists
		if _, err := os.Stat(targetLogDir); os.IsNotExist(err) {
			return nil // No logs for this repo, nothing to clean up
		}
	} else {
		targetLogDir = logDir
	}

	// Get all log files in directory
	logFiles, err := m.getLogFiles(targetLogDir)
	if err != nil {
		return fmt.Errorf("getting log files: %w", err)
	}

	// Check disk space and cleanup if needed
	if m.config.MinFreeDiskMB > 0 {
		if err := m.cleanupByDiskSpace(targetLogDir, logFiles); err != nil {
			return fmt.Errorf("disk space cleanup: %w", err)
		}
	}

	// Cleanup old files based on retention policy
	if m.config.RetentionDays > 0 {
		if err := m.cleanupByAge(logFiles); err != nil {
			return fmt.Errorf("age-based cleanup: %w", err)
		}
	}

	return nil
}

// CheckDiskSpace returns available disk space in bytes for the given directory
func (m *LogCleanupManager) CheckDiskSpace(logDir string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(logDir, &stat); err != nil {
		return 0, fmt.Errorf("getting disk usage: %w", err)
	}

	// Available space = block size * available blocks
	availableBytes := int64(stat.Bavail) * int64(stat.Bsize)
	return availableBytes, nil
}

// cleanupLoop runs the periodic cleanup process
func (m *LogCleanupManager) cleanupLoop(ctx context.Context, logDir string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case <-m.ticker.C:
			if err := m.CleanupOldLogs(logDir); err != nil {
				// Cleanup failures should not crash the system
				// TODO: Consider adding metrics/alerts for cleanup failures
				continue
			}
		}
	}
}

// getLogFiles returns all log files in the directory, sorted by modification time (oldest first)
func (m *LogCleanupManager) getLogFiles(logDir string) ([]logFileInfo, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("reading log directory: %w", err)
	}

	var logFiles []logFileInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Consider files with .log extension or containing .log. (rotated logs)
		if !strings.HasSuffix(name, ".log") && !strings.Contains(name, ".log.") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		logFiles = append(logFiles, logFileInfo{
			name:    name,
			path:    filepath.Join(logDir, name),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].modTime.Before(logFiles[j].modTime)
	})

	return logFiles, nil
}

// cleanupByAge removes log files older than the retention period
func (m *LogCleanupManager) cleanupByAge(logFiles []logFileInfo) error {
	cutoffTime := time.Now().Add(-time.Duration(m.config.RetentionDays) * 24 * time.Hour)

	for _, file := range logFiles {
		if file.modTime.Before(cutoffTime) {
			if err := m.removeLogFile(file); err != nil {
				return fmt.Errorf("removing old log file %s: %w", file.name, err)
			}
		}
	}

	return nil
}

// cleanupByDiskSpace removes oldest log files until minimum free disk space is achieved
func (m *LogCleanupManager) cleanupByDiskSpace(logDir string, logFiles []logFileInfo) error {
	availableBytes, err := m.CheckDiskSpace(logDir)
	if err != nil {
		return fmt.Errorf("checking disk space: %w", err)
	}

	minFreeBytes := m.config.MinFreeDiskMB * 1024 * 1024
	if availableBytes >= minFreeBytes {
		return nil // Sufficient disk space
	}

	// Calculate how much space we need to free
	spaceNeeded := minFreeBytes - availableBytes

	// Remove oldest files until we have enough space
	var spaceFreed int64
	for _, file := range logFiles {
		if spaceFreed >= spaceNeeded {
			break
		}

		// Skip the current active log file (assume it's the newest without rotation number)
		if !strings.Contains(file.name, ".log.") && !strings.HasSuffix(file.name, ".gz") {
			continue
		}

		spaceFreed += file.size
		if err := m.removeLogFile(file); err != nil {
			return fmt.Errorf("removing log file for disk space %s: %w", file.name, err)
		}
	}

	return nil
}

// removeLogFile removes a log file, optionally archiving it first
func (m *LogCleanupManager) removeLogFile(file logFileInfo) error {
	// Archive before deletion if configured and file is not already compressed
	if m.config.ArchiveBeforeDelete && !strings.HasSuffix(file.name, ".gz") {
		if err := m.archiveFile(file.path); err != nil {
			return fmt.Errorf("archiving file before deletion: %w", err)
		}
		// The original file is removed by archiveFile, so we're done
		// The archival method creates .gz and removes original
		// Now delete the archived version as well
		archivePath := file.path + ".gz"
		if err := os.Remove(archivePath); err != nil {
			return fmt.Errorf("removing archived file: %w", err)
		}
	} else {
		// Just remove the file directly
		if err := os.Remove(file.path); err != nil {
			return fmt.Errorf("removing file: %w", err)
		}
	}

	return nil
}

// archiveFile compresses a file using gzip
func (m *LogCleanupManager) archiveFile(filePath string) error {
	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file for archival: %w", err)
	}
	defer srcFile.Close()

	archivePath := filePath + ".gz"
	dstFile, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("creating archive file: %w", err)
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	// Copy file content through gzip compressor
	buffer := make([]byte, 32*1024) // 32KB buffer for efficient copying
	for {
		n, err := srcFile.Read(buffer)
		if n > 0 {
			if _, writeErr := gzWriter.Write(buffer[:n]); writeErr != nil {
				os.Remove(archivePath) // Clean up partial archive
				return fmt.Errorf("writing to archive: %w", writeErr)
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			os.Remove(archivePath) // Clean up partial archive
			return fmt.Errorf("reading source file: %w", err)
		}
	}

	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("finalizing archive: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("syncing archive file: %w", err)
	}

	// Remove original file after successful archival
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("removing original file after archival: %w", err)
	}

	return nil
}

// validateConfig validates the cleanup configuration
func (m *LogCleanupManager) validateConfig() error {
	if m.config.RetentionDays < 0 {
		return fmt.Errorf("retention_days cannot be negative")
	}

	if m.config.MinFreeDiskMB < 0 {
		return fmt.Errorf("min_free_disk_mb cannot be negative")
	}

	if m.config.CleanupInterval <= 0 {
		return fmt.Errorf("cleanup_interval must be greater than 0")
	}

	// At least one cleanup policy must be enabled
	if m.config.RetentionDays == 0 && m.config.MinFreeDiskMB == 0 {
		return fmt.Errorf("at least one cleanup policy (retention_days or min_free_disk_mb) must be configured")
	}

	return nil
}

// logFileInfo holds information about a log file
type logFileInfo struct {
	name    string
	path    string
	size    int64
	modTime time.Time
}
