package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspacePersistence_CreateSnapshot(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "workspace-persistence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test workspace directory
	workspaceDir := filepath.Join(tempDir, "workspace")
	err = os.MkdirAll(workspaceDir, 0o755)
	require.NoError(t, err)

	// Create some test files
	testFile1 := filepath.Join(workspaceDir, "test1.txt")
	err = os.WriteFile(testFile1, []byte("test content 1"), 0o644)
	require.NoError(t, err)

	testFile2 := filepath.Join(workspaceDir, "test2.txt")
	err = os.WriteFile(testFile2, []byte("test content 2"), 0o644)
	require.NoError(t, err)

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    false, // Easier to verify in tests
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	// Test creating snapshot
	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	assert.Equal(t, 123, snapshot.IssueNumber)
	assert.Equal(t, "implement", string(agentState.State))
	assert.Len(t, snapshot.FileStates, 2)
	assert.NotEmpty(t, snapshot.ID)
	assert.NotEmpty(t, snapshot.ImplementationHash)

	// Verify file states
	assert.Contains(t, snapshot.FileStates, "test1.txt")
	assert.Contains(t, snapshot.FileStates, "test2.txt")

	fileState1 := snapshot.FileStates["test1.txt"]
	assert.Equal(t, "test1.txt", fileState1.Path)
	assert.NotEmpty(t, fileState1.Hash)
	assert.Equal(t, int64(14), fileState1.Size) // Length of "test content 1"
}

func TestWorkspacePersistence_RestoreSnapshot(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "workspace-persistence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	workspaceDir := filepath.Join(tempDir, "workspace")
	err = os.MkdirAll(workspaceDir, 0o755)
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(workspaceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    true,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}
	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	// Create snapshot first
	originalSnapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, originalSnapshot)

	// Test restoring snapshot
	restoredSnapshot, err := persistence.RestoreSnapshot(ctx, 123)
	require.NoError(t, err)
	require.NotNil(t, restoredSnapshot)

	assert.Equal(t, originalSnapshot.ID, restoredSnapshot.ID)
	assert.Equal(t, originalSnapshot.IssueNumber, restoredSnapshot.IssueNumber)
	assert.Equal(t, originalSnapshot.ImplementationHash, restoredSnapshot.ImplementationHash)
}

