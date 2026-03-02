package config

import (
	"context"
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
	var validationErrs ValidationErrors

	// Basic field validation
	validateFields(cfg, &validationErrs)

	// Interdependency validation BEFORE defaults (to catch zero values)
	validateInterdependencies(cfg, &validationErrs)

	// Apply defaults after validation
	ApplyDefaults(cfg)

	// Workspace validation
	validateWorkspacePermissions(cfg, &validationErrs)

	// Numeric range validation
	validateNumericRanges(cfg, &validationErrs)

	// Duration validation
	validateDurations(cfg, &validationErrs)

	// Network validation (can be skipped for faster startup)
	if !skipNetworkValidation {
		validateNetworkAccess(ctx, cfg, &validationErrs)
	}

	return validationErrs.ToError()
}

// validateFields validates required fields and formats.
func validateFields(cfg *Config, errs *ValidationErrors) {
	// GitHub configuration
	if cfg.GitHub.Token == "" {
		errs.Add("github.token", "", "required", "Set GITHUB_TOKEN environment variable or provide token directly")
	} else if !strings.HasPrefix(cfg.GitHub.Token, "ghp_") && !strings.HasPrefix(cfg.GitHub.Token, "github_pat_") {
		errs.Add("github.token", maskToken(cfg.GitHub.Token), "format", "Use a personal access token from GitHub settings (example: ghp_xxxxxxxxxxxxxxxxxxxx)")
	}

	if cfg.GitHub.Owner == "" {
		errs.Add("github.owner", "", "required", "Specify the GitHub repository owner/organization (example: myorg)")
	}

	if cfg.GitHub.Repo == "" {
		errs.Add("github.repo", "", "required", "Specify the GitHub repository name (example: myrepo)")
	}

	// Claude configuration
	if cfg.Claude.APIKey == "" {
		errs.Add("claude.api_key", "", "required", "Set ANTHROPIC_API_KEY environment variable or provide key directly")
	} else if !strings.HasPrefix(cfg.Claude.APIKey, "sk-ant-") {
		errs.Add("claude.api_key", maskToken(cfg.Claude.APIKey), "format", "Use an API key from Anthropic Console (example: sk-ant-api03-xxxxxxxxxxxx)")
	}
}

// validateInterdependencies checks that related configuration settings make sense together.
func validateInterdependencies(cfg *Config, errs *ValidationErrors) {
	// Agent concurrency checks
	if cfg.Agents.Developer.Enabled && cfg.Agents.Developer.MaxConcurrent <= 0 {
		errs.Add("agents.developer.max_concurrent", cfg.Agents.Developer.MaxConcurrent, "dependency", "must be greater than 0 when developer agent is enabled (recommended: 1-3)")
	}

	// Creativity checks
	if cfg.Creativity.Enabled {
		if cfg.Creativity.MaxPendingSuggestions <= 0 {
			errs.Add("creativity.max_pending_suggestions", cfg.Creativity.MaxPendingSuggestions, "dependency", "must be positive when creativity is enabled (recommended: 1)")
		}
	}

	// Decomposition checks
	if cfg.Decomposition.Enabled {
		if cfg.Decomposition.MaxSubtasks <= 0 {
			errs.Add("decomposition.max_subtasks", cfg.Decomposition.MaxSubtasks, "dependency", "must be positive when decomposition is enabled (recommended: 3-5)")
		}
		if cfg.Decomposition.MaxIterationBudget <= 0 {
			errs.Add("decomposition.max_iteration_budget", cfg.Decomposition.MaxIterationBudget, "dependency", "must be positive when decomposition is enabled (recommended: 20-50)")
		}
	}

	// Workspace limits validation
	if cfg.Workspace.Limits.MaxSizeMB > 0 && cfg.Workspace.Limits.MinFreeDiskMB > 0 {
		if cfg.Workspace.Limits.MinFreeDiskMB <= cfg.Workspace.Limits.MaxSizeMB {
			errs.Add("workspace.limits.min_free_disk_mb", 
				fmt.Sprintf("%d (max_size_mb: %d)", cfg.Workspace.Limits.MinFreeDiskMB, cfg.Workspace.Limits.MaxSizeMB), 
				"logical", 
				fmt.Sprintf("should be larger than max_size_mb to prevent disk space exhaustion (recommended: %d)", cfg.Workspace.Limits.MaxSizeMB*2))
		}
	}
}

// validateWorkspacePermissions checks workspace directory accessibility.
func validateWorkspacePermissions(cfg *Config, errs *ValidationErrors) {
	workspaceDir := cfg.Agents.Developer.WorkspaceDir
	if workspaceDir == "" {
		return
	}

	// Check if directory exists or can be created
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		errs.Add("agents.developer.workspace_dir", workspaceDir, "permissions", 
			fmt.Sprintf("cannot create directory: %v. Choose a writable directory path or fix permissions", err))
		return
	}

	// Check write permissions by creating a test file
	testFile := filepath.Join(workspaceDir, ".agentctl-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		errs.Add("agents.developer.workspace_dir", workspaceDir, "permissions", 
			fmt.Sprintf("directory is not writable: %v. Fix directory permissions or choose a different path", err))
	} else {
		// Clean up test file
		os.Remove(testFile)
	}

	// Check state directory as well
	stateDir := cfg.State.Dir
	if stateDir != "" {
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			errs.Add("state.dir", stateDir, "permissions", 
				fmt.Sprintf("cannot create state directory: %v. Choose a writable directory path or fix permissions", err))
		}
	}
}

