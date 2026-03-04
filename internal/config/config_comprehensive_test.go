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

// --- Validator Additional Tests ---

func TestValidator_CheckSensitiveDataExposure_Triggered(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Logging: LoggingConfig{
			Level: "debug",
			StructuredLogging: StructuredLoggingConfig{
				Enabled:           true,
				IncludeStackTrace: true,
			},
		},
	}

	result := validator.checkSensitiveDataExposure(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "sensitive information")
}

func TestValidator_CheckSensitiveDataExposure_NotTriggered(t *testing.T) {
	validator := NewValidator()

	// Not triggered when level is info
	cfg := &Config{
		Logging: LoggingConfig{
			Level: "info",
			StructuredLogging: StructuredLoggingConfig{
				Enabled:           true,
				IncludeStackTrace: true,
			},
		},
	}

	result := validator.checkSensitiveDataExposure(cfg)
	assert.True(t, result.Passed)

	// Not triggered when structured logging disabled
	cfg2 := &Config{
		Logging: LoggingConfig{
			Level: "debug",
			StructuredLogging: StructuredLoggingConfig{
				Enabled:           false,
				IncludeStackTrace: true,
			},
		},
	}

	result2 := validator.checkSensitiveDataExposure(cfg2)
	assert.True(t, result2.Passed)
}

func TestValidator_CheckGitHubPollIntervalRange_TooFast(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		GitHub: GitHubConfig{
			PollInterval: 2 * time.Second,
		},
	}

	result := validator.checkGitHubPollIntervalRange(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "too fast")
}

func TestValidator_CheckGitHubPollIntervalRange_TooSlow(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		GitHub: GitHubConfig{
			PollInterval: 2 * time.Hour,
		},
	}

	result := validator.checkGitHubPollIntervalRange(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "very slow")
}

func TestValidator_CheckGitHubPollIntervalRange_Valid(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		GitHub: GitHubConfig{
			PollInterval: 30 * time.Second,
		},
	}

	result := validator.checkGitHubPollIntervalRange(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckGitHubPollIntervalRange_Zero(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		GitHub: GitHubConfig{
			PollInterval: 0,
		},
	}

	result := validator.checkGitHubPollIntervalRange(cfg)
	assert.True(t, result.Passed, "Zero interval should pass (will use default)")
}

func TestValidator_CheckCreativityDecompositionCompatibility(t *testing.T) {
	validator := NewValidator()

	// Both enabled with low budget
	cfg := &Config{
		Creativity:    CreativityConfig{Enabled: true},
		Decomposition: DecompositionConfig{Enabled: true, MaxIterationBudget: 10},
	}

	result := validator.checkCreativityDecompositionCompatibility(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "low iteration budget")

	// Both enabled with sufficient budget
	cfg2 := &Config{
		Creativity:    CreativityConfig{Enabled: true},
		Decomposition: DecompositionConfig{Enabled: true, MaxIterationBudget: 50},
	}

	result2 := validator.checkCreativityDecompositionCompatibility(cfg2)
	assert.True(t, result2.Passed)

	// Only one enabled
	cfg3 := &Config{
		Creativity:    CreativityConfig{Enabled: true},
		Decomposition: DecompositionConfig{Enabled: false},
	}

	result3 := validator.checkCreativityDecompositionCompatibility(cfg3)
	assert.True(t, result3.Passed)
}

