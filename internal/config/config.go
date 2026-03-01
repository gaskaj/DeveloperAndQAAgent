package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	GitHub     GitHubConfig     `mapstructure:"github"`
	Claude     ClaudeConfig     `mapstructure:"claude"`
	Agents     AgentsConfig     `mapstructure:"agents"`
	State      StateConfig      `mapstructure:"state"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Creativity    CreativityConfig    `mapstructure:"creativity"`
	Decomposition DecompositionConfig `mapstructure:"decomposition"`
}

// GitHubConfig holds GitHub-related configuration.
type GitHubConfig struct {
	Token        string        `mapstructure:"token"`
	Owner        string        `mapstructure:"owner"`
	Repo         string        `mapstructure:"repo"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	WatchLabels  []string      `mapstructure:"watch_labels"`
}

// ClaudeConfig holds Claude AI configuration.
type ClaudeConfig struct {
	APIKey    string `mapstructure:"api_key"`
	Model     string `mapstructure:"model"`
	MaxTokens int    `mapstructure:"max_tokens"`
}

// AgentsConfig holds per-agent configuration.
type AgentsConfig struct {
	Developer DeveloperAgentConfig `mapstructure:"developer"`
}

// DeveloperAgentConfig holds developer agent settings.
type DeveloperAgentConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	MaxConcurrent int    `mapstructure:"max_concurrent"`
	WorkspaceDir  string `mapstructure:"workspace_dir"`
}

// StateConfig holds state storage configuration.
type StateConfig struct {
	Backend string `mapstructure:"backend"`
	Dir     string `mapstructure:"dir"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

// CreativityConfig holds configuration for the creativity engine.
type CreativityConfig struct {
	Enabled                   bool `mapstructure:"enabled"`
	IdleThresholdSeconds      int  `mapstructure:"idle_threshold_seconds"`
	SuggestionCooldownSeconds int  `mapstructure:"suggestion_cooldown_seconds"`
	MaxPendingSuggestions     int  `mapstructure:"max_pending_suggestions"`
	MaxRejectionHistory       int  `mapstructure:"max_rejection_history"`
}

// DecompositionConfig holds configuration for issue decomposition.
type DecompositionConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	MaxIterationBudget int  `mapstructure:"max_iteration_budget"`
	MaxSubtasks        int  `mapstructure:"max_subtasks"`
}

// Load reads configuration from the given file path, expanding environment variables.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables in all string values.
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.Contains(val, "${") {
			expanded := os.Expand(val, os.Getenv)
			v.Set(key, expanded)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}
