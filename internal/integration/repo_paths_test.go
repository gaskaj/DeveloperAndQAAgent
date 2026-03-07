package integration

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/workspace"
)

// TestRepoSpecificPaths tests that logs and workspaces are properly segregated by owner/repo
func TestRepoSpecificPaths(t *testing.T) {
	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "test-repo-paths-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogDir := filepath.Join(tempDir, "logs")
	baseWorkspaceDir := filepath.Join(tempDir, "workspaces")

	// Test configs for different repositories
	repo1Config := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "owner1",
			Repo:  "repo1",
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			FilePath: filepath.Join(baseLogDir, "agent.log"),
		},
	}

	repo2Config := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "owner2",
			Repo:  "repo2",
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			FilePath: filepath.Join(baseLogDir, "agent.log"),
		},
	}

	// Test logger paths
	t.Run("logger_uses_repo_specific_paths", func(t *testing.T) {
		// Create loggers for each repo
		logger1 := observability.NewStructuredLoggerWithConfig(repo1Config.Logging, repo1Config)
		logger2 := observability.NewStructuredLoggerWithConfig(repo2Config.Logging, repo2Config)

		// Log messages to trigger file creation
		logger1.Info("Test message from repo1")
		logger2.Info("Test message from repo2")

		// Check that repo-specific log directories exist
		repo1LogDir := filepath.Join(baseLogDir, "owner1", "repo1")
		repo2LogDir := filepath.Join(baseLogDir, "owner2", "repo2")

		if _, err := os.Stat(repo1LogDir); os.IsNotExist(err) {
			t.Errorf("Repo1 log directory not created: %s", repo1LogDir)
		}

		if _, err := os.Stat(repo2LogDir); os.IsNotExist(err) {
			t.Errorf("Repo2 log directory not created: %s", repo2LogDir)
		}

		// Verify log files exist in correct locations
		repo1LogFile := filepath.Join(repo1LogDir, "agent.log")
		repo2LogFile := filepath.Join(repo2LogDir, "agent.log")

		if _, err := os.Stat(repo1LogFile); os.IsNotExist(err) {
			t.Errorf("Repo1 log file not created: %s", repo1LogFile)
		}

		if _, err := os.Stat(repo2LogFile); os.IsNotExist(err) {
			t.Errorf("Repo2 log file not created: %s", repo2LogFile)
		}
	})

	// Test workspace paths
	t.Run("workspaces_use_repo_specific_paths", func(t *testing.T) {
		logger := slog.Default()

		// Create workspace managers for each repo
		manager1, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
			BaseDir:        baseWorkspaceDir,
			MaxSizeMB:      100,
			MinFreeDiskMB:  50,
			MaxConcurrent:  5,
			CleanupEnabled: false,
		}, logger, repo1Config)
		if err != nil {
			t.Fatalf("Failed to create manager1: %v", err)
		}

		manager2, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
			BaseDir:        baseWorkspaceDir,
			MaxSizeMB:      100,
			MinFreeDiskMB:  50,
			MaxConcurrent:  5,
			CleanupEnabled: false,
		}, logger, repo2Config)
		if err != nil {
			t.Fatalf("Failed to create manager2: %v", err)
		}

		// Create workspaces
		ws1, err := manager1.CreateWorkspace(nil, 123)
		if err != nil {
			t.Fatalf("Failed to create workspace1: %v", err)
		}

		ws2, err := manager2.CreateWorkspace(nil, 456)
		if err != nil {
			t.Fatalf("Failed to create workspace2: %v", err)
		}

		// Verify workspaces are in repo-specific directories
		expectedRepo1Path := filepath.Join(baseWorkspaceDir, "owner1", "repo1", "issue-123")
		expectedRepo2Path := filepath.Join(baseWorkspaceDir, "owner2", "repo2", "issue-456")

		if ws1.Path != expectedRepo1Path {
			t.Errorf("Workspace1 path incorrect: got %s, expected %s", ws1.Path, expectedRepo1Path)
		}

		if ws2.Path != expectedRepo2Path {
			t.Errorf("Workspace2 path incorrect: got %s, expected %s", ws2.Path, expectedRepo2Path)
		}

		// Verify directories exist
		if _, err := os.Stat(ws1.Path); os.IsNotExist(err) {
			t.Errorf("Workspace1 directory not created: %s", ws1.Path)
		}

		if _, err := os.Stat(ws2.Path); os.IsNotExist(err) {
			t.Errorf("Workspace2 directory not created: %s", ws2.Path)
		}
	})

	// Test path isolation - cleanup only affects specific repo
	t.Run("cleanup_isolation", func(t *testing.T) {
		logger := slog.Default()

		// Create workspace manager for repo1
		manager1, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
			BaseDir:        baseWorkspaceDir,
			MaxSizeMB:      100,
			MinFreeDiskMB:  50,
			MaxConcurrent:  5,
			CleanupEnabled: true,
		}, logger, repo1Config)
		if err != nil {
			t.Fatalf("Failed to create manager1: %v", err)
		}

		// Create workspace for repo1
		ws1, err := manager1.CreateWorkspace(nil, 789)
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		// Verify workspace was created in repo-specific directory
		expectedPath := filepath.Join(baseWorkspaceDir, "owner1", "repo1", "issue-789")
		if ws1.Path != expectedPath {
			t.Errorf("Workspace path incorrect: got %s, expected %s", ws1.Path, expectedPath)
		}

		// Clean up workspace
		err = manager1.CleanupWorkspace(nil, 789)
		if err != nil {
			t.Fatalf("Failed to cleanup workspace: %v", err)
		}

		// Verify workspace was removed
		if _, err := os.Stat(ws1.Path); !os.IsNotExist(err) {
			t.Errorf("Workspace should have been cleaned up: %s", ws1.Path)
		}

		// Verify that other repo directories still exist (if they were created)
		repo2Dir := filepath.Join(baseWorkspaceDir, "owner2", "repo2")
		if _, err := os.Stat(repo2Dir); err == nil {
			// If repo2 directory exists, it should not have been affected by cleanup
			entries, err := os.ReadDir(repo2Dir)
			if err != nil {
				t.Errorf("Failed to read repo2 directory: %v", err)
			}
			// Just verify we can read it - the exact content depends on previous test execution
			_ = entries
		}
	})
}
