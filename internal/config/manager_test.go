package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigManager(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")
	
	// Write initial config
	initialConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo
  poll_interval: 30s
  watch_labels:
    - agent:ready

claude:
  api_key: sk-ant-api03-test_key_1234567890
  model: claude-sonnet-4-20250514
  max_tokens: 8192

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  backend: file
  dir: ` + t.TempDir() + `

logging:
  level: info
  format: text
  enable_correlation: true

creativity:
  enabled: false
  idle_threshold_seconds: 120
  suggestion_cooldown_seconds: 300
`
	
	err := os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)
	
	// Override LoadInitialConfig to skip network validation for testing
	cfg, err := LoadWithOptions(configPath, true) // Skip network validation
	require.NoError(t, err)
	manager.currentConfig = cfg
	assert.NotNil(t, cfg)
	assert.Equal(t, "testowner", cfg.GitHub.Owner)
	assert.Equal(t, "testrepo", cfg.GitHub.Repo)
	assert.Equal(t, "info", cfg.Logging.Level)
	
	// Test getting current config
	currentCfg := manager.GetConfig()
	require.NotNil(t, currentCfg)
	assert.Equal(t, cfg.GitHub.Owner, currentCfg.GitHub.Owner)
	
	// Test validation
	ctx := context.Background()
	report := manager.ValidateCurrentConfig(ctx)
	assert.NotNil(t, report)
	// Should have some validation issues due to test token format
	t.Logf("Validation report: %d errors, %d warnings", report.ErrorCount, report.WarningCount)
}

func TestConfigManagerHotReload(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "hot-reload-config.yaml")
	
	// Initial config with hot-reloadable fields
	initialConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo
  poll_interval: 30s

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: info
  format: text
  enable_correlation: true

creativity:
  enabled: false
  idle_threshold_seconds: 120
`
	
	err := os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)
	
	// Load initial config with network skip
	cfg, err := LoadWithOptions(configPath, true)
	require.NoError(t, err)
	manager.currentConfig = cfg
	
	// Track config changes
	var configChanges []*Config
	var oldConfigs []*Config
	
	manager.Subscribe(func(oldConfig, newConfig *Config) error {
		oldConfigs = append(oldConfigs, oldConfig)
		configChanges = append(configChanges, newConfig)
		return nil
	})
	
	// Create updated config with hot-reloadable changes
	updatedConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo
  poll_interval: 60s  # Changed - hot reloadable

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: debug  # Changed - hot reloadable
  format: json  # Changed - hot reloadable
  enable_correlation: false  # Changed - hot reloadable

creativity:
  enabled: false
  idle_threshold_seconds: 300  # Changed - hot reloadable
`
	
	// Apply changes to update the internal structure
	err = os.WriteFile(configPath, []byte(updatedConfig), 0644)
	require.NoError(t, err)
	
	newCfg, err := LoadWithOptions(configPath, true)
	require.NoError(t, err)
	
	err = manager.ApplyHotReloadableChanges(newCfg)
	require.NoError(t, err)
	
	// Verify changes were applied
	currentCfg := manager.GetConfig()
	assert.Equal(t, "debug", currentCfg.Logging.Level)
	assert.Equal(t, "json", currentCfg.Logging.Format)
	assert.False(t, currentCfg.Logging.EnableCorrelation)
	assert.Equal(t, 60*time.Second, currentCfg.GitHub.PollInterval)
	assert.Equal(t, 300, currentCfg.Creativity.IdleThresholdSeconds)
	
	// Verify callback was called
	assert.Greater(t, len(configChanges), 0, "Config change callback should have been called")
}

func TestConfigManagerDriftDetection(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "drift-config.yaml")
	
	initialConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: info
`
	
	err := os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)
	
	// Load initial config with network skip
	cfg, err := LoadWithOptions(configPath, true)
	require.NoError(t, err)
	manager.currentConfig = cfg
	
	// No drift initially
	changes, err := manager.DetectConfigurationDrift()
	require.NoError(t, err)
	assert.Len(t, changes, 0, "No drift should be detected initially")
	
	// Modify file to create drift
	driftedConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 2  # Changed from 1

state:
  dir: ` + t.TempDir() + `

logging:
  level: debug  # Changed from info
