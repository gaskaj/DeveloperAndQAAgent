package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// WorkspaceState represents the current state of a workspace.
type WorkspaceState string

const (
	WorkspaceStateActive  WorkspaceState = "active"
	WorkspaceStateStale   WorkspaceState = "stale"
	WorkspaceStateFailed  WorkspaceState = "failed"
	WorkspaceStateCleaned WorkspaceState = "cleaned"
)

// Workspace represents a managed workspace for issue processing.
type Workspace struct {
	ID        int            `json:"id"`
	Path      string         `json:"path"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	SizeMB    int64          `json:"size_mb"`
	State     WorkspaceState `json:"state"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WorkspaceStats provides statistics about workspace usage.
type WorkspaceStats struct {
	TotalWorkspaces  int   `json:"total_workspaces"`
	ActiveWorkspaces int   `json:"active_workspaces"`
	StaleWorkspaces  int   `json:"stale_workspaces"`
	FailedWorkspaces int   `json:"failed_workspaces"`
	TotalSizeMB      int64 `json:"total_size_mb"`
	DiskFreeMB       int64 `json:"disk_free_mb"`
	DiskUsedMB       int64 `json:"disk_used_mb"`
}

// Manager defines the interface for workspace management operations.
type Manager interface {
	// CreateWorkspace creates a new workspace for the given issue ID.
	CreateWorkspace(ctx context.Context, issueID int) (*Workspace, error)
	
	// CleanupWorkspace removes a specific workspace.
	CleanupWorkspace(ctx context.Context, issueID int) error
	
	// CleanupStaleWorkspaces removes workspaces older than the specified duration.
	CleanupStaleWorkspaces(ctx context.Context, olderThan time.Duration) error
	
	// GetWorkspaceStats returns current workspace statistics.
	GetWorkspaceStats(ctx context.Context) (*WorkspaceStats, error)
	
	// CheckDiskSpace verifies sufficient disk space is available.
	CheckDiskSpace(ctx context.Context, requiredMB int64) error
	
	// GetWorkspace retrieves workspace information for an issue ID.
	GetWorkspace(ctx context.Context, issueID int) (*Workspace, error)
	
	// UpdateWorkspaceState updates the state of a workspace.
	UpdateWorkspaceState(ctx context.Context, issueID int, state WorkspaceState) error
	
	// ListWorkspaces returns all workspaces matching the given criteria.
	ListWorkspaces(ctx context.Context, state WorkspaceState) ([]*Workspace, error)
}

// ManagerConfig holds configuration for the workspace manager.
type ManagerConfig struct {
	BaseDir              string        `mapstructure:"base_dir"`
	MaxSizeMB            int64         `mapstructure:"max_size_mb"`
	MinFreeDiskMB        int64         `mapstructure:"min_free_disk_mb"`
	MaxConcurrent        int           `mapstructure:"max_concurrent"`
	SuccessRetention     time.Duration `mapstructure:"success_retention"`
	FailureRetention     time.Duration `mapstructure:"failure_retention"`
	DiskCheckInterval    time.Duration `mapstructure:"disk_check_interval"`
	CleanupInterval      time.Duration `mapstructure:"cleanup_interval"`
	CleanupEnabled       bool          `mapstructure:"cleanup_enabled"`
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() ManagerConfig {
	return ManagerConfig{
		BaseDir:              "./workspaces",
		MaxSizeMB:            1024,  // 1GB
		MinFreeDiskMB:        2048,  // 2GB
		MaxConcurrent:        5,
		SuccessRetention:     24 * time.Hour,   // 1 day
		FailureRetention:     168 * time.Hour,  // 1 week
		DiskCheckInterval:    5 * time.Minute,
		CleanupInterval:      1 * time.Hour,
		CleanupEnabled:       true,
	}
}

// managerImpl implements the Manager interface.
type managerImpl struct {
	config   ManagerConfig
	logger   *slog.Logger
	monitor  *Monitor
	workspaces map[int]*Workspace // In-memory cache of workspace info
}

// NewManager creates a new workspace manager with the given configuration.
func NewManager(config ManagerConfig, logger *slog.Logger) (Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure base directory exists
	if err := os.MkdirAll(config.BaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating base directory %s: %w", config.BaseDir, err)
	}

	monitor := NewMonitor(config, logger)

	mgr := &managerImpl{
		config:     config,
		logger:     logger,
		monitor:    monitor,
		workspaces: make(map[int]*Workspace),
	}

	// Initialize workspace cache by scanning existing directories
	if err := mgr.loadExistingWorkspaces(); err != nil {
		logger.Warn("failed to load existing workspaces", "error", err)
	}

	return mgr, nil
}

// CreateWorkspace creates a new workspace for the given issue ID.
func (m *managerImpl) CreateWorkspace(ctx context.Context, issueID int) (*Workspace, error) {
	m.logger.Debug("creating workspace", "issue_id", issueID)

	// Check if workspace already exists
	if existing, exists := m.workspaces[issueID]; exists && existing.State == WorkspaceStateActive {
		m.logger.Debug("workspace already exists", "issue_id", issueID, "path", existing.Path)
		return existing, nil
	}

	// Check disk space before creating
	if err := m.CheckDiskSpace(ctx, m.config.MaxSizeMB); err != nil {
		return nil, fmt.Errorf("insufficient disk space: %w", err)
	}

	// Check concurrent workspace limit
	if err := m.checkConcurrentLimit(); err != nil {
		return nil, fmt.Errorf("concurrent limit exceeded: %w", err)
	}

	// Create workspace directory
	workspacePath := filepath.Join(m.config.BaseDir, fmt.Sprintf("issue-%d", issueID))
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspace directory: %w", err)
	}

	workspace := &Workspace{
		ID:        issueID,
		Path:      workspacePath,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		SizeMB:    0, // Will be updated after clone
		State:     WorkspaceStateActive,
		Metadata:  make(map[string]interface{}),
	}

	// Cache the workspace
	m.workspaces[issueID] = workspace

	m.logger.Info("workspace created", 
		"issue_id", issueID, 
		"path", workspacePath,
		"active_workspaces", m.countActiveWorkspaces(),
	)

	return workspace, nil
}

// CleanupWorkspace removes a specific workspace.
func (m *managerImpl) CleanupWorkspace(ctx context.Context, issueID int) error {
	m.logger.Debug("cleaning up workspace", "issue_id", issueID)

	workspace, exists := m.workspaces[issueID]
	if !exists {
		// Try to find workspace by path if not cached
		workspacePath := filepath.Join(m.config.BaseDir, fmt.Sprintf("issue-%d", issueID))
		if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
			m.logger.Debug("workspace not found", "issue_id", issueID)
			return nil
		}
		workspace = &Workspace{
			ID:   issueID,
			Path: workspacePath,
		}
	}

	// Remove workspace directory
	if err := os.RemoveAll(workspace.Path); err != nil {
		return fmt.Errorf("removing workspace directory %s: %w", workspace.Path, err)
	}

	// Update workspace state and remove from cache
	workspace.State = WorkspaceStateCleaned
	workspace.UpdatedAt = time.Now()
	delete(m.workspaces, issueID)

	m.logger.Info("workspace cleaned up", 
		"issue_id", issueID, 
		"path", workspace.Path,
		"active_workspaces", m.countActiveWorkspaces(),
	)

	return nil
}

