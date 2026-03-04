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

// ValidationRule defines a single validation rule with metadata
type ValidationRule struct {
	Name        string
	Field       string
	Check       func(*Config) *ValidationResult
	Level       ValidationLevel
	Category    ValidationCategory
	Description string
}

// ValidationLevel indicates the severity of validation issues
type ValidationLevel int

const (
	ValidationLevelError ValidationLevel = iota
	ValidationLevelWarning
	ValidationLevelInfo
)

// ValidationCategory groups related validation rules
type ValidationCategory int

const (
	CategoryRequired ValidationCategory = iota
	CategoryFormat
	CategoryPermissions
	CategoryNetwork
	CategoryLimits
	CategoryCompatibility
	CategorySecurity
	CategoryPerformance
)

// ValidationResult contains the outcome of a validation rule
type ValidationResult struct {
	Rule        *ValidationRule
	Passed      bool
	Value       interface{}
	Issue       string
	Fix         string
	Example     string
	Metadata    map[string]interface{}
}

// ValidationReport aggregates validation results
type ValidationReport struct {
	Passed       []*ValidationResult
	Failed       []*ValidationResult
	Warnings     []*ValidationResult
	TotalRules   int
	ErrorCount   int
	WarningCount int
}

// Validator provides comprehensive configuration validation
type Validator struct {
	rules       []*ValidationRule
	skipNetwork bool
}

// NewValidator creates a new configuration validator
func NewValidator() *Validator {
	v := &Validator{
		rules: make([]*ValidationRule, 0),
	}
	v.registerDefaultRules()
	return v
}

// WithSkipNetwork configures the validator to skip network-based validation
func (v *Validator) WithSkipNetwork(skip bool) *Validator {
	v.skipNetwork = skip
	return v
}

// ValidateConfig performs comprehensive validation and returns a detailed report
func (v *Validator) ValidateConfig(ctx context.Context, cfg *Config) *ValidationReport {
	report := &ValidationReport{
		Passed:     make([]*ValidationResult, 0),
		Failed:     make([]*ValidationResult, 0),
		Warnings:   make([]*ValidationResult, 0),
		TotalRules: len(v.rules),
	}

	for _, rule := range v.rules {
		// Skip network rules if requested
		if v.skipNetwork && rule.Category == CategoryNetwork {
			continue
		}

		result := rule.Check(cfg)
		result.Rule = rule

		if result.Passed {
			report.Passed = append(report.Passed, result)
		} else {
			switch rule.Level {
			case ValidationLevelError:
				report.Failed = append(report.Failed, result)
				report.ErrorCount++
			case ValidationLevelWarning:
				report.Warnings = append(report.Warnings, result)
				report.WarningCount++
			}
		}
	}

	return report
}

// ToError converts validation report to error for backward compatibility
func (r *ValidationReport) ToError() error {
	if r.ErrorCount == 0 {
		return nil
	}

	var errors []error
	for _, result := range r.Failed {
		err := &ConfigValidationError{
			Field:   result.Rule.Field,
			Value:   result.Value,
			Issue:   result.Issue,
			Fix:     result.Fix,
			Example: result.Example,
		}
		errors = append(errors, err)
	}

	if len(errors) == 1 {
		return errors[0]
	}

	// Join multiple errors
	var validationErrs ValidationErrors
	for _, err := range errors {
		if cve, ok := err.(*ConfigValidationError); ok {
			validationErrs.Add(cve.Field, cve.Value, cve.Issue, cve.Fix)
		}
	}
	return validationErrs.ToError()
}

