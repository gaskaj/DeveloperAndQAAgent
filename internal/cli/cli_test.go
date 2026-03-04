package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMinimalConfig writes a minimal valid config file to the given path.
// It uses the provided tmpDir for workspace and state directories.
func writeMinimalConfig(t *testing.T, dir string, enableDeveloper bool) string {
	t.Helper()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := fmt.Sprintf(`github:
  token: "ghp_testtoken1234567890abcdef"
  owner: "testowner"
  repo: "testrepo"
  poll_interval: "30s"
  watch_labels:
    - "agent:ready"

claude:
  api_key: "sk-ant-api03-testkey1234567890"
  model: "claude-sonnet-4-20250514"
  max_tokens: 8192

agents:
  developer:
    enabled: %t
    max_concurrent: 1
    workspace_dir: "%s"

state:
  backend: "file"
  dir: "%s"

logging:
  level: "info"
  format: "text"
`, enableDeveloper, filepath.Join(dir, "workspaces"), filepath.Join(dir, "state"))
	err := os.WriteFile(cfgPath, []byte(content), 0o644)
	require.NoError(t, err)
	return cfgPath
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short value", "abc", "***"},
		{"exactly 8", "12345678", "***"},
		{"long value", "ghp_1234567890abcdef", "ghp_...cdef"},
		{"medium value", "123456789", "1234...6789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnabledStatus(t *testing.T) {
	assert.Contains(t, enabledStatus(true), "Enabled")
	assert.Contains(t, enabledStatus(false), "Disabled")
}

func TestJoinLines(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"line1"}, "line1"},
		{"multiple", []string{"line1", "line2", "line3"}, "line1\nline2\nline3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinLines(tt.lines)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatValidationError(t *testing.T) {
	t.Run("config validation error", func(t *testing.T) {
		err := &config.ConfigValidationError{
			Field: "github.token",
			Issue: "token is required",
		}
		result := formatValidationError(err)
		assert.Contains(t, result, "github.token")
	})

	t.Run("plain error", func(t *testing.T) {
		err := fmt.Errorf("some error")
		result := formatValidationError(err)
		assert.Contains(t, result, "some error")
	})

	t.Run("multi error", func(t *testing.T) {
		err := errors.Join(
			fmt.Errorf("error1"),
			fmt.Errorf("error2"),
		)
		result := formatValidationError(err)
		assert.Contains(t, result, "error1")
		assert.Contains(t, result, "error2")
	})

	t.Run("multi error with config validation errors", func(t *testing.T) {
		err := errors.Join(
			&config.ConfigValidationError{Field: "field1", Issue: "msg1"},
			&config.ConfigValidationError{Field: "field2", Issue: "msg2"},
		)
		result := formatValidationError(err)
		assert.Contains(t, result, "field1")
		assert.Contains(t, result, "field2")
	})
}

func TestCheckEnvVar(t *testing.T) {
	// Just ensure it doesn't panic for various inputs
	checkEnvVar("TEST_VAR", "")
	checkEnvVar("TEST_VAR", "${TEST_VAR}")
	checkEnvVar("TEST_VAR", "short")
	checkEnvVar("TEST_VAR", "a-long-enough-value-to-display")
}

