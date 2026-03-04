package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DiskStats represents disk usage statistics.
type DiskStats struct {
	TotalMB      int64   `json:"total_mb"`
	UsedMB       int64   `json:"used_mb"`
	AvailableMB  int64   `json:"available_mb"`
	UsagePercent float64 `json:"usage_percent"`
}

// Monitor handles resource monitoring and disk space checks.
type Monitor struct {
	config ManagerConfig
	logger *slog.Logger
}

// NewMonitor creates a new resource monitor.
func NewMonitor(config ManagerConfig, logger *slog.Logger) *Monitor {
	return &Monitor{
		config: config,
		logger: logger,
	}
}

// CheckDiskSpace verifies that sufficient disk space is available.
func (m *Monitor) CheckDiskSpace(ctx context.Context, requiredMB int64) error {
	stats, err := m.GetDiskStats(m.config.BaseDir)
	if err != nil {
		return fmt.Errorf("getting disk stats: %w", err)
	}

	// Check minimum free disk space requirement
	if stats.AvailableMB < m.config.MinFreeDiskMB {
		return fmt.Errorf("insufficient disk space: %d MB available, %d MB minimum required",
			stats.AvailableMB, m.config.MinFreeDiskMB)
	}

	// Check if there's enough space for the requested operation
	if stats.AvailableMB < requiredMB {
		return fmt.Errorf("insufficient disk space for operation: %d MB available, %d MB required",
			stats.AvailableMB, requiredMB)
	}

	m.logger.Debug("disk space check passed",
		"available_mb", stats.AvailableMB,
		"required_mb", requiredMB,
		"min_free_mb", m.config.MinFreeDiskMB,
	)

	return nil
}

// GetDiskStats returns disk usage statistics for the given path.
func (m *Monitor) GetDiskStats(path string) (*DiskStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("getting disk stats for %s: %w", path, err)
	}

	// Calculate disk usage in MB
	blockSize := int64(stat.Bsize)
	totalBlocks := int64(stat.Blocks)
	freeBlocks := int64(stat.Bavail)
	usedBlocks := totalBlocks - int64(stat.Bfree)

	totalMB := (totalBlocks * blockSize) / (1024 * 1024)
	usedMB := (usedBlocks * blockSize) / (1024 * 1024)
	availableMB := (freeBlocks * blockSize) / (1024 * 1024)

	usagePercent := 0.0
	if totalMB > 0 {
		usagePercent = float64(usedMB) / float64(totalMB) * 100.0
	}

	return &DiskStats{
		TotalMB:      totalMB,
		UsedMB:       usedMB,
		AvailableMB:  availableMB,
		UsagePercent: usagePercent,
	}, nil
}

// MonitorDiskSpace continuously monitors disk space and logs warnings when thresholds are exceeded.
func (m *Monitor) MonitorDiskSpace(ctx context.Context) {
	ticker := time.NewTicker(m.config.DiskCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("disk monitoring stopped")
			return
		case <-ticker.C:
			if err := m.checkAndLogDiskUsage(); err != nil {
				m.logger.Error("disk monitoring check failed", "error", err)
			}
		}
	}
}

// checkAndLogDiskUsage performs a disk usage check and logs warnings if thresholds are exceeded.
func (m *Monitor) checkAndLogDiskUsage() error {
	stats, err := m.GetDiskStats(m.config.BaseDir)
	if err != nil {
		return fmt.Errorf("getting disk stats: %w", err)
	}

	// Log warning if disk usage is high
	if stats.UsagePercent > 90.0 {
		m.logger.Error("critical disk usage",
			"usage_percent", stats.UsagePercent,
			"available_mb", stats.AvailableMB,
			"used_mb", stats.UsedMB,
			"total_mb", stats.TotalMB,
		)
	} else if stats.UsagePercent > 80.0 {
		m.logger.Warn("high disk usage",
			"usage_percent", stats.UsagePercent,
			"available_mb", stats.AvailableMB,
		)
	}

	// Log warning if below minimum free space threshold
	if stats.AvailableMB < m.config.MinFreeDiskMB {
		m.logger.Warn("disk space below minimum threshold",
			"available_mb", stats.AvailableMB,
			"min_free_mb", m.config.MinFreeDiskMB,
		)
	}

	// Periodic info log at debug level
	m.logger.Debug("disk usage check",
		"usage_percent", stats.UsagePercent,
		"available_mb", stats.AvailableMB,
		"used_mb", stats.UsedMB,
	)

	return nil
}

// ValidateWorkspaceSize checks if a workspace exceeds the maximum allowed size.
func (m *Monitor) ValidateWorkspaceSize(ctx context.Context, workspacePath string) error {
	// Calculate workspace size
	var size int64
	err := filepath.Walk(workspacePath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("calculating workspace size: %w", err)
	}

	sizeMB := size / (1024 * 1024)

	if sizeMB > m.config.MaxSizeMB {
		return fmt.Errorf("workspace size exceeds limit: %d MB > %d MB", sizeMB, m.config.MaxSizeMB)
	}

	m.logger.Debug("workspace size validation passed",
		"path", workspacePath,
		"size_mb", sizeMB,
		"max_size_mb", m.config.MaxSizeMB,
	)

	return nil
}

// GetResourceUsage returns current resource usage metrics.
func (m *Monitor) GetResourceUsage(ctx context.Context) (map[string]interface{}, error) {
	stats, err := m.GetDiskStats(m.config.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("getting disk stats: %w", err)
	}

	usage := map[string]interface{}{
		"disk": map[string]interface{}{
			"total_mb":      stats.TotalMB,
			"used_mb":       stats.UsedMB,
			"available_mb":  stats.AvailableMB,
			"usage_percent": stats.UsagePercent,
		},
		"thresholds": map[string]interface{}{
			"min_free_mb": m.config.MinFreeDiskMB,
			"max_size_mb": m.config.MaxSizeMB,
		},
		"timestamp": time.Now(),
	}

	return usage, nil
}