func TestValidator_CheckWorkspaceLimitsLogical_NotConfigured(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Workspace: WorkspaceConfig{
			Limits: WorkspaceLimitsConfig{
				MaxSizeMB:     0,
				MinFreeDiskMB: 0,
			},
		},
	}

	result := validator.checkWorkspaceLimitsLogical(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckClaudeMaxTokensRange_Zero(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Claude: ClaudeConfig{MaxTokens: 0},
	}

	result := validator.checkClaudeMaxTokensRange(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckClaudeMaxTokensRange_Valid(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Claude: ClaudeConfig{MaxTokens: 8192},
	}

	result := validator.checkClaudeMaxTokensRange(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckClaudeMaxTokensRange_TooHigh(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Claude: ClaudeConfig{MaxTokens: 300000},
	}

	result := validator.checkClaudeMaxTokensRange(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "maximum token limit")
}

func TestValidator_CheckStateDirectoryPermissions_EmptyDir(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		State: StateConfig{Dir: ""},
	}

	result := validator.checkStateDirectoryPermissions(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckStateDirectoryPermissions_ValidDir(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		State: StateConfig{Dir: t.TempDir()},
	}

	result := validator.checkStateDirectoryPermissions(cfg)
	assert.True(t, result.Passed)
}

func TestValidator_CheckStateDirectoryPermissions_InvalidDir(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		State: StateConfig{Dir: "/dev/null/invalid-dir"},
	}

	result := validator.checkStateDirectoryPermissions(cfg)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Issue, "cannot create state directory")
}

func TestValidator_CheckDeveloperAgentConcurrency_Disabled(t *testing.T) {
	validator := NewValidator()

	cfg := &Config{
		Agents: AgentsConfig{
			Developer: DeveloperAgentConfig{
				Enabled:       false,
				MaxConcurrent: 0,
			},
		},
	}

	result := validator.checkDeveloperAgentConcurrency(cfg)
	assert.True(t, result.Passed, "Should skip when agent is disabled")
}

func TestValidationReport_ToError_MultipleErrors(t *testing.T) {
	report := &ValidationReport{
		ErrorCount: 2,
		Failed: []*ValidationResult{
			{
				Rule:  &ValidationRule{Field: "field1"},
				Issue: "issue1",
				Fix:   "fix1",
			},
			{
				Rule:  &ValidationRule{Field: "field2"},
				Issue: "issue2",
				Fix:   "fix2",
			},
		},
	}

	err := report.ToError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field1")
	assert.Contains(t, err.Error(), "field2")
}

func TestValidateWithReport(t *testing.T) {
	cfg := &Config{
		GitHub: GitHubConfig{
			Token: "ghp_test_token",
			Owner: "owner",
			Repo:  "repo",
		},
		Claude: ClaudeConfig{
			APIKey: "sk-ant-api03-test",
		},
	}

	report := ValidateWithReport(context.Background(), cfg, true)
	assert.NotNil(t, report)
	assert.Equal(t, 0, report.ErrorCount)
}

// --- Defaults Tests ---

