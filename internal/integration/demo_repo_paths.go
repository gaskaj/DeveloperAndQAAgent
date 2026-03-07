package integration

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/workspace"
)

// DemonstrateRepoSpecificPaths shows how the new repo-specific path functionality works
func DemonstrateRepoSpecificPaths() error {
	// Create a temporary demo directory
	demoDir, err := os.MkdirTemp("", "demo-repo-paths-")
	if err != nil {
		return fmt.Errorf("failed to create demo directory: %w", err)
	}
	defer func() {
		fmt.Printf("Demo files created in: %s\n", demoDir)
		fmt.Println("You can inspect the directory structure after running this demo.")
	}()

	baseLogDir := filepath.Join(demoDir, "logs")
	baseWorkspaceDir := filepath.Join(demoDir, "workspaces")

	// Configuration for Agent 1 working on gaskaj/OpenAgentFramework
	config1 := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "gaskaj",
			Repo:  "OpenAgentFramework",
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			FilePath: filepath.Join(baseLogDir, "agent.log"),
		},
	}

	// Configuration for Agent 2 working on microsoft/vscode
	config2 := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "microsoft",
			Repo:  "vscode",
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			FilePath: filepath.Join(baseLogDir, "agent.log"),
		},
	}

	// Configuration for Agent 3 working on golang/go
	config3 := &config.Config{
		GitHub: config.GitHubConfig{
			Owner: "golang",
			Repo:  "go",
		},
		Logging: config.LoggingConfig{
			Level:    "info",
			FilePath: filepath.Join(baseLogDir, "agent.log"),
		},
	}

	fmt.Println("=== Multi-Repository Agent Demo ===")
	fmt.Printf("Base demo directory: %s\n\n", demoDir)

	// Demonstrate repo-specific paths
	fmt.Println("Repository path examples:")
	fmt.Printf("  Repo 1: %s\n", config1.GetRepoPath())
	fmt.Printf("  Repo 2: %s\n", config2.GetRepoPath())
	fmt.Printf("  Repo 3: %s\n", config3.GetRepoPath())
	fmt.Println()

	fmt.Println("Log paths:")
	fmt.Printf("  Agent 1 logs: %s\n", config1.GetLogPath(baseLogDir))
	fmt.Printf("  Agent 2 logs: %s\n", config2.GetLogPath(baseLogDir))
	fmt.Printf("  Agent 3 logs: %s\n", config3.GetLogPath(baseLogDir))
	fmt.Println()

	fmt.Println("Workspace paths:")
	fmt.Printf("  Agent 1 workspaces: %s\n", config1.GetWorkspacePath(baseWorkspaceDir))
	fmt.Printf("  Agent 2 workspaces: %s\n", config2.GetWorkspacePath(baseWorkspaceDir))
	fmt.Printf("  Agent 3 workspaces: %s\n", config3.GetWorkspacePath(baseWorkspaceDir))
	fmt.Println()

	// Create loggers to demonstrate logging isolation
	fmt.Println("=== Creating Loggers (with repo-specific paths) ===")
	logger1 := observability.NewStructuredLoggerWithConfig(config1.Logging, config1)
	logger2 := observability.NewStructuredLoggerWithConfig(config2.Logging, config2)
	logger3 := observability.NewStructuredLoggerWithConfig(config3.Logging, config3)

	// Log messages from each agent
	logger1.Info("Agent 1 started working on gaskaj/OpenAgentFramework issue #164")
	logger2.Info("Agent 2 started working on microsoft/vscode issue #4567")
	logger3.Info("Agent 3 started working on golang/go issue #8901")

	// Create workspace managers for each repo
	fmt.Println("=== Creating Workspace Managers (with repo-specific paths) ===")

	manager1, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
		BaseDir:        baseWorkspaceDir,
		MaxSizeMB:      1024,
		MinFreeDiskMB:  2048,
		MaxConcurrent:  5,
		CleanupEnabled: true,
	}, slog.Default(), config1)
	if err != nil {
		return fmt.Errorf("failed to create workspace manager 1: %w", err)
	}

	manager2, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
		BaseDir:        baseWorkspaceDir,
		MaxSizeMB:      1024,
		MinFreeDiskMB:  2048,
		MaxConcurrent:  5,
		CleanupEnabled: true,
	}, slog.Default(), config2)
	if err != nil {
		return fmt.Errorf("failed to create workspace manager 2: %w", err)
	}

	manager3, err := workspace.NewManagerWithAppConfig(workspace.ManagerConfig{
		BaseDir:        baseWorkspaceDir,
		MaxSizeMB:      1024,
		MinFreeDiskMB:  2048,
		MaxConcurrent:  5,
		CleanupEnabled: true,
	}, slog.Default(), config3)
	if err != nil {
		return fmt.Errorf("failed to create workspace manager 3: %w", err)
	}

	// Create workspaces for different issues
	fmt.Println("=== Creating Workspaces ===")

	ws1, err := manager1.CreateWorkspace(nil, 164)
	if err != nil {
		return fmt.Errorf("failed to create workspace 1: %w", err)
	}
	fmt.Printf("Created workspace for gaskaj/OpenAgentFramework#164: %s\n", ws1.Path)

	ws2, err := manager2.CreateWorkspace(nil, 4567)
	if err != nil {
		return fmt.Errorf("failed to create workspace 2: %w", err)
	}
	fmt.Printf("Created workspace for microsoft/vscode#4567: %s\n", ws2.Path)

	ws3, err := manager3.CreateWorkspace(nil, 8901)
	if err != nil {
		return fmt.Errorf("failed to create workspace 3: %w", err)
	}
	fmt.Printf("Created workspace for golang/go#8901: %s\n", ws3.Path)

	// Show the resulting directory structure
	fmt.Println("\n=== Final Directory Structure ===")
	showDirStructure(demoDir, "")

	// Demonstrate isolation - cleanup only affects the specific repo
	fmt.Println("\n=== Demonstrating Isolation ===")
	fmt.Println("Cleaning up workspace for gaskaj/OpenAgentFramework#164...")

	err = manager1.CleanupWorkspace(nil, 164)
	if err != nil {
		return fmt.Errorf("failed to cleanup workspace 1: %w", err)
	}

	fmt.Println("Workspace cleaned up. Other repos' workspaces remain untouched:")
	fmt.Printf("  microsoft/vscode#4567: %s (still exists: %v)\n", ws2.Path, dirExists(ws2.Path))
	fmt.Printf("  golang/go#8901: %s (still exists: %v)\n", ws3.Path, dirExists(ws3.Path))

	return nil
}

// showDirStructure recursively displays the directory structure
func showDirStructure(path string, indent string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Printf("%sError reading %s: %v\n", indent, path, err)
		return
	}

	for i, entry := range entries {
		isLast := i == len(entries)-1
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}

		fmt.Printf("%s%s%s", indent, prefix, entry.Name())

		if entry.IsDir() {
			fmt.Println("/")
			nextIndent := indent
			if isLast {
				nextIndent += "    "
			} else {
				nextIndent += "│   "
			}

			fullPath := filepath.Join(path, entry.Name())
			showDirStructure(fullPath, nextIndent)
		} else {
			fmt.Println()
		}
	}
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && info.IsDir()
}
