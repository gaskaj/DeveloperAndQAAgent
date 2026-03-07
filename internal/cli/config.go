package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  "Commands for validating, inspecting, and managing agentctl configuration.",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file without starting agents",
	Long: `Validate configuration file for syntax errors, required fields, and logical consistency.
This command performs comprehensive validation including:
- Required field presence
- Format validation (tokens, URLs, durations)
- Range validation (numeric limits, timeouts)
- Path accessibility (workspace directories)
- Optional network validation (API access)`,
	RunE: runConfigValidate,
}

var configShowDefaultsCmd = &cobra.Command{
	Use:   "show-defaults",
	Short: "Display all default configuration values",
	Long: `Display all default values used when configuration fields are not specified.
This is useful for understanding what values will be used for optional settings.`,
	RunE: runConfigShowDefaults,
}

var configEnvVarsCmd = &cobra.Command{
	Use:   "env-vars",
	Short: "Show environment variable mappings",
	Long: `Display the mapping between configuration keys and environment variables.
Environment variables can be used to override configuration file values.`,
	RunE: runConfigEnvVars,
}

var (
	skipNetworkValidation bool
	showPassedChecks      bool
	outputFormat          string
)

func init() {
	// Add config subcommands
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configShowDefaultsCmd)
	configCmd.AddCommand(configEnvVarsCmd)

	// Config validate flags
	configValidateCmd.Flags().BoolVar(&skipNetworkValidation, "skip-network", false, "skip network connectivity validation")
	configValidateCmd.Flags().BoolVar(&showPassedChecks, "show-passed", false, "show successful validation checks")

	// Config show-defaults flags
	configShowDefaultsCmd.Flags().StringVar(&outputFormat, "format", "yaml", "output format (yaml, json, table)")

	// Config env-vars flags
	configEnvVarsCmd.Flags().StringVar(&outputFormat, "format", "table", "output format (table, yaml, json)")

	// Register config command with root
	rootCmd.AddCommand(configCmd)
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file path is required (use --config flag)")
	}

	// Check if config file exists
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", cfgFile)
	}

	// Load configuration
	cfg, err := config.LoadWithOptions(cfgFile, skipNetworkValidation)
	if err != nil {
		fmt.Printf("❌ Configuration validation failed:\n%v\n", err)
		return err
	}

	// Success message
	fmt.Printf("✅ Configuration validation passed: %s\n", cfgFile)

	if showPassedChecks {
		printPassedValidations(cfg)
	}

	return nil
}

func runConfigShowDefaults(cmd *cobra.Command, args []string) error {
	defaults := config.GetDefaults()

	switch outputFormat {
	case "yaml":
		return printDefaultsYAML(defaults)
	case "json":
		return printDefaultsJSON(defaults)
	case "table":
		return printDefaultsTable(defaults)
	default:
		return fmt.Errorf("unsupported output format: %s (supported: yaml, json, table)", outputFormat)
	}
}

func runConfigEnvVars(cmd *cobra.Command, args []string) error {
	envMappings := getEnvironmentVariableMappings()

	switch outputFormat {
	case "table":
		return printEnvVarsTable(envMappings)
	case "yaml":
		return printEnvVarsYAML(envMappings)
	case "json":
		return printEnvVarsJSON(envMappings)
	default:
		return fmt.Errorf("unsupported output format: %s (supported: table, yaml, json)", outputFormat)
	}
}

func printPassedValidations(cfg *config.Config) {
	fmt.Println("\n✅ Passed validations:")

	// Required fields
	if cfg.GitHub.Token != "" {
		fmt.Println("  • github.token: present and properly formatted")
	}
	if cfg.GitHub.Owner != "" {
		fmt.Println("  • github.owner: present")
	}
	if cfg.GitHub.Repo != "" {
		fmt.Println("  • github.repo: present")
	}
	if cfg.Claude.APIKey != "" {
		fmt.Println("  • claude.api_key: present and properly formatted")
	}

	// Workspace directory
	if cfg.Agents.Developer.WorkspaceDir != "" {
		fmt.Printf("  • agents.developer.workspace_dir: accessible (%s)\n", cfg.Agents.Developer.WorkspaceDir)
	}

	// State directory
	if cfg.State.Dir != "" {
		fmt.Printf("  • state.dir: accessible (%s)\n", cfg.State.Dir)
	}

	// Concurrency settings
	if cfg.Agents.Developer.Enabled && cfg.Agents.Developer.MaxConcurrent > 0 {
		fmt.Printf("  • agents.developer.max_concurrent: valid (%d)\n", cfg.Agents.Developer.MaxConcurrent)
	}

	// Workspace limits
	if cfg.Workspace.Limits.MinFreeDiskMB > cfg.Workspace.Limits.MaxSizeMB {
		fmt.Printf("  • workspace.limits: disk space properly configured (min_free: %dMB, max_size: %dMB)\n",
			cfg.Workspace.Limits.MinFreeDiskMB, cfg.Workspace.Limits.MaxSizeMB)
	}
}