// CleanupStaleWorkspaces removes workspaces older than the specified duration.
func (m *managerImpl) CleanupStaleWorkspaces(ctx context.Context, olderThan time.Duration) error {
	if !m.config.CleanupEnabled {
		m.logger.Debug("cleanup disabled, skipping stale workspace cleanup")
		return nil
	}

	m.logger.Debug("cleaning up stale workspaces", "older_than", olderThan)

	cutoffTime := time.Now().Add(-olderThan)
	var cleanedCount int

	for issueID, workspace := range m.workspaces {
		// Skip active workspaces (they should have shorter retention)
		if workspace.State == WorkspaceStateActive {
			continue
		}

		// Check if workspace is old enough for cleanup
		if workspace.UpdatedAt.After(cutoffTime) {
			continue
		}

		if err := m.CleanupWorkspace(ctx, issueID); err != nil {
			m.logger.Error("failed to cleanup stale workspace", 
				"issue_id", issueID, 
				"error", err,
			)
			continue
		}

		cleanedCount++

		// Check context cancellation periodically
		if ctx.Err() != nil {
			m.logger.Info("cleanup cancelled", "cleaned_count", cleanedCount)
			return ctx.Err()
		}
	}

	m.logger.Info("stale workspace cleanup completed", 
		"cleaned_count", cleanedCount,
		"remaining_workspaces", len(m.workspaces),
	)

	return nil
}

