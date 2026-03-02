package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
)

// ConfigValidationError provides structured error reporting with actionable feedback.
type ConfigValidationError struct {
	Field   string
	Value   interface{}
	Issue   string
	Fix     string
	Example string
}

func (e ConfigValidationError) Error() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s: %s", e.Field, e.Issue))
	if e.Fix != "" {
		parts = append(parts, fmt.Sprintf("Fix: %s", e.Fix))
	}
	if e.Example != "" {
		parts = append(parts, fmt.Sprintf("Example: %s", e.Example))
	}
	return strings.Join(parts, ". ")
}

// Validate checks that all required configuration fields are set.
func Validate(cfg *Config) error {
	return ValidateWithContext(context.Background(), cfg, false)
}

// ValidateWithContext performs comprehensive validation with optional runtime checks.
func ValidateWithContext(ctx context.Context, cfg *Config, skipNetworkValidation bool) error {
	var errs []error

	// Basic field validation
	if fieldErrs := validateFields(cfg); len(fieldErrs) > 0 {
		errs = append(errs, fieldErrs...)
	}

	// Interdependency validation BEFORE defaults (to catch zero values)
	if interdepErrs := validateInterdependencies(cfg); len(interdepErrs) > 0 {
		errs = append(errs, interdepErrs...)
	}

	// Apply defaults after validation
	applyDefaults(cfg)

	// Workspace validation
	if workspaceErrs := validateWorkspacePermissions(cfg); len(workspaceErrs) > 0 {
		errs = append(errs, workspaceErrs...)
	}

	// Network validation (can be skipped for faster startup)
	if !skipNetworkValidation {
		if networkErrs := validateNetworkAccess(ctx, cfg); len(networkErrs) > 0 {
			errs = append(errs, networkErrs...)
		}
	}

	return errors.Join(errs...)
}

// validateFields validates required fields and formats.
func validateFields(cfg *Config) []error {
	var errs []error

	// GitHub configuration
	if cfg.GitHub.Token == "" {
		errs = append(errs, ConfigValidationError{
			Field:   "github.token",
			Issue:   "required field is empty",
			Fix:     "Set GITHUB_TOKEN environment variable or provide token directly",
			Example: "ghp_xxxxxxxxxxxxxxxxxxxx",
		})
	} else if !strings.HasPrefix(cfg.GitHub.Token, "ghp_") && !strings.HasPrefix(cfg.GitHub.Token, "github_pat_") {
		errs = append(errs, ConfigValidationError{
			Field:   "github.token",
			Value:   maskToken(cfg.GitHub.Token),
			Issue:   "token format appears invalid",
			Fix:     "Use a personal access token from GitHub settings",
			Example: "ghp_xxxxxxxxxxxxxxxxxxxx",
		})
	}

	if cfg.GitHub.Owner == "" {
		errs = append(errs, ConfigValidationError{
			Field:   "github.owner",
			Issue:   "required field is empty",
			Fix:     "Specify the GitHub repository owner/organization",
			Example: "myorg",
		})
	}

	if cfg.GitHub.Repo == "" {
		errs = append(errs, ConfigValidationError{
			Field:   "github.repo",
			Issue:   "required field is empty",
			Fix:     "Specify the GitHub repository name",
			Example: "myrepo",
		})
	}

	// Claude configuration
	if cfg.Claude.APIKey == "" {
		errs = append(errs, ConfigValidationError{
			Field:   "claude.api_key",
			Issue:   "required field is empty",
			Fix:     "Set ANTHROPIC_API_KEY environment variable or provide key directly",
			Example: "sk-ant-api03-xxxxxxxxxxxx",
		})
	} else if !strings.HasPrefix(cfg.Claude.APIKey, "sk-ant-") {
		errs = append(errs, ConfigValidationError{
			Field:   "claude.api_key",
			Value:   maskToken(cfg.Claude.APIKey),
			Issue:   "API key format appears invalid",
			Fix:     "Use an API key from Anthropic Console",
			Example: "sk-ant-api03-xxxxxxxxxxxx",
		})
	}

	return errs
}

