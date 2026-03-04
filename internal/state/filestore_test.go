package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	ws := &AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 42,
		IssueTitle:  "Test Issue",
		State:       StateImplement,
		BranchName:  "agent/issue-42",
		UpdatedAt:   now,
		CreatedAt:   now,
	}

	err = store.Save(ctx, ws)
	require.NoError(t, err)

	loaded, err := store.Load(ctx, "developer")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, "developer", loaded.AgentType)
	assert.Equal(t, 42, loaded.IssueNumber)
	assert.Equal(t, "Test Issue", loaded.IssueTitle)
	assert.Equal(t, StateImplement, loaded.State)
	assert.Equal(t, "agent/issue-42", loaded.BranchName)
}

func TestFileStore_LoadNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	loaded, err := store.Load(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()

	ws := &AgentWorkState{
		AgentType: "developer",
		State:     StateIdle,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
	}

	err = store.Save(ctx, ws)
	require.NoError(t, err)

	err = store.Delete(ctx, "developer")
	require.NoError(t, err)

	loaded, err := store.Load(ctx, "developer")
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestFileStore_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	err = store.Delete(context.Background(), "nonexistent")
	require.NoError(t, err)
}

func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()

	states := []*AgentWorkState{
		{AgentType: "developer", State: StateImplement, UpdatedAt: now, CreatedAt: now},
		{AgentType: "qa", State: StateIdle, UpdatedAt: now, CreatedAt: now},
	}

	for _, s := range states {
		require.NoError(t, store.Save(ctx, s))
	}

	listed, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
}

func TestFileStore_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	listed, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestFileStore_Overwrite(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()

	ws := &AgentWorkState{
		AgentType: "developer",
		State:     StateIdle,
		UpdatedAt: now,
		CreatedAt: now,
	}

	require.NoError(t, store.Save(ctx, ws))

	ws.State = StateImplement
	ws.IssueNumber = 10
	require.NoError(t, store.Save(ctx, ws))

	loaded, err := store.Load(ctx, "developer")
	require.NoError(t, err)
	assert.Equal(t, StateImplement, loaded.State)
	assert.Equal(t, 10, loaded.IssueNumber)
}

func TestNewFileStore_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "state")

	store, err := NewFileStore(dir)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Verify directory was created
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewFileStore_InvalidPath(t *testing.T) {
	// Use /dev/null as a non-directory path (on macOS/Linux)
	store, err := NewFileStore("/dev/null/state")
	assert.Error(t, err)
	assert.Nil(t, store)
}

func TestFileStore_SaveAllFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	ws := &AgentWorkState{
		AgentType:          "developer",
		IssueNumber:        42,
		IssueTitle:         "Test Issue",
		State:              StateImplement,
		BranchName:         "agent/issue-42",
		WorkspaceDir:       "/tmp/workspace",
		PRNumber:           100,
		ParentIssue:        10,
		ChildIssues:        []int{11, 12, 13},
		Error:              "test error",
		UpdatedAt:          now,
		CreatedAt:          now,
		CheckpointStage:    "editing",
		InterruptedBy:      "shutdown",
		ImplementationHash: "abc123",
		CheckpointMetadata: map[string]interface{}{
			"key": "value",
		},
		WorkspaceSnapshot: &WorkspaceSnapshot{
			ID:                 "snap-1",
			Timestamp:          now,
			Stage:              "implement",
			FileCount:          5,
			ImplementationHash: "def456",
		},
	}

	require.NoError(t, store.Save(ctx, ws))

	loaded, err := store.Load(ctx, "developer")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, ws.AgentType, loaded.AgentType)
	assert.Equal(t, ws.IssueNumber, loaded.IssueNumber)
	assert.Equal(t, ws.IssueTitle, loaded.IssueTitle)
	assert.Equal(t, ws.State, loaded.State)
	assert.Equal(t, ws.BranchName, loaded.BranchName)
	assert.Equal(t, ws.WorkspaceDir, loaded.WorkspaceDir)
	assert.Equal(t, ws.PRNumber, loaded.PRNumber)
	assert.Equal(t, ws.ParentIssue, loaded.ParentIssue)
	assert.Equal(t, ws.ChildIssues, loaded.ChildIssues)
	assert.Equal(t, ws.Error, loaded.Error)
	assert.Equal(t, ws.CheckpointStage, loaded.CheckpointStage)
	assert.Equal(t, ws.InterruptedBy, loaded.InterruptedBy)
	assert.Equal(t, ws.ImplementationHash, loaded.ImplementationHash)
	require.NotNil(t, loaded.WorkspaceSnapshot)
	assert.Equal(t, "snap-1", loaded.WorkspaceSnapshot.ID)
	assert.Equal(t, 5, loaded.WorkspaceSnapshot.FileCount)
}

func TestFileStore_ListSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()

	// Save a valid state
	ws := &AgentWorkState{
		AgentType: "developer",
		State:     StateIdle,
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Save(ctx, ws))

	// Create a subdirectory in the state dir (should be skipped)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

	// Create a non-JSON file (should be skipped)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644))

	listed, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
	assert.Equal(t, "developer", listed[0].AgentType)
}

func TestFileStore_ListSkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()

	// Save a valid state
	ws := &AgentWorkState{
		AgentType: "developer",
		State:     StateIdle,
		UpdatedAt: time.Now(),
	}
	require.NoError(t, store.Save(ctx, ws))

	// Write an invalid JSON file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0o644))

	listed, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
	assert.Equal(t, "developer", listed[0].AgentType)
}

func TestFileStore_LoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	// Write invalid JSON directly
	require.NoError(t, os.WriteFile(filepath.Join(dir, "developer.json"), []byte("{invalid"), 0o644))

	loaded, err := store.Load(context.Background(), "developer")
	assert.Error(t, err)
	assert.Nil(t, loaded)
	assert.Contains(t, err.Error(), "unmarshaling state")
}

func TestFileStore_Path(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	expected := filepath.Join(dir, "developer.json")
	assert.Equal(t, expected, store.path("developer"))
}

func TestFileStore_MultipleAgents(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()

	agents := []string{"developer", "qa", "reviewer"}
	for _, agent := range agents {
		ws := &AgentWorkState{
			AgentType: agent,
			State:     StateIdle,
			UpdatedAt: now,
			CreatedAt: now,
		}
		require.NoError(t, store.Save(ctx, ws))
	}

	// Load each individually
	for _, agent := range agents {
		loaded, err := store.Load(ctx, agent)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, agent, loaded.AgentType)
	}

	// List all
	listed, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 3)

	// Delete one and verify
	require.NoError(t, store.Delete(ctx, "qa"))
	listed, err = store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
}