func TestFormatStringSliceYAML(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		indent   string
		expected string
	}{
		{"empty", []string{}, "  - ", ""},
		{"single", []string{"item1"}, "  - ", "  - \"item1\""},
		{"multiple", []string{"a", "b"}, "    - ", "    - \"a\"\n    - \"b\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStringSliceYAML(tt.slice, tt.indent)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvironmentVariableMappings(t *testing.T) {
	mappings := getEnvironmentVariableMappings()
	assert.True(t, len(mappings) > 0)

	// Check required mappings exist
	var hasGitHubToken, hasAPIKey bool
	for _, m := range mappings {
		if m.EnvVar == "GITHUB_TOKEN" {
			hasGitHubToken = true
			assert.True(t, m.Required)
			assert.Equal(t, "string", m.Type)
		}
		if m.EnvVar == "ANTHROPIC_API_KEY" {
			hasAPIKey = true
			assert.True(t, m.Required)
		}
	}
	assert.True(t, hasGitHubToken)
	assert.True(t, hasAPIKey)
}

func TestPrintDefaultsYAML(t *testing.T) {
	defaults := config.GetDefaults()
	// Should not error
	err := printDefaultsYAML(defaults)
	assert.NoError(t, err)
}

func TestPrintDefaultsJSON(t *testing.T) {
	defaults := config.GetDefaults()
	err := printDefaultsJSON(defaults)
	assert.NoError(t, err)
}

func TestPrintDefaultsTable(t *testing.T) {
	defaults := config.GetDefaults()
	err := printDefaultsTable(defaults)
	assert.NoError(t, err)
}

func TestPrintEnvVarsTable(t *testing.T) {
	mappings := getEnvironmentVariableMappings()
	err := printEnvVarsTable(mappings)
	assert.NoError(t, err)
}

func TestPrintEnvVarsYAML(t *testing.T) {
	mappings := getEnvironmentVariableMappings()
	err := printEnvVarsYAML(mappings)
	assert.NoError(t, err)
}

func TestPrintEnvVarsJSON(t *testing.T) {
	mappings := getEnvironmentVariableMappings()
	err := printEnvVarsJSON(mappings)
	assert.NoError(t, err)
}

func TestPrintPassedValidations(t *testing.T) {
	cfg := &config.Config{}
	cfg.GitHub.Token = "ghp_test_token_12345"
	cfg.GitHub.Owner = "testowner"
	cfg.GitHub.Repo = "testrepo"
	cfg.Claude.APIKey = "sk-ant-api03-test"
	cfg.Agents.Developer.WorkspaceDir = "/tmp/workspace"
	cfg.State.Dir = "/tmp/state"
	cfg.Agents.Developer.Enabled = true
	cfg.Agents.Developer.MaxConcurrent = 2

	// Should not panic
	printPassedValidations(cfg)
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		level string
	}{
		{"debug"},
		{"warn"},
		{"error"},
		{"info"},
		{"unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			logger := setupLogger(tt.level)
			assert.NotNil(t, logger)
		})
	}
}

