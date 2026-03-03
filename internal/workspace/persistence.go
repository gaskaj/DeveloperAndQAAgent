package workspace

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)



// FileState represents the state of a single file in the workspace.
type FileState struct {
	Path         string    `json:"path"`
	Hash         string    `json:"hash"`
	Size         int64     `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	IsModified   bool      `json:"is_modified"`
}

// GitSnapshot represents the git state of the workspace.
type GitSnapshot struct {
	Branch      string `json:"branch"`
	CommitHash  string `json:"commit_hash"`
	HasChanges  bool   `json:"has_changes"`
	StagedFiles []string `json:"staged_files"`
}

// ProgressMarker represents a checkpoint in the implementation process.
type ProgressMarker struct {
	Stage       string                 `json:"stage"`
	Timestamp   time.Time              `json:"timestamp"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ConversationSnapshot represents preserved conversation context.
type ConversationSnapshot struct {
	MessageCount      int       `json:"message_count"`
	LastInteraction   time.Time `json:"last_interaction"`
	ContextSummary    string    `json:"context_summary"`
	CompressedHistory string    `json:"compressed_history,omitempty"`
}

// WorkspaceSnapshot represents a complete snapshot of workspace state.
type WorkspaceSnapshot struct {
	ID              string               `json:"id"`
	IssueNumber     int                  `json:"issue_number"`
	Timestamp       time.Time            `json:"timestamp"`
	FileStates      map[string]FileState `json:"file_states"`
	GitState        GitSnapshot          `json:"git_state"`
	ProgressMarkers []ProgressMarker     `json:"progress_markers"`
	ClaudeContext   ConversationSnapshot `json:"claude_context"`
	AgentState      *state.AgentWorkState `json:"agent_state"`
	ImplementationHash string            `json:"implementation_hash"`
}

// WorkspacePersistence manages persistence and recovery of workspace state.
type WorkspacePersistence struct {
	config        config.PersistenceConfig
	logger        *slog.Logger
	snapshotDir   string
}

// NewWorkspacePersistence creates a new workspace persistence manager.
func NewWorkspacePersistence(persistenceConfig config.PersistenceConfig, baseDir string, logger *slog.Logger) *WorkspacePersistence {
	if logger == nil {
		logger = slog.Default()
	}

	snapshotDir := filepath.Join(baseDir, ".snapshots")
	
	return &WorkspacePersistence{
		config:      persistenceConfig,
		logger:      logger,
		snapshotDir: snapshotDir,
	}
}

// CreateSnapshot creates a snapshot of the current workspace state.
func (wp *WorkspacePersistence) CreateSnapshot(ctx context.Context, workspaceDir string, agentState *state.AgentWorkState) (*WorkspaceSnapshot, error) {
	if !wp.config.Enabled {
		return nil, nil
	}

	wp.logger.Debug("creating workspace snapshot", "workspace", workspaceDir, "issue", agentState.IssueNumber)

	// Ensure snapshot directory exists
	if err := os.MkdirAll(wp.snapshotDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating snapshot directory: %w", err)
	}

	snapshot := &WorkspaceSnapshot{
		ID:          generateSnapshotID(),
		IssueNumber: agentState.IssueNumber,
		Timestamp:   time.Now(),
		AgentState:  agentState,
	}

	// Capture file states
	fileStates, err := wp.captureFileStates(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("capturing file states: %w", err)
	}
	snapshot.FileStates = fileStates

	// Capture git state
	gitState, err := wp.captureGitState(workspaceDir)
	if err != nil {
		wp.logger.Warn("failed to capture git state", "error", err)
		// Continue without git state
		snapshot.GitState = GitSnapshot{}
	} else {
		snapshot.GitState = *gitState
	}

	// Generate implementation hash for change detection
	snapshot.ImplementationHash = wp.generateImplementationHash(snapshot)

	// Save snapshot to disk
	if err := wp.saveSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("saving snapshot: %w", err)
	}

	// Cleanup old snapshots
	wp.cleanupOldSnapshots(agentState.IssueNumber)

	wp.logger.Info("workspace snapshot created", 
		"snapshot_id", snapshot.ID,
		"issue", agentState.IssueNumber,
		"file_count", len(snapshot.FileStates),
	)

	return snapshot, nil
}