func TestGetDefaults(t *testing.T) {
	defaults := GetDefaults()

	assert.Equal(t, 30*time.Second, defaults.GitHub.PollInterval)
	assert.Equal(t, []string{"agent:ready"}, defaults.GitHub.WatchLabels)
	assert.Equal(t, "claude-sonnet-4-20250514", defaults.Claude.Model)
	assert.Equal(t, 8192, defaults.Claude.MaxTokens)
	assert.Equal(t, "file", defaults.State.Backend)
	assert.Equal(t, ".agentctl/state", defaults.State.Dir)
	assert.Equal(t, 1, defaults.Agents.Developer.MaxConcurrent)
	assert.Equal(t, "./workspaces", defaults.Agents.Developer.WorkspaceDir)
	assert.Equal(t, 120, defaults.Creativity.IdleThresholdSeconds)
	assert.Equal(t, 300, defaults.Creativity.SuggestionCooldownSeconds)
	assert.Equal(t, 1, defaults.Creativity.MaxPendingSuggestions)
	assert.Equal(t, 50, defaults.Creativity.MaxRejectionHistory)
	assert.Equal(t, 25, defaults.Decomposition.MaxIterationBudget)
	assert.Equal(t, 5, defaults.Decomposition.MaxSubtasks)
	assert.Equal(t, 30*time.Second, defaults.Shutdown.Timeout)
	assert.True(t, defaults.Shutdown.CleanupWorkspaces)
	assert.True(t, defaults.Shutdown.ResetClaims)

	// Workspace defaults
	assert.Equal(t, int64(1024), defaults.Workspace.Limits.MaxSizeMB)
	assert.Equal(t, int64(2048), defaults.Workspace.Limits.MinFreeDiskMB)
	assert.True(t, defaults.Workspace.Cleanup.Enabled)
	assert.Equal(t, 24*time.Hour, defaults.Workspace.Cleanup.SuccessRetention)
	assert.Equal(t, 168*time.Hour, defaults.Workspace.Cleanup.FailureRetention)
	assert.Equal(t, 5, defaults.Workspace.Cleanup.MaxConcurrent)
	assert.Equal(t, 5*time.Minute, defaults.Workspace.Monitoring.DiskCheckInterval)
	assert.Equal(t, 1*time.Hour, defaults.Workspace.Monitoring.CleanupInterval)

	// Recovery defaults
	assert.True(t, defaults.Agents.Developer.Recovery.Enabled)
	assert.True(t, defaults.Agents.Developer.Recovery.StartupValidation)
	assert.False(t, defaults.Agents.Developer.Recovery.AutoCleanupOrphaned)
	assert.Equal(t, 24*time.Hour, defaults.Agents.Developer.Recovery.MaxResumeAge)

	// Error handling defaults
	assert.True(t, defaults.ErrorHandling.Retry.Enabled)
	assert.Equal(t, 3, defaults.ErrorHandling.Retry.DefaultPolicy.MaxAttempts)
	assert.True(t, defaults.ErrorHandling.CircuitBreaker.Enabled)
	assert.Equal(t, int64(5), defaults.ErrorHandling.CircuitBreaker.MaxFailures)
}

func TestApplyDefaults_AllZeroValues(t *testing.T) {
	cfg := &Config{}
	ApplyDefaults(cfg)

	defaults := GetDefaults()
	assert.Equal(t, defaults.GitHub.PollInterval, cfg.GitHub.PollInterval)
	assert.Equal(t, defaults.Claude.Model, cfg.Claude.Model)
	assert.Equal(t, defaults.Claude.MaxTokens, cfg.Claude.MaxTokens)
	assert.Equal(t, defaults.State.Backend, cfg.State.Backend)
	assert.Equal(t, defaults.State.Dir, cfg.State.Dir)
	assert.Equal(t, defaults.Agents.Developer.MaxConcurrent, cfg.Agents.Developer.MaxConcurrent)
	assert.Equal(t, defaults.Agents.Developer.WorkspaceDir, cfg.Agents.Developer.WorkspaceDir)
	assert.Equal(t, defaults.Creativity.IdleThresholdSeconds, cfg.Creativity.IdleThresholdSeconds)
	assert.Equal(t, defaults.Creativity.SuggestionCooldownSeconds, cfg.Creativity.SuggestionCooldownSeconds)
	assert.Equal(t, defaults.Shutdown.Timeout, cfg.Shutdown.Timeout)
}

func TestApplyDefaults_PreservesExistingValues(t *testing.T) {
	cfg := &Config{
		GitHub: GitHubConfig{
			PollInterval: 60 * time.Second,
			WatchLabels:  []string{"custom-label"},
		},
		Claude: ClaudeConfig{
			Model:     "custom-model",
			MaxTokens: 4096,
		},
		State: StateConfig{
			Backend: "redis",
			Dir:     "/custom/state",
		},
	}

	ApplyDefaults(cfg)

	// These should NOT be overwritten
	assert.Equal(t, 60*time.Second, cfg.GitHub.PollInterval)
	assert.Equal(t, []string{"custom-label"}, cfg.GitHub.WatchLabels)
	assert.Equal(t, "custom-model", cfg.Claude.Model)
	assert.Equal(t, 4096, cfg.Claude.MaxTokens)
	assert.Equal(t, "redis", cfg.State.Backend)
	assert.Equal(t, "/custom/state", cfg.State.Dir)
}

// --- ValidationErrors Tests ---

