package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator(t *testing.T) {
	tests := []struct {
		name             string
		config           *Config
		skipNetwork      bool
		expectedErrors   int
		expectedWarnings int
		expectPass       bool
	}{
		{
			name: "valid_minimal_config",
			config: &Config{
				GitHub: GitHubConfig{
					Token: "ghp_test_token_1234567890",
					Owner: "testowner",
					Repo:  "testrepo",
				},
				Claude: ClaudeConfig{
					APIKey: "sk-ant-api03-test_key_1234567890",
				},
				Agents: AgentsConfig{
					Developer: DeveloperAgentConfig{
						Enabled:       true,
						MaxConcurrent: 1,
						WorkspaceDir:  t.TempDir(),
					},
				},
				State: StateConfig{
					Dir: t.TempDir(),
				},
			},
			skipNetwork:      true,
			expectedErrors:   0,
			expectedWarnings: 0,
			expectPass:       true,
		},
		{
			name: "missing_required_fields",
			config: &Config{
				GitHub: GitHubConfig{
					// Missing token, owner, repo
				},
				Claude: ClaudeConfig{
					// Missing API key
				},
			},
			skipNetwork:    true,
			expectedErrors: 4, // token, owner, repo, api_key
			expectPass:     false,
		},
		{
			name: "invalid_token_formats",
			config: &Config{
				GitHub: GitHubConfig{
					Token: "invalid_token_format",
					Owner: "testowner",
					Repo:  "testrepo",
				},
				Claude: ClaudeConfig{
					APIKey: "invalid_api_key_format",
				},
				Agents: AgentsConfig{
					Developer: DeveloperAgentConfig{
						Enabled: true,
					},
				},
			},
			skipNetwork:    true,
			expectedErrors: 3, // github token format, claude key format, max_concurrent
			expectPass:     false,
		},
		{
			name: "warning_conditions",
			config: &Config{
				GitHub: GitHubConfig{
					Token: "ghp_test_token_1234567890",
					Owner: "testowner",
					Repo:  "testrepo",
				},
				Claude: ClaudeConfig{
					APIKey:    "sk-ant-api03-test_key_1234567890",
					MaxTokens: 300000, // Exceeds reasonable limit
				},
				Agents: AgentsConfig{
					Developer: DeveloperAgentConfig{
						Enabled:       true,
						MaxConcurrent: 1,
						WorkspaceDir:  t.TempDir(),
					},
				},
				State: StateConfig{
					Dir: t.TempDir(),
				},
				Workspace: WorkspaceConfig{
					Limits: WorkspaceLimitsConfig{
						MaxSizeMB:     1000,
						MinFreeDiskMB: 500, // Less than max size
					},
				},
			},
			skipNetwork:      true,
			expectedErrors:   0,
			expectedWarnings: 2, // max tokens and workspace limits
			expectPass:       true,
		},
		{
			name: "agent_enabled_without_concurrency",
			config: &Config{
				GitHub: GitHubConfig{
					Token: "ghp_test_token_1234567890",
					Owner: "testowner",
					Repo:  "testrepo",
				},
				Claude: ClaudeConfig{
					APIKey: "sk-ant-api03-test_key_1234567890",
				},
				Agents: AgentsConfig{
					Developer: DeveloperAgentConfig{
						Enabled:       true,
						MaxConcurrent: 0, // Invalid with enabled=true
					},
				},
			},
			skipNetwork:    true,
			expectedErrors: 1,
			expectPass:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator().WithSkipNetwork(tt.skipNetwork)

			report := validator.ValidateConfig(context.Background(), tt.config)

			assert.Equal(t, tt.expectedErrors, report.ErrorCount, "Error count mismatch")
			assert.Equal(t, tt.expectedWarnings, report.WarningCount, "Warning count mismatch")

			if tt.expectPass {
				assert.Equal(t, 0, report.ErrorCount, "Should pass validation")
			} else {
				assert.Greater(t, report.ErrorCount, 0, "Should fail validation")
			}

			// Verify report structure
			assert.Equal(t, len(validator.rules), report.TotalRules, "Total rules should match validator rules")

			// Check that all results have proper metadata
			for _, result := range report.Failed {
				assert.NotNil(t, result.Rule)
				assert.NotEmpty(t, result.Issue)
				// Most failed rules should have fix suggestions
				if result.Rule.Level == ValidationLevelError {
					assert.NotEmpty(t, result.Fix, "Error rules should have fix suggestions")
				}
			}

			for _, result := range report.Warnings {
				assert.NotNil(t, result.Rule)
				assert.NotEmpty(t, result.Issue)
			}
		})
	}
}

func TestValidationRuleCategories(t *testing.T) {
	validator := NewValidator().WithSkipNetwork(true)

	// Count rules by category
	categoryCounts := make(map[ValidationCategory]int)
	for _, rule := range validator.rules {
		categoryCounts[rule.Category]++
	}

	// Ensure we have rules for each important category
	assert.Greater(t, categoryCounts[CategoryRequired], 0, "Should have required field rules")
	assert.Greater(t, categoryCounts[CategoryFormat], 0, "Should have format validation rules")
	assert.Greater(t, categoryCounts[CategoryPermissions], 0, "Should have permissions rules")
	assert.Greater(t, categoryCounts[CategoryLimits], 0, "Should have limits validation rules")

	t.Logf("Rule distribution by category: %+v", categoryCounts)
}