// registerDefaultRules registers all built-in validation rules
func (v *Validator) registerDefaultRules() {
	// GitHub token validation
	v.addRule(&ValidationRule{
		Name:        "github_token_required",
		Field:       "github.token",
		Check:       v.checkGitHubTokenRequired,
		Level:       ValidationLevelError,
		Category:    CategoryRequired,
		Description: "GitHub token is required for repository access",
	})

	v.addRule(&ValidationRule{
		Name:        "github_token_format",
		Field:       "github.token",
		Check:       v.checkGitHubTokenFormat,
		Level:       ValidationLevelError,
		Category:    CategoryFormat,
		Description: "GitHub token must be a valid personal access token format",
	})

	// GitHub repository validation
	v.addRule(&ValidationRule{
		Name:        "github_owner_required",
		Field:       "github.owner",
		Check:       v.checkGitHubOwnerRequired,
		Level:       ValidationLevelError,
		Category:    CategoryRequired,
		Description: "GitHub repository owner is required",
	})

	v.addRule(&ValidationRule{
		Name:        "github_repo_required",
		Field:       "github.repo",
		Check:       v.checkGitHubRepoRequired,
		Level:       ValidationLevelError,
		Category:    CategoryRequired,
		Description: "GitHub repository name is required",
	})

	// Claude API validation
	v.addRule(&ValidationRule{
		Name:        "claude_api_key_required",
		Field:       "claude.api_key",
		Check:       v.checkClaudeAPIKeyRequired,
		Level:       ValidationLevelError,
		Category:    CategoryRequired,
		Description: "Claude API key is required for AI functionality",
	})

	v.addRule(&ValidationRule{
		Name:        "claude_api_key_format",
		Field:       "claude.api_key",
		Check:       v.checkClaudeAPIKeyFormat,
		Level:       ValidationLevelError,
		Category:    CategoryFormat,
		Description: "Claude API key must be a valid Anthropic API key format",
	})

	// Agent configuration validation
	v.addRule(&ValidationRule{
		Name:        "developer_agent_concurrency",
		Field:       "agents.developer.max_concurrent",
		Check:       v.checkDeveloperAgentConcurrency,
		Level:       ValidationLevelError,
		Category:    CategoryLimits,
		Description: "Developer agent max_concurrent must be positive when enabled",
	})

	// Workspace validation
	v.addRule(&ValidationRule{
		Name:        "workspace_permissions",
		Field:       "agents.developer.workspace_dir",
		Check:       v.checkWorkspacePermissions,
		Level:       ValidationLevelError,
		Category:    CategoryPermissions,
		Description: "Workspace directory must be writable",
	})

	// State directory validation
	v.addRule(&ValidationRule{
		Name:        "state_directory_permissions",
		Field:       "state.dir",
		Check:       v.checkStateDirectoryPermissions,
		Level:       ValidationLevelError,
		Category:    CategoryPermissions,
		Description: "State directory must be writable",
	})

	// Numeric range validations
	v.addRule(&ValidationRule{
		Name:        "claude_max_tokens_range",
		Field:       "claude.max_tokens",
		Check:       v.checkClaudeMaxTokensRange,
		Level:       ValidationLevelWarning,
		Category:    CategoryLimits,
		Description: "Claude max_tokens should be within reasonable limits",
	})

	v.addRule(&ValidationRule{
		Name:        "workspace_limits_logical",
		Field:       "workspace.limits",
		Check:       v.checkWorkspaceLimitsLogical,
		Level:       ValidationLevelWarning,
		Category:    CategoryLimits,
		Description: "Workspace limits should be logically consistent",
	})

	// Duration validations
	v.addRule(&ValidationRule{
		Name:        "github_poll_interval_range",
		Field:       "github.poll_interval",
		Check:       v.checkGitHubPollIntervalRange,
		Level:       ValidationLevelWarning,
		Category:    CategoryPerformance,
		Description: "GitHub poll interval should balance responsiveness and rate limits",
	})

	// Security validations
	v.addRule(&ValidationRule{
		Name:        "sensitive_data_exposure",
		Field:       "logging",
		Check:       v.checkSensitiveDataExposure,
		Level:       ValidationLevelWarning,
		Category:    CategorySecurity,
		Description: "Logging configuration should not expose sensitive data",
	})

	// Network validation rules (skipped if network validation is disabled)
	v.addRule(&ValidationRule{
		Name:        "github_api_connectivity",
		Field:       "github.token",
		Check:       v.checkGitHubAPIConnectivity,
		Level:       ValidationLevelError,
		Category:    CategoryNetwork,
		Description: "GitHub API must be accessible with provided token",
	})

	v.addRule(&ValidationRule{
		Name:        "claude_api_connectivity",
		Field:       "claude.api_key",
		Check:       v.checkClaudeAPIConnectivity,
		Level:       ValidationLevelError,
		Category:    CategoryNetwork,
		Description: "Claude API must be accessible with provided key",
	})

	// Compatibility validations
	v.addRule(&ValidationRule{
		Name:        "creativity_decomposition_compatibility",
		Field:       "creativity,decomposition",
		Check:       v.checkCreativityDecompositionCompatibility,
		Level:       ValidationLevelInfo,
		Category:    CategoryCompatibility,
		Description: "Creativity and decomposition settings compatibility check",
	})
}

