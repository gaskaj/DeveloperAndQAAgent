package config

import (
	"testing"
)

func TestConfig_GetRepoPath(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "valid owner and repo",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "gaskaj",
					Repo:  "OpenAgentFramework",
				},
			},
			expected: "gaskaj/OpenAgentFramework",
		},
		{
			name: "empty owner",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "",
					Repo:  "OpenAgentFramework",
				},
			},
			expected: "",
		},
		{
			name: "empty repo",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "gaskaj",
					Repo:  "",
				},
			},
			expected: "",
		},
		{
			name: "both empty",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "",
					Repo:  "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetRepoPath()
			if result != tt.expected {
				t.Errorf("GetRepoPath() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestConfig_GetLogPath(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		baseLogDir string
		expected   string
	}{
		{
			name: "with repo path",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "gaskaj",
					Repo:  "OpenAgentFramework",
				},
			},
			baseLogDir: "./logs",
			expected:   "logs/gaskaj/OpenAgentFramework",
		},
		{
			name: "without repo path",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "",
					Repo:  "OpenAgentFramework",
				},
			},
			baseLogDir: "./logs",
			expected:   "./logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetLogPath(tt.baseLogDir)
			if result != tt.expected {
				t.Errorf("GetLogPath() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestConfig_GetWorkspacePath(t *testing.T) {
	tests := []struct {
		name             string
		config           Config
		baseWorkspaceDir string
		expected         string
	}{
		{
			name: "with repo path",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "gaskaj",
					Repo:  "OpenAgentFramework",
				},
			},
			baseWorkspaceDir: "./workspaces",
			expected:         "workspaces/gaskaj/OpenAgentFramework",
		},
		{
			name: "without repo path",
			config: Config{
				GitHub: GitHubConfig{
					Owner: "gaskaj",
					Repo:  "",
				},
			},
			baseWorkspaceDir: "./workspaces",
			expected:         "./workspaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetWorkspacePath(tt.baseWorkspaceDir)
			if result != tt.expected {
				t.Errorf("GetWorkspacePath() = %q, expected %q", result, tt.expected)
			}
		})
	}
}