func TestValidationLevels(t *testing.T) {
	validator := NewValidator().WithSkipNetwork(true)

	// Count rules by level
	levelCounts := make(map[ValidationLevel]int)
	for _, rule := range validator.rules {
		levelCounts[rule.Level]++
	}

	// Ensure we have different severity levels
	assert.Greater(t, levelCounts[ValidationLevelError], 0, "Should have error-level rules")
	assert.Greater(t, levelCounts[ValidationLevelWarning], 0, "Should have warning-level rules")

	t.Logf("Rule distribution by level: %+v", levelCounts)
}

func TestWorkspacePermissionValidation(t *testing.T) {
	validator := NewValidator()

	// Test writable directory
	writableDir := t.TempDir()
	config := &Config{
		Agents: AgentsConfig{
			Developer: DeveloperAgentConfig{
				WorkspaceDir: writableDir,
			},
		},
	}

	result := validator.checkWorkspacePermissions(config)
	assert.True(t, result.Passed, "Should pass for writable directory")

	// Test non-existent directory that can be created
	nonExistentDir := t.TempDir() + "/subdir/deeper"
	config.Agents.Developer.WorkspaceDir = nonExistentDir

	result = validator.checkWorkspacePermissions(config)
	assert.True(t, result.Passed, "Should pass for directory that can be created")

	// Test read-only directory (simulate by using /dev/null parent)
	readOnlyPath := "/dev/null/cannot-create"
	config.Agents.Developer.WorkspaceDir = readOnlyPath

	result = validator.checkWorkspacePermissions(config)
	assert.False(t, result.Passed, "Should fail for unwritable path")
	assert.NotEmpty(t, result.Issue)
	assert.NotEmpty(t, result.Fix)
}

func TestTokenFormatValidation(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{"valid_classic_pat", "ghp_1234567890abcdef", true},
		{"valid_fine_grained_pat", "github_pat_11ABCDEFG", true},
		{"valid_app_token", "ghs_1234567890abcdef", true},
		{"invalid_format", "invalid_token", false},
		{"empty_token", "", true}, // Skip format check if empty
		{"old_format", "1234567890abcdef", false},
		{"wrong_prefix", "gho_1234567890abcdef", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				GitHub: GitHubConfig{
					Token: tt.token,
				},
			}

			result := validator.checkGitHubTokenFormat(config)
			assert.Equal(t, tt.expected, result.Passed, "Token format validation mismatch")

			if !result.Passed && tt.token != "" {
				assert.NotEmpty(t, result.Issue)
				assert.NotEmpty(t, result.Fix)
				assert.NotEmpty(t, result.Example)
			}
		})
	}
}

func TestReportToErrorConversion(t *testing.T) {
	// Test successful report
	successReport := &ValidationReport{
		ErrorCount: 0,
	}

	err := successReport.ToError()
	assert.NoError(t, err)

	// Test failed report with single error
	failedReport := &ValidationReport{
		ErrorCount: 1,
		Failed: []*ValidationResult{
			{
				Rule:  &ValidationRule{Field: "test.field"},
				Issue: "test issue",
				Fix:   "test fix",
			},
		},
	}

	err = failedReport.ToError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test issue")

	// Check if it's a structured validation error
	if validationErr, ok := err.(*ConfigValidationError); ok {
		assert.Equal(t, "test.field", validationErr.Field)
		assert.Equal(t, "test issue", validationErr.Issue)
		assert.Equal(t, "test fix", validationErr.Fix)
	}
}

func TestValidatorWithNetworkSkip(t *testing.T) {
	validator := NewValidator()

	// Count network rules
	networkRuleCount := 0
	for _, rule := range validator.rules {
		if rule.Category == CategoryNetwork {
			networkRuleCount++
		}
	}

	assert.Greater(t, networkRuleCount, 0, "Should have network validation rules")

	// Test that network rules are skipped
	validatorSkipNetwork := NewValidator().WithSkipNetwork(true)
	config := &Config{
		GitHub: GitHubConfig{
			Token: "ghp_test_token",
			Owner: "testowner",
			Repo:  "testrepo",
		},
		Claude: ClaudeConfig{
			APIKey: "sk-ant-api03-test_key",
		},
	}

	report := validatorSkipNetwork.ValidateConfig(context.Background(), config)

	// Verify no network rules were executed
	networkRulesExecuted := 0
	for _, result := range report.Passed {
		if result.Rule.Category == CategoryNetwork {
			networkRulesExecuted++
		}
	}
	for _, result := range report.Failed {
		if result.Rule.Category == CategoryNetwork {
			networkRulesExecuted++
		}
	}

	assert.Equal(t, 0, networkRulesExecuted, "No network rules should be executed when skipped")
}