// RestoreSnapshot restores workspace state from the latest snapshot.
func (wp *WorkspacePersistence) RestoreSnapshot(ctx context.Context, issueNumber int) (*WorkspaceSnapshot, error) {
	if !wp.config.Enabled || !wp.config.ResumeOnRestart {
		return nil, nil
	}

	wp.logger.Debug("restoring workspace snapshot", "issue", issueNumber)

	// Find latest snapshot for the issue
	snapshot, err := wp.findLatestSnapshot(issueNumber)
	if err != nil {
		return nil, fmt.Errorf("finding latest snapshot: %w", err)
	}
	if snapshot == nil {
		wp.logger.Debug("no snapshot found for issue", "issue", issueNumber)
		return nil, nil
	}

	// Validate snapshot if configured
	if wp.config.ValidateBeforeResume {
		if err := wp.ValidateSnapshot(snapshot); err != nil {
			wp.logger.Warn("snapshot validation failed, skipping restore", "error", err)
			return nil, fmt.Errorf("snapshot validation failed: %w", err)
		}
	}

	wp.logger.Info("workspace snapshot restored",
		"snapshot_id", snapshot.ID,
		"issue", issueNumber,
		"snapshot_age", time.Since(snapshot.Timestamp),
	)

	return snapshot, nil
}

// ValidateSnapshot ensures snapshot integrity before restoration.
func (wp *WorkspacePersistence) ValidateSnapshot(snapshot *WorkspaceSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}

	// Check snapshot age
	maxAge := time.Duration(wp.config.RetentionHours) * time.Hour
	if time.Since(snapshot.Timestamp) > maxAge {
		return fmt.Errorf("snapshot too old: %v > %v", time.Since(snapshot.Timestamp), maxAge)
	}

	// Validate required fields
	if snapshot.ID == "" {
		return fmt.Errorf("snapshot missing ID")
	}
	if snapshot.IssueNumber <= 0 {
		return fmt.Errorf("invalid issue number: %d", snapshot.IssueNumber)
	}
	if snapshot.AgentState == nil {
		return fmt.Errorf("snapshot missing agent state")
	}

	// Validate file states
	if len(snapshot.FileStates) == 0 {
		wp.logger.Warn("snapshot has no file states")
	}

	return nil
}

// GetSnapshots returns all snapshots for a given issue number.
func (wp *WorkspacePersistence) GetSnapshots(issueNumber int) ([]*WorkspaceSnapshot, error) {
	if !wp.config.Enabled {
		return nil, nil
	}

	pattern := filepath.Join(wp.snapshotDir, fmt.Sprintf("issue-%d-*.json", issueNumber))
	if wp.config.CompressSnapshots {
		pattern = filepath.Join(wp.snapshotDir, fmt.Sprintf("issue-%d-*.json.gz", issueNumber))
	}

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("finding snapshot files: %w", err)
	}

	var snapshots []*WorkspaceSnapshot
	for _, file := range files {
		snapshot, err := wp.loadSnapshot(file)
		if err != nil {
			wp.logger.Warn("failed to load snapshot", "file", file, "error", err)
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

// DeleteSnapshot removes a specific snapshot.
func (wp *WorkspacePersistence) DeleteSnapshot(snapshotID string) error {
	pattern := filepath.Join(wp.snapshotDir, fmt.Sprintf("*-%s.json*", snapshotID))
	
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("finding snapshot files: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			wp.logger.Warn("failed to remove snapshot file", "file", file, "error", err)
		}
	}

	return nil
}

// captureFileStates captures the current state of all files in the workspace.
func (wp *WorkspacePersistence) captureFileStates(workspaceDir string) (map[string]FileState, error) {
	fileStates := make(map[string]FileState)

	err := filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files/directories
		if info.IsDir() || filepath.Base(path)[0] == '.' {
			return nil
		}

		// Skip snapshot directory
		if filepath.HasPrefix(path, wp.snapshotDir) {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return err
		}

		// Calculate file hash for change detection
		hash, err := wp.calculateFileHash(path)
		if err != nil {
			wp.logger.Warn("failed to calculate file hash", "path", path, "error", err)
			hash = "" // Continue without hash
		}

		fileStates[relPath] = FileState{
			Path:         relPath,
			Hash:         hash,
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			IsModified:   false, // Will be determined during comparison
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking workspace directory: %w", err)
	}

	return fileStates, nil
}

// captureGitState captures the current git state of the workspace.
func (wp *WorkspacePersistence) captureGitState(workspaceDir string) (*GitSnapshot, error) {
	// This is a simplified version - in a full implementation, you'd use go-git
	gitDir := filepath.Join(workspaceDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository")
	}

	// For now, return empty git state - this would be implemented using go-git
	return &GitSnapshot{
		Branch:      "main", // Would be determined from git
		CommitHash:  "",     // Would be determined from git
		HasChanges:  false,  // Would be determined from git
		StagedFiles: nil,    // Would be determined from git
	}, nil
}

// calculateFileHash calculates SHA256 hash of a file for change detection.
func (wp *WorkspacePersistence) calculateFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// generateImplementationHash generates a hash representing the overall implementation state.
func (wp *WorkspacePersistence) generateImplementationHash(snapshot *WorkspaceSnapshot) string {
	hasher := sha256.New()
	
	// Include file hashes
	for _, fileState := range snapshot.FileStates {
		hasher.Write([]byte(fileState.Path + fileState.Hash))
	}
	
	// Include git state
	hasher.Write([]byte(snapshot.GitState.Branch + snapshot.GitState.CommitHash))
	
	// Include agent state
	hasher.Write([]byte(fmt.Sprintf("%d:%s", snapshot.IssueNumber, snapshot.AgentState.State)))
	
	return hex.EncodeToString(hasher.Sum(nil))[:12] // First 12 chars for brevity
}

// saveSnapshot saves a snapshot to disk with optional compression.
func (wp *WorkspacePersistence) saveSnapshot(snapshot *WorkspaceSnapshot) error {
	filename := fmt.Sprintf("issue-%d-%s-%s.json", 
		snapshot.IssueNumber, 
		snapshot.Timestamp.Format("20060102-150405"), 
		snapshot.ID[:8])
	
	filePath := filepath.Join(wp.snapshotDir, filename)
	
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}

	if wp.config.CompressSnapshots {
		filePath += ".gz"
		return wp.saveCompressed(filePath, data)
	}

	return wp.saveUncompressed(filePath, data)
}