// validateInterdependencies checks that related configuration settings make sense together.
func validateInterdependencies(cfg *Config) []error {
	var errs []error

	// Agent concurrency checks
	if cfg.Agents.Developer.Enabled && cfg.Agents.Developer.MaxConcurrent <= 0 {
		errs = append(errs, ConfigValidationError{
			Field:   "agents.developer.max_concurrent",
			Value:   cfg.Agents.Developer.MaxConcurrent,
			Issue:   "must be greater than 0 when developer agent is enabled",
			Fix:     "Set to a positive integer (recommended: 1-3)",
			Example: "1",
		})
	}

	// Creativity checks
	if cfg.Creativity.Enabled {
		if cfg.Creativity.MaxPendingSuggestions <= 0 {
			errs = append(errs, ConfigValidationError{
				Field:   "creativity.max_pending_suggestions",
				Value:   cfg.Creativity.MaxPendingSuggestions,
				Issue:   "must be positive when creativity is enabled",
				Fix:     "Set to positive integer to limit open suggestions",
				Example: "1",
			})
		}
	}

	// Decomposition checks
	if cfg.Decomposition.Enabled {
		if cfg.Decomposition.MaxSubtasks <= 0 {
			errs = append(errs, ConfigValidationError{
				Field:   "decomposition.max_subtasks",
				Value:   cfg.Decomposition.MaxSubtasks,
				Issue:   "must be positive when decomposition is enabled",
				Fix:     "Set to positive integer (recommended: 3-5)",
				Example: "5",
			})
		}
		if cfg.Decomposition.MaxIterationBudget <= 0 {
			errs = append(errs, ConfigValidationError{
				Field:   "decomposition.max_iteration_budget",
				Value:   cfg.Decomposition.MaxIterationBudget,
				Issue:   "must be positive when decomposition is enabled",
				Fix:     "Set to positive integer (recommended: 20-50)",
				Example: "25",
			})
		}
	}

	// Workspace limits validation
	if cfg.Workspace.Limits.MaxSizeMB > 0 && cfg.Workspace.Limits.MinFreeDiskMB > 0 {
		if cfg.Workspace.Limits.MinFreeDiskMB <= cfg.Workspace.Limits.MaxSizeMB {
			errs = append(errs, ConfigValidationError{
				Field:   "workspace.limits.min_free_disk_mb",
				Value:   fmt.Sprintf("%d (max_size_mb: %d)", cfg.Workspace.Limits.MinFreeDiskMB, cfg.Workspace.Limits.MaxSizeMB),
				Issue:   "should be larger than max_size_mb to prevent disk space exhaustion",
				Fix:     "Increase min_free_disk_mb or decrease max_size_mb",
				Example: fmt.Sprintf("%d", cfg.Workspace.Limits.MaxSizeMB*2),
			})
		}
	}

	return errs
}

// validateWorkspacePermissions checks workspace directory accessibility.
func validateWorkspacePermissions(cfg *Config) []error {
	var errs []error

	workspaceDir := cfg.Agents.Developer.WorkspaceDir
	if workspaceDir == "" {
		return errs
	}

	// Check if directory exists or can be created
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		errs = append(errs, ConfigValidationError{
			Field: "agents.developer.workspace_dir",
			Value: workspaceDir,
			Issue: fmt.Sprintf("cannot create directory: %v", err),
			Fix:   "Choose a writable directory path or fix permissions",
			Example: "./workspaces",
		})
		return errs
	}

	// Check write permissions by creating a test file
	testFile := filepath.Join(workspaceDir, ".agentctl-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		errs = append(errs, ConfigValidationError{
			Field: "agents.developer.workspace_dir",
			Value: workspaceDir,
			Issue: fmt.Sprintf("directory is not writable: %v", err),
			Fix:   "Fix directory permissions or choose a different path",
			Example: "./workspaces",
		})
	} else {
		// Clean up test file
		os.Remove(testFile)
	}

	// Check state directory as well
	stateDir := cfg.State.Dir
	if stateDir != "" {
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			errs = append(errs, ConfigValidationError{
				Field: "state.dir",
				Value: stateDir,
				Issue: fmt.Sprintf("cannot create state directory: %v", err),
				Fix:   "Choose a writable directory path or fix permissions",
				Example: ".agentctl/state",
			})
		}
	}

	return errs
}

// validateNetworkAccess performs runtime validation of external services.
func validateNetworkAccess(ctx context.Context, cfg *Config) []error {
	var errs []error

	// Create context with timeout for network operations
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Validate GitHub access
	if err := validateGitHubAccess(ctx, &cfg.GitHub); err != nil {
		errs = append(errs, err)
	}

	// Validate Claude access
	if err := validateClaudeAccess(ctx, &cfg.Claude); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// validateGitHubAccess checks GitHub token permissions and repository access.
func validateGitHubAccess(ctx context.Context, cfg *GitHubConfig) error {
	if cfg.Token == "" {
		return nil // Already handled in field validation
	}

	client := github.NewClient(nil).WithAuthToken(cfg.Token)

	// Check token scopes
	_, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return ConfigValidationError{
			Field: "github.token",
			Issue: fmt.Sprintf("token authentication failed: %v", err),
			Fix:   "Verify token is valid and not expired",
			Example: "ghp_xxxxxxxxxxxxxxxxxxxx",
		}
	}

	// Check required scopes from response headers
	if scopes := resp.Header.Get("X-OAuth-Scopes"); scopes != "" {
		if !strings.Contains(scopes, "repo") && !strings.Contains(scopes, "public_repo") {
			return ConfigValidationError{
				Field: "github.token",
				Issue: "token missing required 'repo' scope",
				Fix:   "Generate new token with 'repo' scope at https://github.com/settings/tokens",
				Example: "Token scopes should include: repo, read:user",
			}
		}
	}

	// Check repository access
	if cfg.Owner != "" && cfg.Repo != "" {
		_, _, err := client.Repositories.Get(ctx, cfg.Owner, cfg.Repo)
		if err != nil {
			return ConfigValidationError{
				Field: "github.owner/github.repo",
				Value: fmt.Sprintf("%s/%s", cfg.Owner, cfg.Repo),
				Issue: fmt.Sprintf("repository not accessible: %v", err),
				Fix:   "Verify repository exists and token has access",
				Example: "myorg/myrepo",
			}
		}
	}

	return nil
}