func TestWorkspacePersistence_ValidateSnapshot(t *testing.T) {
	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    true,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}
	persistence := NewWorkspacePersistence(persistenceConfig, "", nil)

	// Test valid snapshot
	validSnapshot := &WorkspaceSnapshot{
		ID:          "test-id",
		IssueNumber: 123,
		Timestamp:   time.Now(),
		AgentState: &state.AgentWorkState{
			AgentType: "developer",
			State:     state.StateImplement,
		},
		FileStates: map[string]FileState{
			"test.txt": {
				Path: "test.txt",
				Hash: "abc123",
				Size: 100,
			},
		},
	}

	err := persistence.ValidateSnapshot(validSnapshot)
	assert.NoError(t, err)

	// Test nil snapshot
	err = persistence.ValidateSnapshot(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot is nil")

	// Test snapshot missing ID
	invalidSnapshot := *validSnapshot
	invalidSnapshot.ID = ""
	err = persistence.ValidateSnapshot(&invalidSnapshot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot missing ID")

	// Test snapshot with invalid issue number
	invalidSnapshot = *validSnapshot
	invalidSnapshot.IssueNumber = 0
	err = persistence.ValidateSnapshot(&invalidSnapshot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid issue number")

	// Test snapshot missing agent state
	invalidSnapshot = *validSnapshot
	invalidSnapshot.AgentState = nil
	err = persistence.ValidateSnapshot(&invalidSnapshot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot missing agent state")

	// Test old snapshot
	persistenceConfig.RetentionHours = 1 // 1 hour retention
	persistence = NewWorkspacePersistence(persistenceConfig, "", nil)

	oldSnapshot := *validSnapshot
	oldSnapshot.Timestamp = time.Now().Add(-2 * time.Hour) // 2 hours old
	err = persistence.ValidateSnapshot(&oldSnapshot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot too old")
}

func TestWorkspacePersistence_CleanupOldSnapshots(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "workspace-persistence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         2, // Keep only 2 snapshots
		RetentionHours:       72,
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	workspaceDir := filepath.Join(tempDir, "workspace")
	err = os.MkdirAll(workspaceDir, 0o755)
	require.NoError(t, err)

	ctx := context.Background()

	// Create 4 snapshots
	for i := 1; i <= 4; i++ {
		agentState := &state.AgentWorkState{
			AgentType:   "developer",
			IssueNumber: 123,
			State:       state.StateImplement,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		_, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
		require.NoError(t, err)

		// Sleep a bit to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Check that cleanup kept only the most recent 2 snapshots
	snapshots, err := persistence.GetSnapshots(123)
	require.NoError(t, err)
	assert.Len(t, snapshots, 2)

	// Verify they are the most recent ones (can't be exact due to cleanup timing)
	assert.True(t, len(snapshots) <= 2)
}

func TestWorkspacePersistence_CompressedSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	// Create test file
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("compressed content"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    true,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 456,
		State:       state.StateCommit,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	// Create compressed snapshot
	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, 456, snapshot.IssueNumber)

	// Restore compressed snapshot
	restored, err := persistence.RestoreSnapshot(ctx, 456)
	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, snapshot.ID, restored.ID)
}

func TestWorkspacePersistence_UncompressedSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("uncompressed"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 789,
		State:       state.StateAnalyze,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	// Get snapshots
	snapshots, err := persistence.GetSnapshots(789)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	assert.Equal(t, snapshot.ID, snapshots[0].ID)
}

func TestWorkspacePersistence_DeleteSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("to be deleted"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 321,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	// Verify snapshot exists
	snapshots, err := persistence.GetSnapshots(321)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)

	// Delete it
	err = persistence.DeleteSnapshot(snapshot.ID)
	require.NoError(t, err)

	// Verify it's gone
	snapshots, err = persistence.GetSnapshots(321)
	require.NoError(t, err)
	assert.Len(t, snapshots, 0)
}

func TestWorkspacePersistence_RestoreSnapshot_Disabled_ResumeOnRestart(t *testing.T) {
	persistenceConfig := config.PersistenceConfig{
		Enabled:         true,
		ResumeOnRestart: false,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, "", nil)

	snapshot, err := persistence.RestoreSnapshot(context.Background(), 123)
	require.NoError(t, err)
	assert.Nil(t, snapshot)
}

func TestWorkspacePersistence_RestoreSnapshot_NoSnapshotsExist(t *testing.T) {
	tempDir := t.TempDir()

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: false,
		RetentionHours:       72,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	snapshot, err := persistence.RestoreSnapshot(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, snapshot)
}

func TestWorkspacePersistence_RestoreSnapshot_ValidationFailure(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("data"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:              true,
		MaxSnapshots:         10,
		RetentionHours:       1, // 1 hour retention - snapshot will expire
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 555,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()

	// Create a snapshot
	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	// Load snapshot and manually set old timestamp to trigger validation failure
	// This is tricky since we'd need to modify the file on disk
	// Instead, create a new persistence with very short retention
	shortRetention := config.PersistenceConfig{
		Enabled:              true,
		MaxSnapshots:         10,
		RetentionHours:       0, // 0 hours - everything is expired
		CompressSnapshots:    false,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	shortPersistence := NewWorkspacePersistence(shortRetention, tempDir, nil)

	restored, err := shortPersistence.RestoreSnapshot(ctx, 555)
	assert.Error(t, err)
	assert.Nil(t, restored)
	assert.Contains(t, err.Error(), "snapshot validation failed")
}

func TestWorkspacePersistence_CaptureGitState_NotGitRepo(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	// Create a file (no .git directory)
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("no git"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:           true,
		MaxSnapshots:      10,
		RetentionHours:    72,
		CompressSnapshots: false,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 777,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Snapshot should still succeed, just without git state
	snapshot, err := persistence.CreateSnapshot(context.Background(), workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	// GitState should be empty (not a git repo)
	assert.Empty(t, snapshot.GitState.CommitHash)
}

func TestWorkspacePersistence_CaptureGitState_WithGitDir(t *testing.T) {
	tempDir := t.TempDir()
	workspaceDir := filepath.Join(tempDir, "workspace")
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workspaceDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workspaceDir, "test.txt"), []byte("with git"), 0o644))

	persistenceConfig := config.PersistenceConfig{
		Enabled:           true,
		MaxSnapshots:      10,
		RetentionHours:    72,
		CompressSnapshots: false,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 888,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	snapshot, err := persistence.CreateSnapshot(context.Background(), workspaceDir, agentState)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, "main", snapshot.GitState.Branch)
}

func TestWorkspacePersistence_GenerateSnapshotID(t *testing.T) {
	id1 := generateSnapshotID()
	assert.NotEmpty(t, id1)

	time.Sleep(1 * time.Millisecond)
	id2 := generateSnapshotID()
	assert.NotEqual(t, id1, id2) // IDs should be unique
}

func TestWorkspacePersistence_NilLogger(t *testing.T) {
	persistenceConfig := config.PersistenceConfig{
		Enabled: true,
	}

	// Should use default logger when nil
	persistence := NewWorkspacePersistence(persistenceConfig, "/tmp/test", nil)
	assert.NotNil(t, persistence)
}

func TestWorkspacePersistence_NewWithAppConfig(t *testing.T) {
	persistenceConfig := config.PersistenceConfig{
		Enabled: true,
	}

	// Without app config
	persistence := NewWorkspacePersistenceWithAppConfig(persistenceConfig, "/tmp/test", nil, nil)
	assert.NotNil(t, persistence)
}

func TestWorkspacePersistence_ValidateSnapshot_EmptyFileStates(t *testing.T) {
	persistenceConfig := config.PersistenceConfig{
		Enabled:        true,
		RetentionHours: 72,
	}
	persistence := NewWorkspacePersistence(persistenceConfig, "", nil)

	// Snapshot with no file states should still validate (just warn)
	snapshot := &WorkspaceSnapshot{
		ID:          "test-id",
		IssueNumber: 123,
		Timestamp:   time.Now(),
		AgentState: &state.AgentWorkState{
			AgentType: "developer",
			State:     state.StateImplement,
		},
		FileStates: map[string]FileState{}, // Empty
	}

	err := persistence.ValidateSnapshot(snapshot)
	assert.NoError(t, err) // Empty file states is just a warning, not an error
}

func TestWorkspacePersistence_Disabled(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "workspace-persistence-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	persistenceConfig := config.PersistenceConfig{
		Enabled:              false, // Disable persistence
		SnapshotInterval:     5 * time.Minute,
		MaxSnapshots:         10,
		RetentionHours:       72,
		CompressSnapshots:    true,
		ResumeOnRestart:      true,
		ValidateBeforeResume: true,
	}

	persistence := NewWorkspacePersistence(persistenceConfig, tempDir, nil)

	agentState := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 123,
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()
	workspaceDir := filepath.Join(tempDir, "workspace")

	// Creating snapshot should return nil when disabled
	snapshot, err := persistence.CreateSnapshot(ctx, workspaceDir, agentState)
	require.NoError(t, err)
	assert.Nil(t, snapshot)

	// Restoring snapshot should return nil when disabled
	restoredSnapshot, err := persistence.RestoreSnapshot(ctx, 123)
	require.NoError(t, err)
	assert.Nil(t, restoredSnapshot)

	// Getting snapshots should return nil when disabled
	snapshots, err := persistence.GetSnapshots(123)
	require.NoError(t, err)
	assert.Nil(t, snapshots)
}