// validateNumericRanges validates numeric configuration values are within acceptable ranges.
func validateNumericRanges(cfg *Config, errs *ValidationErrors) {
	// Max concurrent validation
	if cfg.Agents.Developer.MaxConcurrent > 50 {
		errs.Add("agents.developer.max_concurrent", cfg.Agents.Developer.MaxConcurrent, "range", "should not exceed 50 to prevent resource exhaustion")
	}

	// Claude max tokens validation
	if cfg.Claude.MaxTokens > 200000 {
		errs.Add("claude.max_tokens", cfg.Claude.MaxTokens, "range", "should not exceed 200000 per Anthropic limits")
	}

	// Workspace size validation
	if cfg.Workspace.Limits.MaxSizeMB > 10240 { // 10GB
		errs.Add("workspace.limits.max_size_mb", cfg.Workspace.Limits.MaxSizeMB, "range", "should not exceed 10240MB (10GB) to prevent excessive disk usage")
	}

	// Creativity thresholds validation
	if cfg.Creativity.IdleThresholdSeconds < 30 {
		errs.Add("creativity.idle_threshold_seconds", cfg.Creativity.IdleThresholdSeconds, "range", "should be at least 30 seconds to avoid excessive creativity triggers")
	}
	if cfg.Creativity.SuggestionCooldownSeconds < 60 {
		errs.Add("creativity.suggestion_cooldown_seconds", cfg.Creativity.SuggestionCooldownSeconds, "range", "should be at least 60 seconds to avoid spam")
	}

	// Decomposition limits validation
	if cfg.Decomposition.MaxSubtasks > 20 {
		errs.Add("decomposition.max_subtasks", cfg.Decomposition.MaxSubtasks, "range", "should not exceed 20 to prevent excessive complexity")
	}
	if cfg.Decomposition.MaxIterationBudget > 100 {
		errs.Add("decomposition.max_iteration_budget", cfg.Decomposition.MaxIterationBudget, "range", "should not exceed 100 to prevent runaway processes")
	}
}

// validateDurations validates duration configuration values.
func validateDurations(cfg *Config, errs *ValidationErrors) {
	// GitHub poll interval validation
	if cfg.GitHub.PollInterval < 5*time.Second {
		errs.Add("github.poll_interval", cfg.GitHub.PollInterval, "range", "should be at least 5 seconds to avoid API rate limits")
	}
	if cfg.GitHub.PollInterval > 1*time.Hour {
		errs.Add("github.poll_interval", cfg.GitHub.PollInterval, "range", "should not exceed 1 hour for reasonable responsiveness")
	}

	// Workspace cleanup retention validation
	if cfg.Workspace.Cleanup.SuccessRetention < 1*time.Hour {
		errs.Add("workspace.cleanup.success_retention", cfg.Workspace.Cleanup.SuccessRetention, "range", "should be at least 1 hour for debugging purposes")
	}
	if cfg.Workspace.Cleanup.FailureRetention < 1*time.Hour {
		errs.Add("workspace.cleanup.failure_retention", cfg.Workspace.Cleanup.FailureRetention, "range", "should be at least 1 hour for debugging purposes")
	}

	// Monitoring intervals validation
	if cfg.Workspace.Monitoring.DiskCheckInterval < 1*time.Minute {
		errs.Add("workspace.monitoring.disk_check_interval", cfg.Workspace.Monitoring.DiskCheckInterval, "range", "should be at least 1 minute to avoid excessive checking")
	}
}

// validateNetworkAccess performs runtime validation of external services.
func validateNetworkAccess(ctx context.Context, cfg *Config, errs *ValidationErrors) {
	// Create context with timeout for network operations
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Validate GitHub access
	validateGitHubAccess(ctx, &cfg.GitHub, errs)

	// Validate Claude access
	validateClaudeAccess(ctx, &cfg.Claude, errs)
}

// validateGitHubAccess checks GitHub token permissions and repository access.
func validateGitHubAccess(ctx context.Context, cfg *GitHubConfig, errs *ValidationErrors) {
	if cfg.Token == "" {
		return // Already handled in field validation
	}

	client := github.NewClient(nil).WithAuthToken(cfg.Token)

	// Check token scopes
	_, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		errs.Add("github.token", "", "network", fmt.Sprintf("token authentication failed: %v. Verify token is valid and not expired", err))
		return
	}

	// Check required scopes from response headers
	if scopes := resp.Header.Get("X-OAuth-Scopes"); scopes != "" {
		if !strings.Contains(scopes, "repo") && !strings.Contains(scopes, "public_repo") {
			errs.Add("github.token", "", "permissions", "token missing required 'repo' scope. Generate new token with 'repo' scope at https://github.com/settings/tokens")
		}
	}

	// Check repository access
	if cfg.Owner != "" && cfg.Repo != "" {
		_, _, err := client.Repositories.Get(ctx, cfg.Owner, cfg.Repo)
		if err != nil {
			errs.Add("github.owner/github.repo", fmt.Sprintf("%s/%s", cfg.Owner, cfg.Repo), "network", fmt.Sprintf("repository not accessible: %v. Verify repository exists and token has access", err))
		}
	}
}

// validateClaudeAccess checks Claude API key validity.
func validateClaudeAccess(ctx context.Context, cfg *ClaudeConfig, errs *ValidationErrors) {
	if cfg.APIKey == "" {
		return // Already handled in field validation
	}

	// Simple health check - make a basic HTTP request to Anthropic API
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/messages", nil)
	if err != nil {
		return // Skip validation if we can't create request
	}

	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Network errors are not config errors - warn but don't fail
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		errs.Add("claude.api_key", "", "network", "API key authentication failed. Verify API key is valid at https://console.anthropic.com/")
	}
}



// maskToken masks sensitive token values for error messages.
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}