// saveUncompressed saves snapshot data to an uncompressed file.
func (wp *WorkspacePersistence) saveUncompressed(filePath string, data []byte) error {
	return os.WriteFile(filePath, data, 0o644)
}

// saveCompressed saves snapshot data to a gzip-compressed file.
func (wp *WorkspacePersistence) saveCompressed(filePath string, data []byte) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	_, err = gzWriter.Write(data)
	return err
}

// loadSnapshot loads a snapshot from disk, handling compression automatically.
func (wp *WorkspacePersistence) loadSnapshot(filePath string) (*WorkspaceSnapshot, error) {
	var reader io.ReadCloser
	
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if filepath.Ext(filePath) == ".gz" {
		reader, err = gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	} else {
		reader = file
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var snapshot WorkspaceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshaling snapshot: %w", err)
	}

	return &snapshot, nil
}

// findLatestSnapshot finds the most recent snapshot for a given issue.
func (wp *WorkspacePersistence) findLatestSnapshot(issueNumber int) (*WorkspaceSnapshot, error) {
	snapshots, err := wp.GetSnapshots(issueNumber)
	if err != nil {
		return nil, err
	}

	if len(snapshots) == 0 {
		return nil, nil
	}

	// Find the most recent snapshot
	var latest *WorkspaceSnapshot
	for _, snapshot := range snapshots {
		if latest == nil || snapshot.Timestamp.After(latest.Timestamp) {
			latest = snapshot
		}
	}

	return latest, nil
}

// cleanupOldSnapshots removes old snapshots for a given issue to stay within limits.
func (wp *WorkspacePersistence) cleanupOldSnapshots(issueNumber int) {
	snapshots, err := wp.GetSnapshots(issueNumber)
	if err != nil {
		wp.logger.Error("failed to get snapshots for cleanup", "error", err)
		return
	}

	// Remove snapshots beyond max count
	if len(snapshots) > wp.config.MaxSnapshots {
		// Sort by timestamp (oldest first)
		for i := 0; i < len(snapshots)-1; i++ {
			for j := i + 1; j < len(snapshots); j++ {
				if snapshots[i].Timestamp.After(snapshots[j].Timestamp) {
					snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
				}
			}
		}

		// Remove excess snapshots (keep the most recent ones)
		excessCount := len(snapshots) - wp.config.MaxSnapshots
		for i := 0; i < excessCount; i++ {
			if err := wp.DeleteSnapshot(snapshots[i].ID); err != nil {
				wp.logger.Error("failed to delete old snapshot", "id", snapshots[i].ID, "error", err)
			}
		}
	}

	// Remove snapshots older than retention period
	cutoff := time.Now().Add(-time.Duration(wp.config.RetentionHours) * time.Hour)
	for _, snapshot := range snapshots {
		if snapshot.Timestamp.Before(cutoff) {
			if err := wp.DeleteSnapshot(snapshot.ID); err != nil {
				wp.logger.Error("failed to delete expired snapshot", "id", snapshot.ID, "error", err)
			}
		}
	}
}

// generateSnapshotID generates a unique snapshot identifier.
func generateSnapshotID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}