func printDefaultsYAML(defaults config.Defaults) error {
	fmt.Printf(`# Default Configuration Values
# Generated by: agentctl config show-defaults --format=yaml

github:
  poll_interval: %v
  watch_labels:
%s

claude:
  model: "%s"
  max_tokens: %d

agents:
  developer:
    enabled: %t
    max_concurrent: %d
    workspace_dir: "%s"
    recovery:
      enabled: %t
      startup_validation: %t
      auto_cleanup_orphaned: %t
      max_resume_age: %v
      validation_interval: %v

state:
  backend: "%s"
  dir: "%s"

logging:
  level: "%s"
  format: "%s"
  enable_correlation: %t

creativity:
  enabled: %t
  idle_threshold_seconds: %d
  suggestion_cooldown_seconds: %d
  max_pending_suggestions: %d
  max_rejection_history: %d

decomposition:
  enabled: %t
  max_iteration_budget: %d
  max_subtasks: %d

workspace:
  cleanup:
    enabled: %t
    success_retention: %v
    failure_retention: %v
    max_concurrent: %d
  limits:
    max_size_mb: %d
    min_free_disk_mb: %d
  monitoring:
    disk_check_interval: %v
    cleanup_interval: %v

shutdown:
  timeout: %v
  cleanup_workspaces: %t
  reset_claims: %t
`,
		defaults.GitHub.PollInterval,
		formatStringSliceYAML(defaults.GitHub.WatchLabels, "    - "),
		defaults.Claude.Model,
		defaults.Claude.MaxTokens,
		defaults.Agents.Developer.Enabled,
		defaults.Agents.Developer.MaxConcurrent,
		defaults.Agents.Developer.WorkspaceDir,
		defaults.Agents.Developer.Recovery.Enabled,
		defaults.Agents.Developer.Recovery.StartupValidation,
		defaults.Agents.Developer.Recovery.AutoCleanupOrphaned,
		defaults.Agents.Developer.Recovery.MaxResumeAge,
		defaults.Agents.Developer.Recovery.ValidationInterval,
		defaults.State.Backend,
		defaults.State.Dir,
		defaults.Logging.Level,
		defaults.Logging.Format,
		defaults.Logging.EnableCorrelation,
		defaults.Creativity.Enabled,
		defaults.Creativity.IdleThresholdSeconds,
		defaults.Creativity.SuggestionCooldownSeconds,
		defaults.Creativity.MaxPendingSuggestions,
		defaults.Creativity.MaxRejectionHistory,
		defaults.Decomposition.Enabled,
		defaults.Decomposition.MaxIterationBudget,
		defaults.Decomposition.MaxSubtasks,
		defaults.Workspace.Cleanup.Enabled,
		defaults.Workspace.Cleanup.SuccessRetention,
		defaults.Workspace.Cleanup.FailureRetention,
		defaults.Workspace.Cleanup.MaxConcurrent,
		defaults.Workspace.Limits.MaxSizeMB,
		defaults.Workspace.Limits.MinFreeDiskMB,
		defaults.Workspace.Monitoring.DiskCheckInterval,
		defaults.Workspace.Monitoring.CleanupInterval,
		defaults.Shutdown.Timeout,
		defaults.Shutdown.CleanupWorkspaces,
		defaults.Shutdown.ResetClaims,
	)

	return nil
}