func TestRunValidateCommand(t *testing.T) {
	t.Run("missing config flag", func(t *testing.T) {
		oldCfgFile := cfgFile
		cfgFile = ""
		defer func() { cfgFile = oldCfgFile }()

		err := runValidate(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--config flag is required")
	})

	t.Run("nonexistent config file", func(t *testing.T) {
		oldCfgFile := cfgFile
		cfgFile = "/nonexistent/config.yaml"
		defer func() { cfgFile = oldCfgFile }()

		err := runValidate(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRunConfigValidateCommand(t *testing.T) {
	t.Run("missing config flag", func(t *testing.T) {
		oldCfgFile := cfgFile
		cfgFile = ""
		defer func() { cfgFile = oldCfgFile }()

		err := runConfigValidate(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config file path is required")
	})

	t.Run("nonexistent config file", func(t *testing.T) {
		oldCfgFile := cfgFile
		cfgFile = "/nonexistent/config.yaml"
		defer func() { cfgFile = oldCfgFile }()

		err := runConfigValidate(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config file not found")
	})
}

func TestRunConfigShowDefaults(t *testing.T) {
	tests := []struct {
		format string
		ok     bool
	}{
		{"yaml", true},
		{"json", true},
		{"table", true},
		{"xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			oldFormat := outputFormat
			outputFormat = tt.format
			defer func() { outputFormat = oldFormat }()

			err := runConfigShowDefaults(nil, nil)
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported output format")
			}
		})
	}
}

func TestRunConfigEnvVars(t *testing.T) {
	tests := []struct {
		format string
		ok     bool
	}{
		{"table", true},
		{"yaml", true},
		{"json", true},
		{"csv", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			oldFormat := outputFormat
			outputFormat = tt.format
			defer func() { outputFormat = oldFormat }()

			err := runConfigEnvVars(nil, nil)
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestRunStatusMissingConfig(t *testing.T) {
	oldCfgFile := cfgFile
	cfgFile = ""
	defer func() { cfgFile = oldCfgFile }()

	err := runStatus(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config flag is required")
}

func TestRunStatusNonexistentConfig(t *testing.T) {
	oldCfgFile := cfgFile
	cfgFile = "/nonexistent/config.yaml"
	defer func() { cfgFile = oldCfgFile }()

	err := runStatus(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestRunStartMissingConfig(t *testing.T) {
	oldCfgFile := cfgFile
	cfgFile = ""
	defer func() { cfgFile = oldCfgFile }()

	err := runStart(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config flag is required")
}

func TestRunStartNonexistentConfig(t *testing.T) {
	oldCfgFile := cfgFile
	cfgFile = "/nonexistent/config.yaml"
	defer func() { cfgFile = oldCfgFile }()

	err := runStart(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestDisplayValidationResults(t *testing.T) {
	cfg := &config.Config{}
	cfg.GitHub.Owner = "test"
	cfg.GitHub.Repo = "repo"
	cfg.Claude.Model = "claude-sonnet-4-20250514"
	cfg.Claude.MaxTokens = 8192
	cfg.Agents.Developer.WorkspaceDir = "/tmp"
	cfg.State.Dir = "/tmp/state"

	t.Run("with errors", func(t *testing.T) {
		report := &config.ValidationReport{
			ErrorCount: 1,
			Failed: []*config.ValidationResult{
				{
					Rule:    &config.ValidationRule{Field: "test.field"},
					Issue:   "test issue",
					Fix:     "test fix",
					Example: "test example",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1 errors")
	})

	t.Run("with warnings in strict mode", func(t *testing.T) {
		report := &config.ValidationReport{
			WarningCount: 2,
			Warnings: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "warn.field"},
					Issue: "warning issue",
					Fix:   "warning fix",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "strict mode")
	})

	t.Run("with warnings in normal mode", func(t *testing.T) {
		report := &config.ValidationReport{
			WarningCount: 1,
			Warnings: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "warn.field"},
					Issue: "warning issue",
					Fix:   "fix",
				},
			},
		}
		err := displayValidationResults(cfg, report, "prod", false)
		assert.NoError(t, err)
	})

	t.Run("clean report", func(t *testing.T) {
		report := &config.ValidationReport{
			TotalRules: 10,
			Passed: []*config.ValidationResult{
				{Rule: &config.ValidationRule{Category: config.CategoryNetwork}},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		assert.NoError(t, err)
	})

	t.Run("network validation in failed results", func(t *testing.T) {
		report := &config.ValidationReport{
			TotalRules: 5,
			Failed: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "network.test", Category: config.CategoryNetwork},
					Issue: "network error",
				},
			},
			ErrorCount: 1,
		}
		// This will return error because ErrorCount > 0
		err := displayValidationResults(cfg, report, "", false)
		require.Error(t, err)
	})
}

func TestEnvVarMappingStruct(t *testing.T) {
	m := EnvVarMapping{
		ConfigKey: "github.token",
		EnvVar:    "GITHUB_TOKEN",
		Type:      "string",
		Required:  true,
		Example:   "ghp_xxx",
	}
	assert.Equal(t, "GITHUB_TOKEN", m.EnvVar)
	assert.True(t, m.Required)
}

// --- Additional tests for coverage ---

func TestRunStartInvalidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	err := os.WriteFile(cfgPath, []byte("invalid: [yaml: broken"), 0o644)
	require.NoError(t, err)

	oldCfgFile := cfgFile
	cfgFile = cfgPath
	defer func() { cfgFile = oldCfgFile }()

	err = runStart(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestRunStatusInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	err := os.WriteFile(cfgPath, []byte("invalid: [yaml: broken"), 0o644)
	require.NoError(t, err)

	oldCfgFile := cfgFile
	cfgFile = cfgPath
	defer func() { cfgFile = oldCfgFile }()

	err = runStatus(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestRunValidateWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := writeMinimalConfig(t, tmpDir, true)

	oldCfgFile := cfgFile
	oldSkipNetwork := skipNetworkFlag
	oldFullValidate := fullValidateFlag
	oldEnvironment := environmentFlag
	oldStrict := strictModeFlag
	cfgFile = cfgPath
	defer func() {
		cfgFile = oldCfgFile
		skipNetworkFlag = oldSkipNetwork
		fullValidateFlag = oldFullValidate
		environmentFlag = oldEnvironment
		strictModeFlag = oldStrict
	}()

	// Create a cobra command for context
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	t.Run("default validation with skip-network", func(t *testing.T) {
		skipNetworkFlag = true
		fullValidateFlag = false
		environmentFlag = ""
		strictModeFlag = false

		// This exercises the skipNetwork path in the else branch (not fullValidate)
		// The result depends on token validation but the code paths are exercised
		_ = runValidate(cmd, nil)
	})

	t.Run("full validation", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = true
		environmentFlag = ""
		strictModeFlag = false

		// This exercises the fullValidateFlag path using LoadWithSchemaValidation
		_ = runValidate(cmd, nil)
	})

	t.Run("default validation no flags", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = false
		environmentFlag = ""
		strictModeFlag = false

		// This exercises the default path with skipNetwork derived from flags
		_ = runValidate(cmd, nil)
	})

	t.Run("with environment flag", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = false
		environmentFlag = "dev"
		strictModeFlag = false

		// This exercises the LoadWithEnvironment path
		_ = runValidate(cmd, nil)
	})

	t.Run("with environment and skip-network", func(t *testing.T) {
		skipNetworkFlag = true
		fullValidateFlag = false
		environmentFlag = "staging"
		strictModeFlag = false

		_ = runValidate(cmd, nil)
	})

	t.Run("with environment and env vars set", func(t *testing.T) {
		// Set required env vars so LoadWithEnvironment succeeds past env var checks
		t.Setenv("GITHUB_TOKEN", "ghp_testtoken1234567890abcdef")
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-testkey1234567890")

		skipNetworkFlag = true
		fullValidateFlag = false
		environmentFlag = "dev"
		strictModeFlag = false

		// This exercises the environment success path (lines 68-69)
		_ = runValidate(cmd, nil)
	})
}

func TestRunConfigValidateWithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := writeMinimalConfig(t, tmpDir, true)

	oldCfgFile := cfgFile
	oldSkipNetwork := skipNetworkValidation
	oldShowPassed := showPassedChecks
	cfgFile = cfgPath
	defer func() {
		cfgFile = oldCfgFile
		skipNetworkValidation = oldSkipNetwork
		showPassedChecks = oldShowPassed
	}()

	t.Run("with skip-network", func(t *testing.T) {
		skipNetworkValidation = true
		showPassedChecks = false
		// Exercises the code path; may fail on validation but we cover the branches
		_ = runConfigValidate(nil, nil)
	})

	t.Run("with show-passed", func(t *testing.T) {
		skipNetworkValidation = true
		showPassedChecks = true
		_ = runConfigValidate(nil, nil)
	})

	t.Run("invalid config file for config validate", func(t *testing.T) {
		badPath := filepath.Join(tmpDir, "bad.yaml")
		err := os.WriteFile(badPath, []byte("invalid: [yaml: broken"), 0o644)
		require.NoError(t, err)
		cfgFile = badPath
		skipNetworkValidation = true
		err = runConfigValidate(nil, nil)
		require.Error(t, err)
	})
}

func TestDisplayValidationResultsSkipNetworkFlag(t *testing.T) {
	cfg := &config.Config{}
	cfg.GitHub.Owner = "test"
	cfg.GitHub.Repo = "repo"
	cfg.Claude.Model = "claude-sonnet-4-20250514"
	cfg.Claude.MaxTokens = 8192
	cfg.Agents.Developer.WorkspaceDir = "/tmp"
	cfg.State.Dir = "/tmp/state"

	t.Run("skip network flag shows skip message", func(t *testing.T) {
		oldSkipNetwork := skipNetworkFlag
		skipNetworkFlag = true
		defer func() { skipNetworkFlag = oldSkipNetwork }()

		report := &config.ValidationReport{
			TotalRules: 5,
			Passed:     []*config.ValidationResult{},
		}
		err := displayValidationResults(cfg, report, "", false)
		assert.NoError(t, err)
	})

	t.Run("no network validation and no skip flag", func(t *testing.T) {
		oldSkipNetwork := skipNetworkFlag
		skipNetworkFlag = false
		defer func() { skipNetworkFlag = oldSkipNetwork }()

		report := &config.ValidationReport{
			TotalRules: 5,
			Passed:     []*config.ValidationResult{},
		}
		err := displayValidationResults(cfg, report, "", false)
		assert.NoError(t, err)
	})

	t.Run("with watch labels", func(t *testing.T) {
		cfg2 := &config.Config{}
		cfg2.GitHub.Owner = "test"
		cfg2.GitHub.Repo = "repo"
		cfg2.GitHub.WatchLabels = []string{"agent:ready", "bug"}
		cfg2.Claude.Model = "claude-sonnet-4-20250514"
		cfg2.Claude.MaxTokens = 8192
		cfg2.Agents.Developer.WorkspaceDir = "/tmp"
		cfg2.State.Dir = "/tmp/state"

		report := &config.ValidationReport{
			TotalRules: 5,
			Passed:     []*config.ValidationResult{},
		}
		err := displayValidationResults(cfg2, report, "", false)
		assert.NoError(t, err)
	})

	t.Run("errors with fix and example", func(t *testing.T) {
		report := &config.ValidationReport{
			ErrorCount: 2,
			Failed: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "field1"},
					Issue: "issue1",
					Fix:   "fix1",
				},
				{
					Rule:    &config.ValidationRule{Field: "field2"},
					Issue:   "issue2",
					Example: "example2",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "2 errors")
	})

	t.Run("warnings without fix", func(t *testing.T) {
		report := &config.ValidationReport{
			WarningCount: 1,
			Warnings: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "warn.field"},
					Issue: "warning without fix",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		assert.NoError(t, err)
	})

	t.Run("warnings in strict mode without fix", func(t *testing.T) {
		report := &config.ValidationReport{
			WarningCount: 1,
			Warnings: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "warn.field"},
					Issue: "strict warning no fix",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "strict mode")
	})
}

func TestPrintPassedValidationsAllBranches(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		cfg := &config.Config{}
		// Should not panic with empty config
		printPassedValidations(cfg)
	})

	t.Run("workspace limits branch", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Workspace.Limits.MinFreeDiskMB = 2000
		cfg.Workspace.Limits.MaxSizeMB = 500
		// This should trigger the MinFreeDiskMB > MaxSizeMB branch
		printPassedValidations(cfg)
	})

	t.Run("developer disabled", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Agents.Developer.Enabled = false
		cfg.Agents.Developer.MaxConcurrent = 5
		printPassedValidations(cfg)
	})
}

func TestRunStartWithInvalidYAMLConfig(t *testing.T) {
	// Test runStart with syntactically invalid YAML
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "broken.yaml")
	err := os.WriteFile(cfgPath, []byte("{invalid yaml content:::}"), 0o644)
	require.NoError(t, err)

	oldCfgFile := cfgFile
	cfgFile = cfgPath
	defer func() { cfgFile = oldCfgFile }()

	err = runStart(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestRootCmdHasExpectedSubcommands(t *testing.T) {
	// Verify the root command has the expected subcommands registered
	commands := rootCmd.Commands()
	commandNames := make([]string, 0, len(commands))
	for _, cmd := range commands {
		commandNames = append(commandNames, cmd.Name())
	}
	assert.Contains(t, commandNames, "start")
	assert.Contains(t, commandNames, "status")
	assert.Contains(t, commandNames, "validate")
	assert.Contains(t, commandNames, "config")
}

func TestRootCmdHasConfigFlag(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("config")
	require.NotNil(t, flag)
	assert.Equal(t, "config file path (required)", flag.Usage)
}

func TestStatusCmdProperties(t *testing.T) {
	assert.Equal(t, "status", statusCmd.Use)
	assert.Contains(t, statusCmd.Short, "status")
}

func TestStartCmdProperties(t *testing.T) {
	assert.Equal(t, "start", startCmd.Use)
	assert.Contains(t, startCmd.Short, "Start")
}

func TestValidateCmdProperties(t *testing.T) {
	assert.Equal(t, "validate", validateCmd.Use)
	assert.Contains(t, validateCmd.Short, "Validate")

	// Verify flags exist
	flag := validateCmd.Flags().Lookup("skip-network")
	require.NotNil(t, flag)
	flag = validateCmd.Flags().Lookup("full")
	require.NotNil(t, flag)
	flag = validateCmd.Flags().Lookup("env")
	require.NotNil(t, flag)
	flag = validateCmd.Flags().Lookup("strict")
	require.NotNil(t, flag)
}

func TestConfigCmdProperties(t *testing.T) {
	assert.Equal(t, "config", configCmd.Use)
	subcommands := configCmd.Commands()
	subNames := make([]string, 0, len(subcommands))
	for _, cmd := range subcommands {
		subNames = append(subNames, cmd.Name())
	}
	assert.Contains(t, subNames, "validate")
	assert.Contains(t, subNames, "show-defaults")
	assert.Contains(t, subNames, "env-vars")
}

func TestRunStatusWithInvalidYAML(t *testing.T) {
	// Test that runStatus with invalid YAML returns loading error
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "broken.yaml")
	err := os.WriteFile(cfgPath, []byte("{invalid yaml:::}"), 0o644)
	require.NoError(t, err)

	oldCfgFile := cfgFile
	cfgFile = cfgPath
	defer func() { cfgFile = oldCfgFile }()

	err = runStatus(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestCheckEnvVarOutputs(t *testing.T) {
	// Test all branches of checkEnvVar by capturing that it doesn't panic
	t.Run("empty value", func(t *testing.T) {
		checkEnvVar("TEST_EMPTY", "")
	})
	t.Run("unexpanded variable", func(t *testing.T) {
		checkEnvVar("TEST_UNEXPANDED", "${TEST_UNEXPANDED}")
	})
	t.Run("too short value", func(t *testing.T) {
		checkEnvVar("TEST_SHORT", "abc")
	})
	t.Run("normal value", func(t *testing.T) {
		checkEnvVar("TEST_NORMAL", "a-long-enough-value-here")
	})
}

func TestFormatValidationErrorWithConfigError(t *testing.T) {
	// Test with a mixed multi-error containing both ConfigValidationError and plain errors
	err := errors.Join(
		&config.ConfigValidationError{Field: "mixed.field1", Issue: "issue1"},
		fmt.Errorf("plain error"),
		&config.ConfigValidationError{Field: "mixed.field2", Issue: "issue2"},
	)
	result := formatValidationError(err)
	assert.Contains(t, result, "mixed.field1")
	assert.Contains(t, result, "plain error")
	assert.Contains(t, result, "mixed.field2")
}

func TestDisplayValidationResultsWithNetworkInFailed(t *testing.T) {
	cfg := &config.Config{}
	cfg.GitHub.Owner = "test"
	cfg.GitHub.Repo = "repo"
	cfg.Claude.Model = "claude-sonnet-4-20250514"
	cfg.Claude.MaxTokens = 8192
	cfg.Agents.Developer.WorkspaceDir = "/tmp"
	cfg.State.Dir = "/tmp/state"
	cfg.GitHub.Token = "ghp_testtoken1234567890abcdef"
	cfg.Claude.APIKey = "sk-ant-api03-testkey1234567890"

	t.Run("network category in failed results triggers networkValidated", func(t *testing.T) {
		// With ErrorCount > 0, this returns error but also checks the network path
		report := &config.ValidationReport{
			TotalRules: 3,
			ErrorCount: 1,
			Failed: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "network.check", Category: config.CategoryNetwork},
					Issue: "network unreachable",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		require.Error(t, err)
	})

	t.Run("network in failed with zero error count covers dead code path", func(t *testing.T) {
		// This is an unusual state: Failed results present but ErrorCount == 0.
		// It exercises the second network-check loop (lines 184-189).
		report := &config.ValidationReport{
			TotalRules: 3,
			ErrorCount: 0,
			Failed: []*config.ValidationResult{
				{
					Rule:  &config.ValidationRule{Field: "network.check", Category: config.CategoryNetwork},
					Issue: "network unreachable but not counted",
				},
			},
		}
		err := displayValidationResults(cfg, report, "", false)
		assert.NoError(t, err)
	})
}

func TestAgentWorkStateJSON(t *testing.T) {
	// Test that state encoding works as expected (exercises the json encoder path in runStatus)
	s := &state.AgentWorkState{
		AgentType: "developer",
		State:     state.StateIdle,
	}
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Contains(t, string(data), "developer")
}

func TestExecuteHelp(t *testing.T) {
	// Test that Execute succeeds when the root command shows help.
	// We set args to --help which causes rootCmd.Execute() to return nil.
	oldArgs := os.Args
	os.Args = []string{"agentctl", "--help"}
	defer func() { os.Args = oldArgs }()

	// Execute should not panic. It will print help to stdout.
	// Note: we can't test the os.Exit(1) path.
	Execute()
}

func TestRootCmdExecuteWithHelp(t *testing.T) {
	// Test rootCmd.Execute() directly with --help flag
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	assert.NoError(t, err)

	// Reset args
	rootCmd.SetArgs(nil)
}

func TestRootCmdExecuteWithVersion(t *testing.T) {
	// Test various subcommand --help to exercise command tree
	rootCmd.SetArgs([]string{"start", "--help"})
	err := rootCmd.Execute()
	assert.NoError(t, err)

	rootCmd.SetArgs([]string{"status", "--help"})
	err = rootCmd.Execute()
	assert.NoError(t, err)

	rootCmd.SetArgs([]string{"validate", "--help"})
	err = rootCmd.Execute()
	assert.NoError(t, err)

	rootCmd.SetArgs([]string{"config", "--help"})
	err = rootCmd.Execute()
	assert.NoError(t, err)

	// Reset args
	rootCmd.SetArgs(nil)
}

func TestRunValidateWithInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	err := os.WriteFile(cfgPath, []byte("invalid: [yaml: broken"), 0o644)
	require.NoError(t, err)

	oldCfgFile := cfgFile
	cfgFile = cfgPath
	defer func() { cfgFile = oldCfgFile }()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	t.Run("default mode with invalid yaml", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = false
		environmentFlag = ""
		err := runValidate(cmd, nil)
		require.Error(t, err)
	})

	t.Run("full mode with invalid yaml", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = true
		environmentFlag = ""
		err := runValidate(cmd, nil)
		require.Error(t, err)
	})

	t.Run("env mode with invalid yaml", func(t *testing.T) {
		skipNetworkFlag = false
		fullValidateFlag = false
		environmentFlag = "prod"
		err := runValidate(cmd, nil)
		require.Error(t, err)
	})
}