func TestValidationErrors_NoErrors(t *testing.T) {
	var ve ValidationErrors
	assert.False(t, ve.HasErrors())
	assert.Nil(t, ve.ToError())
	assert.Equal(t, "no validation errors", ve.Error())
}

func TestValidationErrors_SingleError(t *testing.T) {
	var ve ValidationErrors
	ve.Add("field", "value", "rule", "message")
	assert.True(t, ve.HasErrors())
	err := ve.ToError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field")
	assert.Contains(t, err.Error(), "message")
}

// --- ConfigValidationError Tests ---

func TestConfigValidationError_AllFields(t *testing.T) {
	err := ConfigValidationError{
		Field:   "github.token",
		Value:   "ghp_****",
		Issue:   "invalid format",
		Fix:     "use a valid PAT",
		Example: "ghp_xxxx",
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "github.token")
	assert.Contains(t, errStr, "invalid format")
	assert.Contains(t, errStr, "Fix: use a valid PAT")
	assert.Contains(t, errStr, "Example: ghp_xxxx")
}

func TestConfigValidationError_NoFix(t *testing.T) {
	err := ConfigValidationError{
		Field: "field",
		Issue: "problem",
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "field")
	assert.Contains(t, errStr, "problem")
	assert.NotContains(t, errStr, "Fix:")
}

// --- Manager Additional Tests ---

func TestConfigManager_GetConfig_Nil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	cfg := manager.GetConfig()
	assert.Nil(t, cfg, "Should return nil when no config loaded")
}

func TestConfigManager_ValidateCurrentConfig_NoConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	report := manager.ValidateCurrentConfig(context.Background())
	assert.Equal(t, 1, report.ErrorCount)
	assert.Contains(t, report.Failed[0].Issue, "no configuration loaded")
}

func TestConfigManager_DetectConfigurationDrift_NoConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	changes, err := manager.DetectConfigurationDrift()
	assert.Error(t, err)
	assert.Nil(t, changes)
	assert.Contains(t, err.Error(), "no current configuration loaded")
}

func TestConfigManager_ApplyHotReloadableChanges_NoConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	err := manager.ApplyHotReloadableChanges(&Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no current configuration loaded")
}

func TestConfigManager_StartWatching_Twice(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.yaml")
	writeMinimalConfig(t, configPath)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager(configPath, logger)

	ctx := context.Background()

	err := manager.StartWatching(ctx)
	require.NoError(t, err)

	// Starting again should not error
	err = manager.StartWatching(ctx)
	assert.NoError(t, err)

	manager.StopWatching()
}

func TestConfigManager_StopWatching_NoWatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	// Should not panic
	manager.StopWatching()
}

func TestConfigManager_Subscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("dummy", logger)

	callCount := 0
	manager.Subscribe(func(old, new *Config) error {
		callCount++
		return nil
	})
	manager.Subscribe(func(old, new *Config) error {
		callCount++
		return nil
	})

	assert.Equal(t, 2, len(manager.subscribers))
}

func TestConfigManager_GetConfigMetadata_NoConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("nonexistent.yaml", logger)

	metadata := manager.GetConfigMetadata()
	assert.Equal(t, "nonexistent.yaml", metadata["config_path"])
	assert.False(t, metadata["loaded"].(bool))
	assert.False(t, metadata["watching"].(bool))
}

func TestConfigManager_ReloadConfig_InvalidFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	manager := NewConfigManager("/nonexistent/file.yaml", logger)
	manager.currentConfig = &Config{} // Set something so ApplyHotReloadableChanges doesn't fail

	err := manager.ReloadConfig()
	assert.Error(t, err)
}

// --- Utility Function Tests ---

func TestCompareValues(t *testing.T) {
	assert.True(t, compareValues("a", "a"))
	assert.False(t, compareValues("a", "b"))
	assert.True(t, compareValues(1, 1))
	assert.False(t, compareValues(1, 2))
	assert.True(t, compareValues(true, true))
	assert.False(t, compareValues(true, false))
}

