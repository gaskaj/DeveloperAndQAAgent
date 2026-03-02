package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
github:
  token: "ghp_test123"
  owner: "testowner"
  repo: "testrepo"
  poll_interval: "10s"
  watch_labels:
    - "agent:ready"
claude:
  api_key: "sk-ant-test123"
  model: "claude-sonnet-4-20250514"
  max_tokens: 4096
agents:
  developer:
    enabled: true
    max_concurrent: 2
    workspace_dir: "/tmp/workspaces"
state:
  backend: "file"
  dir: "/tmp/state"
logging:
  level: "debug"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadWithOptions(cfgPath, true) // Skip network validation in tests
	require.NoError(t, err)

	assert.Equal(t, "ghp_test123", cfg.GitHub.Token)
	assert.Equal(t, "testowner", cfg.GitHub.Owner)
	assert.Equal(t, "testrepo", cfg.GitHub.Repo)
	assert.Equal(t, 10*time.Second, cfg.GitHub.PollInterval)
	assert.Equal(t, []string{"agent:ready"}, cfg.GitHub.WatchLabels)

	assert.Equal(t, "sk-ant-test123", cfg.Claude.APIKey)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Claude.Model)
	assert.Equal(t, 4096, cfg.Claude.MaxTokens)

	assert.True(t, cfg.Agents.Developer.Enabled)
	assert.Equal(t, 2, cfg.Agents.Developer.MaxConcurrent)
	assert.Equal(t, "/tmp/workspaces", cfg.Agents.Developer.WorkspaceDir)

	assert.Equal(t, "file", cfg.State.Backend)
	assert.Equal(t, "/tmp/state", cfg.State.Dir)
	assert.Equal(t, "debug", cfg.Logging.Level)
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_GH_TOKEN", "ghp_fromenv")
	t.Setenv("TEST_CLAUDE_KEY", "sk-ant-fromenv")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
github:
  token: "${TEST_GH_TOKEN}"
  owner: "testowner"
  repo: "testrepo"
claude:
  api_key: "${TEST_CLAUDE_KEY}"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadWithOptions(cfgPath, true) // Skip network validation in tests
	require.NoError(t, err)

	assert.Equal(t, "ghp_fromenv", cfg.GitHub.Token)
	assert.Equal(t, "sk-ant-fromenv", cfg.Claude.APIKey)
}

func TestLoad_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
github:
  owner: "testowner"
  repo: "testrepo"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	_, err := LoadWithOptions(cfgPath, true) // Skip network validation in tests
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.token")
	assert.Contains(t, err.Error(), "GITHUB_TOKEN")
	assert.Contains(t, err.Error(), "claude.api_key")
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
github:
  token: "ghp_test"
  owner: "testowner"
  repo: "testrepo"
claude:
  api_key: "sk-ant-test"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadWithOptions(cfgPath, true) // Skip network validation in tests
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Claude.Model)
	assert.Equal(t, 8192, cfg.Claude.MaxTokens)
	assert.Equal(t, "file", cfg.State.Backend)
	assert.Equal(t, ".agentctl/state", cfg.State.Dir)
	assert.Equal(t, 1, cfg.Agents.Developer.MaxConcurrent)
	assert.Equal(t, "./workspaces", cfg.Agents.Developer.WorkspaceDir)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	require.Error(t, err)
}