// addRule adds a validation rule to the validator
func (v *Validator) addRule(rule *ValidationRule) {
	v.rules = append(v.rules, rule)
}

// Individual validation rule implementations

func (v *Validator) checkGitHubTokenRequired(cfg *Config) *ValidationResult {
	passed := cfg.GitHub.Token != ""
	result := &ValidationResult{
		Passed: passed,
		Value:  cfg.GitHub.Token,
	}

	if !passed {
		result.Issue = "required field is empty"
		result.Fix = "Set GITHUB_TOKEN environment variable or provide token directly"
		result.Example = "ghp_xxxxxxxxxxxxxxxxxxxx"
	}

	return result
}

func (v *Validator) checkGitHubTokenFormat(cfg *Config) *ValidationResult {
	if cfg.GitHub.Token == "" {
		return &ValidationResult{Passed: true} // Skip format check if empty (handled by required check)
	}

	isValidFormat := strings.HasPrefix(cfg.GitHub.Token, "ghp_") || 
		strings.HasPrefix(cfg.GitHub.Token, "github_pat_") ||
		strings.HasPrefix(cfg.GitHub.Token, "ghs_") // GitHub App token

	result := &ValidationResult{
		Passed: isValidFormat,
		Value:  maskToken(cfg.GitHub.Token),
	}

	if !isValidFormat {
		result.Issue = "token format appears invalid"
		result.Fix = "Use a personal access token from GitHub settings"
		result.Example = "ghp_xxxxxxxxxxxxxxxxxxxx"
	}

	return result
}

func (v *Validator) checkGitHubOwnerRequired(cfg *Config) *ValidationResult {
	passed := cfg.GitHub.Owner != ""
	result := &ValidationResult{
		Passed: passed,
		Value:  cfg.GitHub.Owner,
	}

	if !passed {
		result.Issue = "required field is empty"
		result.Fix = "Specify the GitHub repository owner/organization"
		result.Example = "myorg"
	}

	return result
}

func (v *Validator) checkGitHubRepoRequired(cfg *Config) *ValidationResult {
	passed := cfg.GitHub.Repo != ""
	result := &ValidationResult{
		Passed: passed,
		Value:  cfg.GitHub.Repo,
	}

	if !passed {
		result.Issue = "required field is empty"
		result.Fix = "Specify the GitHub repository name"
		result.Example = "myrepo"
	}

	return result
}

func (v *Validator) checkClaudeAPIKeyRequired(cfg *Config) *ValidationResult {
	passed := cfg.Claude.APIKey != ""
	result := &ValidationResult{
		Passed: passed,
		Value:  cfg.Claude.APIKey,
	}

	if !passed {
		result.Issue = "required field is empty"
		result.Fix = "Set ANTHROPIC_API_KEY environment variable or provide key directly"
		result.Example = "sk-ant-api03-xxxxxxxxxxxx"
	}

	return result
}

func (v *Validator) checkClaudeAPIKeyFormat(cfg *Config) *ValidationResult {
	if cfg.Claude.APIKey == "" {
		return &ValidationResult{Passed: true} // Skip format check if empty
	}

	isValidFormat := strings.HasPrefix(cfg.Claude.APIKey, "sk-ant-")
	result := &ValidationResult{
		Passed: isValidFormat,
		Value:  maskToken(cfg.Claude.APIKey),
	}

	if !isValidFormat {
		result.Issue = "API key format appears invalid"
		result.Fix = "Use an API key from Anthropic Console"
		result.Example = "sk-ant-api03-xxxxxxxxxxxx"
	}

	return result
}