func printDefaultsJSON(defaults config.Defaults) error {
	fmt.Printf(`{
  "github": {
    "poll_interval": "%v",
    "watch_labels": ["%s"]
  },
  "claude": {
    "model": "%s",
    "max_tokens": %d
  },
  "agents": {
    "developer": {
      "enabled": %t,
      "max_concurrent": %d,
      "workspace_dir": "%s"
    }
  },
  "state": {
    "backend": "%s",
    "dir": "%s"
  },
  "creativity": {
    "enabled": %t,
    "idle_threshold_seconds": %d,
    "max_pending_suggestions": %d
  },
  "workspace": {
    "limits": {
      "max_size_mb": %d,
      "min_free_disk_mb": %d
    }
  }
}`,
		defaults.GitHub.PollInterval,
		strings.Join(defaults.GitHub.WatchLabels, `", "`),
		defaults.Claude.Model,
		defaults.Claude.MaxTokens,
		defaults.Agents.Developer.Enabled,
		defaults.Agents.Developer.MaxConcurrent,
		defaults.Agents.Developer.WorkspaceDir,
		defaults.State.Backend,
		defaults.State.Dir,
		defaults.Creativity.Enabled,
		defaults.Creativity.IdleThresholdSeconds,
		defaults.Creativity.MaxPendingSuggestions,
		defaults.Workspace.Limits.MaxSizeMB,
		defaults.Workspace.Limits.MinFreeDiskMB,
	)

	return nil
}

func printDefaultsTable(defaults config.Defaults) error {
	fmt.Println("Configuration Defaults")
	fmt.Println("=====================")
	fmt.Println()

	categories := []struct {
		name    string
		entries [][]string
	}{
		{
			name: "GitHub Integration",
			entries: [][]string{
				{"github.poll_interval", fmt.Sprintf("%v", defaults.GitHub.PollInterval), "How often to check for new issues"},
				{"github.watch_labels", strings.Join(defaults.GitHub.WatchLabels, ", "), "Issue labels to monitor"},
			},
		},
		{
			name: "Claude AI",
			entries: [][]string{
				{"claude.model", defaults.Claude.Model, "Default Claude model"},
				{"claude.max_tokens", fmt.Sprintf("%d", defaults.Claude.MaxTokens), "Maximum tokens per request"},
			},
		},
		{
			name: "Developer Agent",
			entries: [][]string{
				{"agents.developer.enabled", fmt.Sprintf("%t", defaults.Agents.Developer.Enabled), "Enable developer agent"},
				{"agents.developer.max_concurrent", fmt.Sprintf("%d", defaults.Agents.Developer.MaxConcurrent), "Maximum concurrent workflows"},
				{"agents.developer.workspace_dir", defaults.Agents.Developer.WorkspaceDir, "Workspace directory path"},
			},
		},
		{
			name: "State Management",
			entries: [][]string{
				{"state.backend", defaults.State.Backend, "State storage backend"},
				{"state.dir", defaults.State.Dir, "State directory path"},
			},
		},
		{
			name: "Workspace Management",
			entries: [][]string{
				{"workspace.limits.max_size_mb", fmt.Sprintf("%d", defaults.Workspace.Limits.MaxSizeMB), "Maximum workspace size"},
				{"workspace.limits.min_free_disk_mb", fmt.Sprintf("%d", defaults.Workspace.Limits.MinFreeDiskMB), "Minimum free disk space"},
				{"workspace.cleanup.success_retention", fmt.Sprintf("%v", defaults.Workspace.Cleanup.SuccessRetention), "Keep successful workspaces"},
				{"workspace.cleanup.failure_retention", fmt.Sprintf("%v", defaults.Workspace.Cleanup.FailureRetention), "Keep failed workspaces"},
			},
		},
	}

	for _, category := range categories {
		fmt.Printf("%s:\n", category.name)
		for _, entry := range category.entries {
			fmt.Printf("  %-35s %-20s %s\n", entry[0], entry[1], entry[2])
		}
		fmt.Println()
	}

	return nil
}

type EnvVarMapping struct {
	ConfigKey string
	EnvVar    string
	Type      string
	Required  bool
	Example   string
}