// GetWorkspaceStats returns current workspace statistics.
func (m *managerImpl) GetWorkspaceStats(ctx context.Context) (*WorkspaceStats, error) {
	stats := &WorkspaceStats{
		TotalWorkspaces: len(m.workspaces),
	}

	var totalSize int64

	// Count workspaces by state and calculate total size
	for _, ws := range m.workspaces {
		switch ws.State {
		case WorkspaceStateActive:
			stats.ActiveWorkspaces++
		case WorkspaceStateStale:
			stats.StaleWorkspaces++
		case WorkspaceStateFailed:
			stats.FailedWorkspaces++
		}

		// Update workspace size if not set
		if ws.SizeMB == 0 {
			if size, err := m.calculateWorkspaceSize(ws.Path); err == nil {
				ws.SizeMB = size
			}
		}

		totalSize += ws.SizeMB
	}

	stats.TotalSizeMB = totalSize

	// Get disk usage information
	if diskStats, err := m.monitor.GetDiskStats(m.config.BaseDir); err == nil {
		stats.DiskFreeMB = diskStats.AvailableMB
		stats.DiskUsedMB = diskStats.UsedMB
	}

	return stats, nil
}

// CheckDiskSpace verifies sufficient disk space is available.
func (m *managerImpl) CheckDiskSpace(ctx context.Context, requiredMB int64) error {
	return m.monitor.CheckDiskSpace(ctx, requiredMB)
}

// GetWorkspace retrieves workspace information for an issue ID.
func (m *managerImpl) GetWorkspace(ctx context.Context, issueID int) (*Workspace, error) {
	workspace, exists := m.workspaces[issueID]
	if !exists {
		return nil, fmt.Errorf("workspace not found for issue %d", issueID)
	}

	// Update workspace size if needed
	if workspace.SizeMB == 0 {
		if size, err := m.calculateWorkspaceSize(workspace.Path); err == nil {
			workspace.SizeMB = size
		}
	}

	return workspace, nil
}

// UpdateWorkspaceState updates the state of a workspace.
func (m *managerImpl) UpdateWorkspaceState(ctx context.Context, issueID int, state WorkspaceState) error {
	workspace, exists := m.workspaces[issueID]
	if !exists {
		return fmt.Errorf("workspace not found for issue %d", issueID)
	}

	oldState := workspace.State
	workspace.State = state
	workspace.UpdatedAt = time.Now()

	m.logger.Debug("workspace state updated", 
		"issue_id", issueID, 
		"old_state", oldState, 
		"new_state", state,
	)

	return nil
}

// ListWorkspaces returns all workspaces matching the given criteria.
func (m *managerImpl) ListWorkspaces(ctx context.Context, state WorkspaceState) ([]*Workspace, error) {
	var result []*Workspace

	for _, workspace := range m.workspaces {
		if state == "" || workspace.State == state {
			result = append(result, workspace)
		}
	}

	return result, nil
}

// loadExistingWorkspaces scans the base directory for existing workspaces.
func (m *managerImpl) loadExistingWorkspaces() error {
	entries, err := os.ReadDir(m.config.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Base directory doesn't exist yet
		}
		return fmt.Errorf("reading base directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Parse issue ID from directory name (format: issue-123)
		var issueID int
		if n, err := fmt.Sscanf(entry.Name(), "issue-%d", &issueID); n != 1 || err != nil {
			continue
		}

		workspacePath := filepath.Join(m.config.BaseDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		workspace := &Workspace{
			ID:        issueID,
			Path:      workspacePath,
			CreatedAt: info.ModTime(),
			UpdatedAt: info.ModTime(),
			State:     WorkspaceStateStale, // Assume stale until proven active
			Metadata:  make(map[string]interface{}),
		}

		// Calculate workspace size
		if size, err := m.calculateWorkspaceSize(workspacePath); err == nil {
			workspace.SizeMB = size
		}

		m.workspaces[issueID] = workspace
	}

	m.logger.Info("loaded existing workspaces", "count", len(m.workspaces))
	return nil
}

// checkConcurrentLimit verifies the maximum concurrent workspaces limit.
func (m *managerImpl) checkConcurrentLimit() error {
	activeCount := m.countActiveWorkspaces()
	if activeCount >= m.config.MaxConcurrent {
		return fmt.Errorf("concurrent workspace limit exceeded: %d >= %d", activeCount, m.config.MaxConcurrent)
	}
	return nil
}

// countActiveWorkspaces returns the number of active workspaces.
func (m *managerImpl) countActiveWorkspaces() int {
	count := 0
	for _, ws := range m.workspaces {
		if ws.State == WorkspaceStateActive {
			count++
		}
	}
	return count
}

// calculateWorkspaceSize calculates the disk usage of a workspace in MB.
func (m *managerImpl) calculateWorkspaceSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("calculating workspace size: %w", err)
	}

	return size / (1024 * 1024), nil // Convert to MB
}