// validateClaudeAccess checks Claude API key validity.
func validateClaudeAccess(ctx context.Context, cfg *ClaudeConfig) error {
	if cfg.APIKey == "" {
		return nil // Already handled in field validation
	}

	// Simple health check - make a basic HTTP request to Anthropic API
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/messages", nil)
	if err != nil {
		return nil // Skip validation if we can't create request
	}

	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Network errors are not config errors - warn but don't fail
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return ConfigValidationError{
			Field: "claude.api_key",
			Issue: "API key authentication failed",
			Fix:   "Verify API key is valid at https://console.anthropic.com/",
			Example: "sk-ant-api03-xxxxxxxxxxxx",
		}
	}

	return nil
}

// applyDefaults sets default values for optional configuration fields.
func applyDefaults(cfg *Config) {
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = "claude-sonnet-4-20250514"
	}
	if cfg.Claude.MaxTokens == 0 {
		cfg.Claude.MaxTokens = 8192
	}
	if cfg.GitHub.PollInterval == 0 {
		cfg.GitHub.PollInterval = 30 * time.Second
	}
	if cfg.State.Backend == "" {
		cfg.State.Backend = "file"
	}
	if cfg.State.Dir == "" {
		cfg.State.Dir = ".agentctl/state"
	}
	if cfg.Agents.Developer.MaxConcurrent == 0 {
		cfg.Agents.Developer.MaxConcurrent = 1
	}
	if cfg.Agents.Developer.WorkspaceDir == "" {
		cfg.Agents.Developer.WorkspaceDir = "./workspaces"
	}

	// Creativity defaults
	if cfg.Creativity.IdleThresholdSeconds == 0 {
		cfg.Creativity.IdleThresholdSeconds = 120
	}
	if cfg.Creativity.SuggestionCooldownSeconds == 0 {
		cfg.Creativity.SuggestionCooldownSeconds = 300
	}
	if cfg.Creativity.MaxPendingSuggestions == 0 {
		cfg.Creativity.MaxPendingSuggestions = 1
	}
	if cfg.Creativity.MaxRejectionHistory == 0 {
		cfg.Creativity.MaxRejectionHistory = 50
	}

	// Decomposition defaults
	if cfg.Decomposition.MaxIterationBudget == 0 {
		cfg.Decomposition.MaxIterationBudget = 25
	}
	if cfg.Decomposition.MaxSubtasks == 0 {
		cfg.Decomposition.MaxSubtasks = 5
	}

	// Workspace defaults
	if cfg.Workspace.Limits.MaxSizeMB == 0 {
		cfg.Workspace.Limits.MaxSizeMB = 1024 // 1GB default
	}
	if cfg.Workspace.Limits.MinFreeDiskMB == 0 {
		cfg.Workspace.Limits.MinFreeDiskMB = 2048 // 2GB default
	}
	if cfg.Workspace.Cleanup.MaxConcurrent == 0 {
		cfg.Workspace.Cleanup.MaxConcurrent = 5
	}
	if cfg.Workspace.Cleanup.SuccessRetention == 0 {
		cfg.Workspace.Cleanup.SuccessRetention = 24 * time.Hour
	}
	if cfg.Workspace.Cleanup.FailureRetention == 0 {
		cfg.Workspace.Cleanup.FailureRetention = 168 * time.Hour // 1 week
	}
	if cfg.Workspace.Monitoring.DiskCheckInterval == 0 {
		cfg.Workspace.Monitoring.DiskCheckInterval = 5 * time.Minute
	}
	if cfg.Workspace.Monitoring.CleanupInterval == 0 {
		cfg.Workspace.Monitoring.CleanupInterval = 1 * time.Hour
	}
}

// maskToken masks sensitive token values for error messages.
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}