func getEnvironmentVariableMappings() []EnvVarMapping {
	return []EnvVarMapping{
		{
			ConfigKey: "github.token",
			EnvVar:    "GITHUB_TOKEN",
			Type:      "string",
			Required:  true,
			Example:   "ghp_xxxxxxxxxxxxxxxxxxxx",
		},
		{
			ConfigKey: "github.owner",
			EnvVar:    "GITHUB_OWNER",
			Type:      "string",
			Required:  true,
			Example:   "myorg",
		},
		{
			ConfigKey: "github.repo",
			EnvVar:    "GITHUB_REPO",
			Type:      "string",
			Required:  true,
			Example:   "myrepo",
		},
		{
			ConfigKey: "claude.api_key",
			EnvVar:    "ANTHROPIC_API_KEY",
			Type:      "string",
			Required:  true,
			Example:   "sk-ant-api03-xxxxxxxxxxxx",
		},
		{
			ConfigKey: "claude.model",
			EnvVar:    "CLAUDE_MODEL",
			Type:      "string",
			Required:  false,
			Example:   "claude-sonnet-4-20250514",
		},
		{
			ConfigKey: "claude.max_tokens",
			EnvVar:    "CLAUDE_MAX_TOKENS",
			Type:      "int",
			Required:  false,
			Example:   "8192",
		},
		{
			ConfigKey: "agents.developer.workspace_dir",
			EnvVar:    "WORKSPACE_DIR",
			Type:      "string",
			Required:  false,
			Example:   "./workspaces",
		},
		{
			ConfigKey: "state.dir",
			EnvVar:    "STATE_DIR",
			Type:      "string",
			Required:  false,
			Example:   ".agentctl/state",
		},
		{
			ConfigKey: "logging.level",
			EnvVar:    "LOG_LEVEL",
			Type:      "string",
			Required:  false,
			Example:   "info",
		},
	}
}

func printEnvVarsTable(mappings []EnvVarMapping) error {
	fmt.Println("Environment Variable Mappings")
	fmt.Println("=============================")
	fmt.Println()
	fmt.Printf("%-35s %-25s %-8s %-8s %s\n", "CONFIG KEY", "ENVIRONMENT VARIABLE", "TYPE", "REQUIRED", "EXAMPLE")
	fmt.Println(strings.Repeat("-", 100))

	for _, mapping := range mappings {
		required := "No"
		if mapping.Required {
			required = "Yes"
		}
		fmt.Printf("%-35s %-25s %-8s %-8s %s\n",
			mapping.ConfigKey,
			mapping.EnvVar,
			mapping.Type,
			required,
			mapping.Example)
	}

	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  export GITHUB_TOKEN=your_token_here")
	fmt.Println("  export ANTHROPIC_API_KEY=your_key_here")
	fmt.Println("  agentctl start")

	return nil
}

func printEnvVarsYAML(mappings []EnvVarMapping) error {
	fmt.Println("# Environment Variable Mappings")
	fmt.Println("mappings:")
	for _, mapping := range mappings {
		fmt.Printf("  - config_key: %s\n", mapping.ConfigKey)
		fmt.Printf("    env_var: %s\n", mapping.EnvVar)
		fmt.Printf("    type: %s\n", mapping.Type)
		fmt.Printf("    required: %t\n", mapping.Required)
		fmt.Printf("    example: \"%s\"\n", mapping.Example)
		fmt.Println()
	}
	return nil
}

func printEnvVarsJSON(mappings []EnvVarMapping) error {
	fmt.Println("{")
	fmt.Println("  \"environment_variables\": [")
	for i, mapping := range mappings {
		fmt.Printf("    {\n")
		fmt.Printf("      \"config_key\": \"%s\",\n", mapping.ConfigKey)
		fmt.Printf("      \"env_var\": \"%s\",\n", mapping.EnvVar)
		fmt.Printf("      \"type\": \"%s\",\n", mapping.Type)
		fmt.Printf("      \"required\": %t,\n", mapping.Required)
		fmt.Printf("      \"example\": \"%s\"\n", mapping.Example)
		if i < len(mappings)-1 {
			fmt.Printf("    },\n")
		} else {
			fmt.Printf("    }\n")
		}
	}
	fmt.Println("  ]")
	fmt.Println("}")
	return nil
}

func formatStringSliceYAML(slice []string, indent string) string {
	if len(slice) == 0 {
		return ""
	}

	var result strings.Builder
	for _, item := range slice {
		result.WriteString(fmt.Sprintf("%s\"%s\"\n", indent, item))
	}

	return strings.TrimSuffix(result.String(), "\n")
}
