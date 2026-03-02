package config

import (
	"errors"
	"fmt"
	"time"
)

// Validate checks that all required configuration fields are set.
func Validate(cfg *Config) error {
	var errs []error

	if cfg.GitHub.Token == "" {
		errs = append(errs, fmt.Errorf("github.token is required"))
	}
	if cfg.GitHub.Owner == "" {
		errs = append(errs, fmt.Errorf("github.owner is required"))
	}
	if cfg.GitHub.Repo == "" {
		errs = append(errs, fmt.Errorf("github.repo is required"))
	}
	if cfg.Claude.APIKey == "" {
		errs = append(errs, fmt.Errorf("claude.api_key is required"))
	}
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = "claude-sonnet-4-20250514"
	}
	if cfg.Claude.MaxTokens == 0 {
		cfg.Claude.MaxTokens = 8192
	}
	if cfg.GitHub.PollInterval == 0 {
		cfg.GitHub.PollInterval = 30_000_000_000 // 30s in nanoseconds
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

	// Creativity defaults.
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

	// Decomposition defaults.
	if cfg.Decomposition.MaxIterationBudget == 0 {
		cfg.Decomposition.MaxIterationBudget = 25
	}
	if cfg.Decomposition.MaxSubtasks == 0 {
		cfg.Decomposition.MaxSubtasks = 5
	}

	// Workspace defaults.
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

	return errors.Join(errs...)
}
