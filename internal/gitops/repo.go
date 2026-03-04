package gitops

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Repo wraps a go-git repository for common operations.
type Repo struct {
	repo      *git.Repository
	worktree  *git.Worktree
	dir       string
	authToken string
}

// CloneFn is the function used to clone repositories.
// Tests can override this to provide mock clone behaviour.
var CloneFn = defaultClone

// Clone clones a repository to the given directory.
func Clone(url, dir, token string) (*Repo, error) {
	return CloneFn(url, dir, token)
}

func defaultClone(url, dir, token string) (*Repo, error) {
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL: url,
		Auth: &http.BasicAuth{
			Username: "x-access-token",
			Password: token,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cloning repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	return &Repo{repo: repo, worktree: wt, dir: dir, authToken: token}, nil
}

// Open opens an existing repository at the given directory.
func Open(dir, token string) (*Repo, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return nil, fmt.Errorf("opening repo at %s: %w", dir, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	return &Repo{repo: repo, worktree: wt, dir: dir, authToken: token}, nil
}

// CheckoutBranch checks out the given branch, creating it if needed.
func (r *Repo) CheckoutBranch(name string, create bool) error {
	opts := &git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: create,
	}
	if err := r.worktree.Checkout(opts); err != nil {
		return fmt.Errorf("checking out branch %s: %w", name, err)
	}
	return nil
}

// Pull pulls the latest changes from the remote.
func (r *Repo) Pull() error {
	err := r.worktree.Pull(&git.PullOptions{
		Auth: &http.BasicAuth{
			Username: "x-access-token",
			Password: r.authToken,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("pulling: %w", err)
	}
	return nil
}

// Push pushes the current branch to the remote.
func (r *Repo) Push() error {
	err := r.repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: "x-access-token",
			Password: r.authToken,
		},
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec("+refs/heads/*:refs/heads/*"),
		},
	})
	if err != nil {
		return fmt.Errorf("pushing: %w", err)
	}
	return nil
}

// Dir returns the working directory path.
func (r *Repo) Dir() string {
	return r.dir
}

// NewRepoFromWorktree creates a Repo from an existing go-git repository and worktree.
// This is used by tests to construct Repo objects from locally-cloned repositories.
func NewRepoFromWorktree(repo *git.Repository, wt *git.Worktree, dir, token string) *Repo {
	return &Repo{repo: repo, worktree: wt, dir: dir, authToken: token}
}

// InitForTest initializes a new git repository at dir with an initial commit.
// This is intended for testing scenarios where a real git repo is needed
// without cloning from a remote.
func InitForTest(dir, token string) (*Repo, error) {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("initializing test repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("getting worktree: %w", err)
	}

	// Stage all existing files
	if _, err := wt.Add("."); err != nil {
		return nil, fmt.Errorf("staging files: %w", err)
	}

	// Create initial commit
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Agent",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating initial commit: %w", err)
	}

	return &Repo{repo: repo, worktree: wt, dir: dir, authToken: token}, nil
}
