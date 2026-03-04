package gitops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"time"
)

// initBareRepo creates a bare repository to act as a remote for push/pull tests.
func initBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, true)
	require.NoError(t, err)
	return dir
}

// initTestRepo creates a test repository with an initial commit using go-git.
func initTestRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	// Create an initial file and commit so HEAD exists
	testFile := filepath.Join(dir, "README.md")
	err = os.WriteFile(testFile, []byte("# Test Repo\n"), 0o644)
	require.NoError(t, err)

	_, err = wt.Add("README.md")
	require.NoError(t, err)

	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	r := NewRepoFromWorktree(repo, wt, dir, "test-token")
	return r, dir
}

// initTestRepoWithRemote creates a test repo cloned from a bare remote.
func initTestRepoWithRemote(t *testing.T) (*Repo, string, string) {
	t.Helper()

	// Create bare remote
	bareDir := initBareRepo(t)

	// Create a working repo, add a commit, and push to bare
	workDir := t.TempDir()
	repo, err := git.PlainInit(workDir, false)
	require.NoError(t, err)

	// Add remote
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	})
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	// Create initial file
	err = os.WriteFile(filepath.Join(workDir, "init.txt"), []byte("init"), 0o644)
	require.NoError(t, err)
	_, err = wt.Add("init.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Push to bare
	err = repo.Push(&git.PushOptions{
		RefSpecs: []gitconfig.RefSpec{"+refs/heads/*:refs/heads/*"},
	})
	require.NoError(t, err)

	r := NewRepoFromWorktree(repo, wt, workDir, "")
	return r, workDir, bareDir
}

func TestOpen_Success(t *testing.T) {
	_, dir := initTestRepo(t)

	r, err := Open(dir, "token")
	require.NoError(t, err)
	assert.Equal(t, dir, r.Dir())
}

func TestOpen_NotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir, "token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "opening repo")
}

func TestOpen_NonExistentDir(t *testing.T) {
	_, err := Open("/nonexistent/path/12345", "token")
	assert.Error(t, err)
}

func TestDir(t *testing.T) {
	r, dir := initTestRepo(t)
	assert.Equal(t, dir, r.Dir())
}

func TestCheckoutBranch_CreateNew(t *testing.T) {
	r, _ := initTestRepo(t)

	err := r.CheckoutBranch("feature-branch", true)
	require.NoError(t, err)

	// Verify we're on the new branch by checking HEAD
	head, err := r.repo.Head()
	require.NoError(t, err)
	assert.Contains(t, head.Name().String(), "feature-branch")
}

func TestCheckoutBranch_SwitchBack(t *testing.T) {
	r, _ := initTestRepo(t)

	// Create a branch
	err := r.CheckoutBranch("feature", true)
	require.NoError(t, err)

	// Switch back to master
	err = r.CheckoutBranch("master", false)
	require.NoError(t, err)

	head, err := r.repo.Head()
	require.NoError(t, err)
	assert.Contains(t, head.Name().String(), "master")
}

func TestCheckoutBranch_NonExistentNonCreate(t *testing.T) {
	r, _ := initTestRepo(t)
	err := r.CheckoutBranch("does-not-exist", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checking out branch")
}

func TestPull_AlreadyUpToDate(t *testing.T) {
	r, _, _ := initTestRepoWithRemote(t)
	// Pull when already up to date should succeed
	err := r.Pull()
	assert.NoError(t, err)
}

func TestPush_Success(t *testing.T) {
	r, workDir, _ := initTestRepoWithRemote(t)

	// Create a new file and commit
	err := os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("new content"), 0o644)
	require.NoError(t, err)
	_, err = r.worktree.Add("new.txt")
	require.NoError(t, err)
	_, err = r.worktree.Commit("Add new file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Push should succeed (no auth needed for local bare repo)
	err = r.Push()
	assert.NoError(t, err)
}

func TestNewRepoFromWorktree(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	r := NewRepoFromWorktree(repo, wt, dir, "my-token")
	assert.Equal(t, dir, r.Dir())
	assert.Equal(t, "my-token", r.authToken)
	assert.Same(t, repo, r.repo)
	assert.Same(t, wt, r.worktree)
}

func TestClone_WithMockFn(t *testing.T) {
	// Save original and restore after test
	origCloneFn := CloneFn
	defer func() { CloneFn = origCloneFn }()

	expectedDir := "/tmp/test-clone"
	CloneFn = func(url, dir, token string) (*Repo, error) {
		assert.Equal(t, "https://github.com/test/repo.git", url)
		assert.Equal(t, expectedDir, dir)
		assert.Equal(t, "my-token", token)
		return &Repo{dir: dir, authToken: token}, nil
	}

	r, err := Clone("https://github.com/test/repo.git", expectedDir, "my-token")
	require.NoError(t, err)
	assert.Equal(t, expectedDir, r.Dir())
}

func TestClone_WithMockFn_Error(t *testing.T) {
	origCloneFn := CloneFn
	defer func() { CloneFn = origCloneFn }()

	CloneFn = func(url, dir, token string) (*Repo, error) {
		return nil, assert.AnError
	}

	r, err := Clone("https://github.com/test/repo.git", "/tmp/dir", "token")
	assert.Error(t, err)
	assert.Nil(t, r)
}

func TestInitForTest(t *testing.T) {
	dir := t.TempDir()
	// InitForTest needs at least one file to commit
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644))

	r, err := InitForTest(dir, "test-token")
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, dir, r.Dir())

	// Should have a clean worktree after the initial commit
	hasChanges, err := r.HasChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges)
}

func TestInitForTest_InvalidDir(t *testing.T) {
	_, err := InitForTest("/nonexistent/deeply/nested/path", "token")
	assert.Error(t, err)
}