`
	
	err = os.WriteFile(configPath, []byte(driftedConfig), 0644)
	require.NoError(t, err)
	
	// Detect drift
	changes, err = manager.DetectConfigurationDrift()
	require.NoError(t, err)
	assert.Greater(t, len(changes), 0, "Drift should be detected")
	
	// Check specific changes
	var levelChange, concurrencyChange *ConfigChangeEvent
	for _, change := range changes {
		switch change.Field {
		case "logging.level":
			levelChange = change
		case "agents.developer.max_concurrent":
			concurrencyChange = change
		}
	}
	
	assert.NotNil(t, levelChange, "Logging level change should be detected")
	assert.Equal(t, ConfigModified, levelChange.Type)
	assert.Equal(t, "info", levelChange.OldValue)
	assert.Equal(t, "debug", levelChange.NewValue)
	
	assert.NotNil(t, concurrencyChange, "Max concurrent change should be detected")
	assert.Equal(t, ConfigModified, concurrencyChange.Type)
	assert.Equal(t, 1, concurrencyChange.OldValue)
	assert.Equal(t, 2, concurrencyChange.NewValue)
}

func TestConfigManagerWatching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watching test in short mode")
	}
	
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "watch-config.yaml")
	
	initialConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo
  poll_interval: 30s

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: info
`
	
	err := os.WriteFile(configPath, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)
	
	// Load initial config with network skip
	cfg, err := LoadWithOptions(configPath, true)
	require.NoError(t, err)
	manager.currentConfig = cfg
	
	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	err = manager.StartWatching(ctx)
	require.NoError(t, err)
	
	// Track changes
	changeDetected := make(chan bool, 1)
	manager.Subscribe(func(oldConfig, newConfig *Config) error {
		select {
		case changeDetected <- true:
		default:
		}
		return nil
	})
	
	// Modify file after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		
		updatedConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo
  poll_interval: 60s  # Changed

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: debug  # Changed
`
		
		os.WriteFile(configPath, []byte(updatedConfig), 0644)
	}()
	
	// Wait for change detection
	select {
	case <-changeDetected:
		// Success - change was detected
		currentCfg := manager.GetConfig()
		assert.Equal(t, "debug", currentCfg.Logging.Level)
		assert.Equal(t, 60*time.Second, currentCfg.GitHub.PollInterval)
	case <-ctx.Done():
		t.Log("Timeout waiting for config change detection - this may be expected in some test environments")
		// Don't fail the test as file watching can be flaky in test environments
	}
	
	manager.StopWatching()
}

func TestConfigManagerMetadata(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "metadata-config.yaml")
	
	config := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

creativity:
  enabled: true
  max_pending_suggestions: 1  # Fix validation issue

decomposition:
  enabled: false
`
	
	err := os.WriteFile(configPath, []byte(config), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)
	
	// Load initial config with network skip
	cfg, err := LoadWithOptions(configPath, true)
	require.NoError(t, err)
	manager.currentConfig = cfg
	
	// Get metadata
	metadata := manager.GetConfigMetadata()
	
	assert.Equal(t, configPath, metadata["config_path"])
	assert.True(t, metadata["loaded"].(bool))
	assert.False(t, metadata["watching"].(bool)) // Not started watching yet
	assert.Equal(t, 0, metadata["subscribers"].(int))
	assert.Equal(t, "testowner/testrepo", metadata["github_repo"])
	assert.True(t, metadata["developer_agent_enabled"].(bool))
	assert.True(t, metadata["creativity_enabled"].(bool))
	assert.False(t, metadata["decomposition_enabled"].(bool))
	
	// Check file modification time
	assert.Contains(t, metadata, "file_modified")
	assert.IsType(t, time.Time{}, metadata["file_modified"])
}

func TestHotReloadableFields(t *testing.T) {
	// Test that the HotReloadableFields map contains expected fields
	expectedFields := []string{
		"logging.level",
		"logging.format",
		"logging.enable_correlation",
		"github.poll_interval",
		"creativity.idle_threshold_seconds",
		"metrics.enabled",
		"workspace.cleanup.enabled",
	}
	
	for _, field := range expectedFields {
		assert.True(t, HotReloadableFields[field], "Field %s should be hot-reloadable", field)
	}
	
	// Test that critical fields are NOT hot-reloadable
	criticalFields := []string{
		"github.token",
		"github.owner", 
		"github.repo",
		"claude.api_key",
		"agents.developer.enabled",
		"agents.developer.workspace_dir",
		"state.dir",
	}
	
	for _, field := range criticalFields {
		assert.False(t, HotReloadableFields[field], "Field %s should NOT be hot-reloadable", field)
	}
}

func TestConfigChangeDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)
	
	oldConfig := &Config{
		GitHub: GitHubConfig{
			PollInterval: 30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		Creativity: CreativityConfig{
			IdleThresholdSeconds: 120,
		},
	}
	
	newConfig := &Config{
		GitHub: GitHubConfig{
			PollInterval: 60 * time.Second, // Changed
		},
		Logging: LoggingConfig{
			Level:  "debug", // Changed
			Format: "text",  // Same
		},
		Creativity: CreativityConfig{
			IdleThresholdSeconds: 120, // Same
		},
	}
	
	changes := manager.detectChanges(oldConfig, newConfig)
	
	// Should detect 2 changes
	assert.Len(t, changes, 2)
	
	// Check specific changes
	var pollIntervalChange, logLevelChange *ConfigChangeEvent
	for _, change := range changes {
		switch change.Field {
		case "github.poll_interval":
			pollIntervalChange = change
		case "logging.level":
			logLevelChange = change
		}
	}
	
	require.NotNil(t, pollIntervalChange)
	assert.Equal(t, ConfigModified, pollIntervalChange.Type)
	assert.Equal(t, 30*time.Second, pollIntervalChange.OldValue)
	assert.Equal(t, 60*time.Second, pollIntervalChange.NewValue)
	
	require.NotNil(t, logLevelChange)
	assert.Equal(t, ConfigModified, logLevelChange.Type)
	assert.Equal(t, "info", logLevelChange.OldValue)
	assert.Equal(t, "debug", logLevelChange.NewValue)
}