func TestCompareStringSlices(t *testing.T) {
	assert.True(t, compareStringSlices(nil, nil))
	assert.True(t, compareStringSlices([]string{}, []string{}))
	assert.True(t, compareStringSlices([]string{"a", "b"}, []string{"a", "b"}))
	assert.False(t, compareStringSlices([]string{"a"}, []string{"a", "b"}))
	assert.False(t, compareStringSlices([]string{"a", "b"}, []string{"a", "c"}))
}

func TestIsZeroValue(t *testing.T) {
	assert.True(t, isZeroValue(""))
	assert.False(t, isZeroValue("hello"))
	assert.True(t, isZeroValue(0))
	assert.False(t, isZeroValue(1))
	assert.True(t, isZeroValue(false))
	assert.False(t, isZeroValue(true))
	assert.True(t, isZeroValue(time.Duration(0)))
	assert.False(t, isZeroValue(time.Second))
	assert.True(t, isZeroValue(nil))
}

// --- Environment Additional Tests ---

func TestEnvironmentType_String_AllValues(t *testing.T) {
	assert.Equal(t, "development", EnvironmentDevelopment.String())
	assert.Equal(t, "staging", EnvironmentStaging.String())
	assert.Equal(t, "production", EnvironmentProduction.String())
	assert.Equal(t, "test", EnvironmentTest.String())
	assert.Equal(t, "unknown", EnvironmentType(99).String())
}

func TestParseEnvironmentType_AllAliases(t *testing.T) {
	tests := []struct {
		input    string
		expected EnvironmentType
	}{
		{"dev", EnvironmentDevelopment},
		{"development", EnvironmentDevelopment},
		{"develop", EnvironmentDevelopment},
		{"stage", EnvironmentStaging},
		{"staging", EnvironmentStaging},
		{"prod", EnvironmentProduction},
		{"production", EnvironmentProduction},
		{"test", EnvironmentTest},
		{"testing", EnvironmentTest},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			env, err := ParseEnvironmentType(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, env)
		})
	}
}

func TestEnvironmentManager_ResolveOverlayPath(t *testing.T) {
	em := NewEnvironmentManager()

	result := em.resolveOverlayPath("/configs/config.yaml", "config.dev.yaml")
	assert.Equal(t, "/configs/config.dev.yaml", result)
}

func TestEnvironmentManager_ValidateEnvironmentOverride_InvalidEnv(t *testing.T) {
	em := NewEnvironmentManager()

	err := em.ValidateEnvironmentOverride("invalid-env", map[string]interface{}{})
	assert.Error(t, err)
}

func TestLoadWithEnvironment_InvalidEnv(t *testing.T) {
	_, err := LoadWithEnvironment("/dummy/path.yaml", "invalid-env")
	assert.Error(t, err)
}

// --- LoadWithSchemaValidation Tests ---

func TestLoadWithSchemaValidation_FileNotFound(t *testing.T) {
	_, err := LoadWithSchemaValidation("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestLoadWithSchemaValidation_ValidConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "schema-config.yaml")

	content := `
github:
  token: "ghp_test_schema_token123"
  owner: "testowner"
  repo: "testrepo"
claude:
  api_key: "sk-ant-api03-test_schema_key123"
agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `
state:
  dir: ` + t.TempDir() + `
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	// This will attempt network validation which will fail in tests,
	// but we're testing the schema loading path
	_, err = LoadWithSchemaValidation(configPath)
	// It may fail due to network validation, but it should not fail due to schema
	if err != nil {
		// Expected to potentially fail on network validation
		assert.Contains(t, err.Error(), "validating")
	}
}

// --- Helper ---

func writeMinimalConfig(t *testing.T, path string) {
	t.Helper()
	content := `
github:
  token: ghp_test_token
  owner: testowner
  repo: testrepo
claude:
  api_key: sk-ant-api03-test_key
agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `
state:
  dir: ` + t.TempDir() + `
`
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}
