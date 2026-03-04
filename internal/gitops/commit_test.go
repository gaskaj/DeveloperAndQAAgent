package gitops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFile_NewFile(t *testing.T) {
	r, dir := initTestRepo(t)

	err := r.WriteFile("hello.txt", "Hello, World!")
	require.NoError(t, err)

	// Verify file was created
	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(data))
}

func TestWriteFile_NestedDirectories(t *testing.T) {
	r, dir := initTestRepo(t)

	err := r.WriteFile("a/b/c/deep.txt", "deep content")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "a/b/c/deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep content", string(data))
}

func TestWriteFile_Overwrite(t *testing.T) {
	r, dir := initTestRepo(t)

	err := r.WriteFile("file.txt", "original")
	require.NoError(t, err)

	err = r.WriteFile("file.txt", "updated")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "updated", string(data))
}

func TestReadFile_Existing(t *testing.T) {
	r, _ := initTestRepo(t)

	// README.md was created in initTestRepo
	content, err := r.ReadFile("README.md")
	require.NoError(t, err)
	assert.Equal(t, "# Test Repo\n", content)
}

func TestReadFile_NonExistent(t *testing.T) {
	r, _ := initTestRepo(t)

	_, err := r.ReadFile("nonexistent.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestReadFile_AfterWrite(t *testing.T) {
	r, _ := initTestRepo(t)

	err := r.WriteFile("new.go", "package main\n")
	require.NoError(t, err)

	content, err := r.ReadFile("new.go")
	require.NoError(t, err)
	assert.Equal(t, "package main\n", content)
}

func TestHasChanges_Clean(t *testing.T) {
	r, _ := initTestRepo(t)

	changed, err := r.HasChanges()
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestHasChanges_WithUnstagedChanges(t *testing.T) {
	r, dir := initTestRepo(t)

	// Create a new file (unstaged)
	err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0o644)
	require.NoError(t, err)

	changed, err := r.HasChanges()
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestHasChanges_WithModifiedFile(t *testing.T) {
	r, dir := initTestRepo(t)

	// Modify existing file
	err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0o644)
	require.NoError(t, err)

	changed, err := r.HasChanges()
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestStageAll(t *testing.T) {
	r, dir := initTestRepo(t)

	// Create a new file
	err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("content"), 0o644)
	require.NoError(t, err)

	err = r.StageAll()
	require.NoError(t, err)

	// Verify file is staged by checking worktree status
	status, err := r.worktree.Status()
	require.NoError(t, err)
	fileStatus := status.File("staged.txt")
	assert.Equal(t, 'A', rune(fileStatus.Staging))
}

func TestStageAll_MultipleFiles(t *testing.T) {
	r, dir := initTestRepo(t)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644)
		require.NoError(t, err)
	}

	err := r.StageAll()
	require.NoError(t, err)

	status, err := r.worktree.Status()
	require.NoError(t, err)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		fileStatus := status.File(name)
		assert.Equal(t, 'A', rune(fileStatus.Staging), "file %s should be staged", name)
	}
}

func TestCommit_Basic(t *testing.T) {
	r, dir := initTestRepo(t)

	// Create and stage a new file
	err := os.WriteFile(filepath.Join(dir, "committed.txt"), []byte("commit me"), 0o644)
	require.NoError(t, err)
	_, err = r.worktree.Add("committed.txt")
	require.NoError(t, err)

	err = r.Commit("test: add committed.txt")
	require.NoError(t, err)

	// Verify commit exists
	head, err := r.repo.Head()
	require.NoError(t, err)
	commit, err := r.repo.CommitObject(head.Hash())
	require.NoError(t, err)
	assert.Equal(t, "test: add committed.txt", commit.Message)
	assert.Equal(t, "DeveloperAgent", commit.Author.Name)
	assert.Equal(t, "agent@devqaagent.local", commit.Author.Email)
}

func TestCommit_AfterStageAll(t *testing.T) {
	r, _ := initTestRepo(t)

	err := r.WriteFile("feature.go", "package feature\n")
	require.NoError(t, err)

	err = r.StageAll()
	require.NoError(t, err)

	err = r.Commit("feat: add feature")
	require.NoError(t, err)

	// Should be clean after commit
	changed, err := r.HasChanges()
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestListFiles_Root(t *testing.T) {
	r, _ := initTestRepo(t)

	files, err := r.ListFiles(".")
	require.NoError(t, err)
	assert.Contains(t, files, "README.md")
}

func TestListFiles_WithSubdirectory(t *testing.T) {
	r, dir := initTestRepo(t)

	// Create subdirectory with files
	subDir := filepath.Join(dir, "pkg")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(subDir, "lib.go"), []byte("package pkg"), 0o644)
	require.NoError(t, err)

	files, err := r.ListFiles("pkg")
	require.NoError(t, err)
	assert.Contains(t, files, "lib.go")
}

func TestListFiles_DirectoryEntry(t *testing.T) {
	r, dir := initTestRepo(t)

	// Create a subdirectory
	err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	require.NoError(t, err)
	// Create a file in subdir so the directory shows up
	err = os.WriteFile(filepath.Join(dir, "subdir", "file.txt"), []byte("x"), 0o644)
	require.NoError(t, err)

	files, err := r.ListFiles(".")
	require.NoError(t, err)

	// Directories should have trailing /
	found := false
	for _, f := range files {
		if f == "subdir/" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected 'subdir/' in file listing, got: %v", files)
}

func TestListFiles_NonExistentDir(t *testing.T) {
	r, _ := initTestRepo(t)

	_, err := r.ListFiles("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listing files")
}

func TestListFiles_Empty(t *testing.T) {
	r, dir := initTestRepo(t)

	emptyDir := filepath.Join(dir, "empty")
	err := os.MkdirAll(emptyDir, 0o755)
	require.NoError(t, err)

	files, err := r.ListFiles("empty")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestFullWorkflow_WriteStageCommit(t *testing.T) {
	r, _ := initTestRepo(t)

	// Write a file
	err := r.WriteFile("main.go", "package main\n\nfunc main() {}\n")
	require.NoError(t, err)

	// Verify changes exist
	changed, err := r.HasChanges()
	require.NoError(t, err)
	assert.True(t, changed)

	// Stage all
	err = r.StageAll()
	require.NoError(t, err)

	// Commit
	err = r.Commit("feat: implement main function")
	require.NoError(t, err)

	// Verify clean
	changed, err = r.HasChanges()
	require.NoError(t, err)
	assert.False(t, changed)

	// Read back
	content, err := r.ReadFile("main.go")
	require.NoError(t, err)
	assert.Equal(t, "package main\n\nfunc main() {}\n", content)

	// List files should show main.go
	files, err := r.ListFiles(".")
	require.NoError(t, err)
	assert.Contains(t, files, "main.go")
}