func (v *Validator) checkDeveloperAgentConcurrency(cfg *Config) *ValidationResult {
	if !cfg.Agents.Developer.Enabled {
		return &ValidationResult{Passed: true} // Skip if agent disabled
	}

	passed := cfg.Agents.Developer.MaxConcurrent > 0
	result := &ValidationResult{
		Passed: passed,
		Value:  cfg.Agents.Developer.MaxConcurrent,
	}

	if !passed {
		result.Issue = "must be greater than 0 when developer agent is enabled"
		result.Fix = "Set to a positive integer (recommended: 1-3)"
		result.Example = "1"
	}

	return result
}

func (v *Validator) checkWorkspacePermissions(cfg *Config) *ValidationResult {
	workspaceDir := cfg.Agents.Developer.WorkspaceDir
	if workspaceDir == "" {
		return &ValidationResult{Passed: true} // Skip if not configured
	}

	// Try to create directory
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return &ValidationResult{
			Passed: false,
			Value:  workspaceDir,
			Issue:  fmt.Sprintf("cannot create directory: %v", err),
			Fix:    "Choose a writable directory path or fix permissions",
		}
	}

	// Test write permissions
	testFile := filepath.Join(workspaceDir, ".agentctl-validation-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return &ValidationResult{
			Passed: false,
			Value:  workspaceDir,
			Issue:  fmt.Sprintf("directory is not writable: %v", err),
			Fix:    "Fix directory permissions or choose a different path",
		}
	}

	// Clean up test file
	os.Remove(testFile)

	return &ValidationResult{
		Passed: true,
		Value:  workspaceDir,
	}
}

func (v *Validator) checkStateDirectoryPermissions(cfg *Config) *ValidationResult {
	stateDir := cfg.State.Dir
	if stateDir == "" {
		return &ValidationResult{Passed: true} // Will use default
	}

	// Try to create directory
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return &ValidationResult{
			Passed: false,
			Value:  stateDir,
			Issue:  fmt.Sprintf("cannot create state directory: %v", err),
			Fix:    "Choose a writable directory path or fix permissions",
		}
	}

	return &ValidationResult{
		Passed: true,
		Value:  stateDir,
	}
}

func (v *Validator) checkClaudeMaxTokensRange(cfg *Config) *ValidationResult {
	maxTokens := cfg.Claude.MaxTokens
	if maxTokens <= 0 {
		return &ValidationResult{Passed: true} // Will use default
	}

	passed := maxTokens <= 200000 // Anthropic's current limit
	result := &ValidationResult{
		Passed: passed,
		Value:  maxTokens,
	}

	if !passed {
		result.Issue = "exceeds Anthropic's maximum token limit"
		result.Fix = "Set to 200000 or lower per Anthropic limits"
		result.Example = "8192"
	}

	return result
}

func (v *Validator) checkWorkspaceLimitsLogical(cfg *Config) *ValidationResult {
	maxSize := cfg.Workspace.Limits.MaxSizeMB
	minFree := cfg.Workspace.Limits.MinFreeDiskMB

	if maxSize <= 0 || minFree <= 0 {
		return &ValidationResult{Passed: true} // Skip if not configured
	}

	passed := minFree > maxSize
	result := &ValidationResult{
		Passed: passed,
		Value:  fmt.Sprintf("max_size: %dMB, min_free: %dMB", maxSize, minFree),
	}

	if !passed {
		result.Issue = "min_free_disk_mb should be larger than max_size_mb to prevent disk exhaustion"
		result.Fix = fmt.Sprintf("Set min_free_disk_mb to at least %dMB", maxSize*2)
		result.Example = fmt.Sprintf("%d", maxSize*2)
	}

	return result
}

func (v *Validator) checkGitHubPollIntervalRange(cfg *Config) *ValidationResult {
	interval := cfg.GitHub.PollInterval
	if interval <= 0 {
		return &ValidationResult{Passed: true} // Will use default
	}

	tooFast := interval < 5*time.Second
	tooSlow := interval > 1*time.Hour

	result := &ValidationResult{
		Passed: !tooFast && !tooSlow,
		Value:  interval,
	}

	if tooFast {
		result.Issue = "poll interval too fast, may hit GitHub API rate limits"
		result.Fix = "Set to at least 5 seconds"
		result.Example = "30s"
	} else if tooSlow {
		result.Issue = "poll interval very slow, may impact responsiveness"
		result.Fix = "Consider setting to 1 hour or less"
		result.Example = "30s"
	}

	return result
}

func (v *Validator) checkSensitiveDataExposure(cfg *Config) *ValidationResult {
	// Check if structured logging might expose sensitive data
	if cfg.Logging.StructuredLogging.Enabled && 
	   cfg.Logging.Level == "debug" &&
	   cfg.Logging.StructuredLogging.IncludeStackTrace {
		
		return &ValidationResult{
			Passed: false,
			Value:  "debug level with stack traces in structured logging",
			Issue:  "debug logging with stack traces may expose sensitive information",
			Fix:    "Use 'info' level for production or disable stack traces",
			Example: "level: info",
		}
	}

	return &ValidationResult{Passed: true}
}

func (v *Validator) checkGitHubAPIConnectivity(cfg *Config) *ValidationResult {
	if cfg.GitHub.Token == "" {
		return &ValidationResult{Passed: true} // Skip if no token
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := github.NewClient(nil).WithAuthToken(cfg.GitHub.Token)

	// Test authentication
	_, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return &ValidationResult{
			Passed: false,
			Value:  maskToken(cfg.GitHub.Token),
			Issue:  fmt.Sprintf("GitHub API authentication failed: %v", err),
			Fix:    "Verify token is valid and not expired",
		}
	}

	// Check required scopes
	if scopes := resp.Header.Get("X-OAuth-Scopes"); scopes != "" {
		if !strings.Contains(scopes, "repo") && !strings.Contains(scopes, "public_repo") {
			return &ValidationResult{
				Passed: false,
				Value:  scopes,
				Issue:  "token missing required 'repo' scope",
				Fix:    "Generate new token with 'repo' scope at https://github.com/settings/tokens",
			}
		}
	}

	// Test repository access if specified
	if cfg.GitHub.Owner != "" && cfg.GitHub.Repo != "" {
		_, _, err := client.Repositories.Get(ctx, cfg.GitHub.Owner, cfg.GitHub.Repo)
		if err != nil {
			return &ValidationResult{
				Passed: false,
				Value:  fmt.Sprintf("%s/%s", cfg.GitHub.Owner, cfg.GitHub.Repo),
				Issue:  fmt.Sprintf("repository not accessible: %v", err),
				Fix:    "Verify repository exists and token has access",
			}
		}
	}

	return &ValidationResult{Passed: true}
}

func (v *Validator) checkClaudeAPIConnectivity(cfg *Config) *ValidationResult {
	if cfg.Claude.APIKey == "" {
		return &ValidationResult{Passed: true} // Skip if no key
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test API key with a simple request
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/messages", nil)
	if err != nil {
		return &ValidationResult{Passed: true} // Skip on request creation error
	}

	req.Header.Set("x-api-key", cfg.Claude.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Network errors don't fail validation - this is just a connectivity test
		return &ValidationResult{Passed: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return &ValidationResult{
			Passed: false,
			Value:  maskToken(cfg.Claude.APIKey),
			Issue:  "Claude API authentication failed",
			Fix:    "Verify API key is valid at https://console.anthropic.com/",
		}
	}

	return &ValidationResult{Passed: true}
}

func (v *Validator) checkCreativityDecompositionCompatibility(cfg *Config) *ValidationResult {
	// This is an informational check about optimal configuration
	if cfg.Creativity.Enabled && cfg.Decomposition.Enabled {
		if cfg.Decomposition.MaxIterationBudget < 50 {
			return &ValidationResult{
				Passed: false,
				Value:  cfg.Decomposition.MaxIterationBudget,
				Issue:  "low iteration budget may limit creativity effectiveness",
				Fix:    "Consider increasing max_iteration_budget to 50+ for creative workflows",
				Example: "50",
			}
		}
	}

	return &ValidationResult{Passed: true}
}