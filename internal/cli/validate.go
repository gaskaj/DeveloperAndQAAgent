package cli

import (
	"fmt"
	"os"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/spf13/cobra"
)

var (
	skipNetworkFlag  bool
	fullValidateFlag bool
	environmentFlag  string
	strictModeFlag   bool
)

func init() {
	validateCmd.Flags().BoolVar(&skipNetworkFlag, "skip-network", false, "Skip network-based validation (faster)")
	validateCmd.Flags().BoolVar(&fullValidateFlag, "full", false, "Enable comprehensive validation including network checks")
	validateCmd.Flags().StringVar(&environmentFlag, "env", "", "Validate for specific environment (dev, staging, prod)")
	validateCmd.Flags().BoolVar(&strictModeFlag, "strict", false, "Enable strict validation mode with warnings as errors")
	rootCmd.AddCommand(validateCmd)
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long: `Validate the configuration file for syntax errors, required fields, 
and optionally test connectivity to external services.

Examples:
  agentctl validate --config config.yaml                    # Basic validation
  agentctl validate --config config.yaml --full            # Full validation with network checks  
  agentctl validate --config config.yaml --skip-network    # Skip network validation (faster)
  agentctl validate --config config.yaml --env=prod        # Validate for production environment
  agentctl validate --config config.yaml --env=prod --strict  # Strict mode (warnings as errors)`,
	RunE: runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("--config flag is required")
	}

	fmt.Printf("Validating configuration: %s\n", cfgFile)

	// Check if file exists
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		fmt.Printf("❌ Configuration file not found: %s\n", cfgFile)
		return fmt.Errorf("config file not found: %s", cfgFile)
	}

	var cfg *config.Config
	var report *config.ValidationReport
	var err error

	// Load configuration with environment overlay if specified
	if environmentFlag != "" {
		fmt.Printf("🌍 Loading configuration for environment: %s\n", environmentFlag)
		cfg, err = config.LoadWithEnvironment(cfgFile, environmentFlag)
		if err != nil {
			fmt.Printf("❌ Environment configuration loading failed: %v\n", err)
			return err
		}

		// Use detailed validation reporting
		skipNetwork := skipNetworkFlag && !fullValidateFlag
		report = config.ValidateWithReport(cmd.Context(), cfg, skipNetwork)
	} else {
		// Choose validation method based on flags
		if fullValidateFlag {
			fmt.Println("🔍 Performing comprehensive validation with network checks...")
			cfg, err = config.LoadWithSchemaValidation(cfgFile)
			if err != nil {
				fmt.Println("❌ Configuration validation failed:")
				fmt.Printf("\n%s\n", formatValidationError(err))
				return err
			}

			// Get detailed report
			report = config.ValidateWithReport(cmd.Context(), cfg, false)
		} else {
			fmt.Println("🔍 Performing validation...")
			skipNetwork := skipNetworkFlag || !fullValidateFlag
			cfg, err = config.LoadWithOptions(cfgFile, skipNetwork)
			if err != nil {
				fmt.Println("❌ Configuration validation failed:")
				fmt.Printf("\n%s\n", formatValidationError(err))
				return err
			}

			// Get detailed report
			report = config.ValidateWithReport(cmd.Context(), cfg, skipNetwork)
		}
	}

	// Display validation results
	return displayValidationResults(cfg, report, environmentFlag, strictModeFlag)
}

func displayValidationResults(cfg *config.Config, report *config.ValidationReport, environment string, strict bool) error {
	// Check for errors first
	if report.ErrorCount > 0 {
		fmt.Println("❌ Configuration validation failed:")
		for _, result := range report.Failed {
			fmt.Printf("  • %s: %s\n", result.Rule.Field, result.Issue)
			if result.Fix != "" {
				fmt.Printf("    Fix: %s\n", result.Fix)
			}
			if result.Example != "" {
				fmt.Printf("    Example: %s\n", result.Example)
			}
		}
		return fmt.Errorf("configuration validation failed with %d errors", report.ErrorCount)
	}

	// Handle warnings
	if report.WarningCount > 0 {
		if strict {
			fmt.Println("❌ Configuration validation failed (strict mode - warnings treated as errors):")
			for _, result := range report.Warnings {
				fmt.Printf("  • %s: %s\n", result.Rule.Field, result.Issue)
				if result.Fix != "" {
					fmt.Printf("    Fix: %s\n", result.Fix)
				}
			}
			return fmt.Errorf("configuration validation failed with %d warnings in strict mode", report.WarningCount)
		} else {
			fmt.Println("⚠️  Configuration validation passed with warnings:")
			for _, result := range report.Warnings {
				fmt.Printf("  • %s: %s\n", result.Rule.Field, result.Issue)
				if result.Fix != "" {
					fmt.Printf("    Recommendation: %s\n", result.Fix)
				}
			}
		}
	}

	// Success message
	fmt.Println("✅ Configuration is valid!")

	// Environment info
	if environment != "" {
		fmt.Printf("🌍 Environment: %s\n", environment)
	}

	// Display validation summary
	fmt.Printf("📊 Validation Summary: %d rules checked, %d passed, %d warnings, %d errors\n",
		report.TotalRules, len(report.Passed), report.WarningCount, report.ErrorCount)

	// Display configuration summary
	fmt.Println("\n📋 Configuration Summary:")
	fmt.Printf("   GitHub Repository: %s/%s\n", cfg.GitHub.Owner, cfg.GitHub.Repo)
	fmt.Printf("   Claude Model: %s\n", cfg.Claude.Model)
	fmt.Printf("   Max Tokens: %d\n", cfg.Claude.MaxTokens)
	fmt.Printf("   Workspace Directory: %s\n", cfg.Agents.Developer.WorkspaceDir)
	fmt.Printf("   State Directory: %s\n", cfg.State.Dir)

	// Show agent status
	fmt.Printf("   Developer Agent: %s\n", enabledStatus(cfg.Agents.Developer.Enabled))
	fmt.Printf("   Creativity Mode: %s\n", enabledStatus(cfg.Creativity.Enabled))
	fmt.Printf("   Issue Decomposition: %s\n", enabledStatus(cfg.Decomposition.Enabled))

	// Show polling configuration
	fmt.Printf("   GitHub Poll Interval: %s\n", cfg.GitHub.PollInterval)
	if len(cfg.GitHub.WatchLabels) > 0 {
		fmt.Printf("   Watching Labels: %v\n", cfg.GitHub.WatchLabels)
	}

	// Environment variable check
	fmt.Println("\n🔐 Environment Variables:")
	checkEnvVar("GITHUB_TOKEN", cfg.GitHub.Token)
	checkEnvVar("ANTHROPIC_API_KEY", cfg.Claude.APIKey)

	// Network validation results
	networkValidated := false
	for _, result := range report.Passed {
		if result.Rule.Category == config.CategoryNetwork {
			networkValidated = true
			break
		}
	}
	for _, result := range report.Failed {
		if result.Rule.Category == config.CategoryNetwork {
			networkValidated = true
			break
		}
	}

	if networkValidated {
		fmt.Println("\n🌐 Network Connectivity: Validated")
	} else if skipNetworkFlag {
		fmt.Println("\n🌐 Network Connectivity: Skipped (use --full for network validation)")
	}

	fmt.Println("\n✅ Configuration validation completed successfully!")
	return nil
}

// formatValidationError formats validation errors in a user-friendly way.
func formatValidationError(err error) string {
	// If it's a structured validation error, format it nicely
	if validationErr, ok := err.(*config.ConfigValidationError); ok {
		return fmt.Sprintf("  • %s", validationErr.Error())
	}

	// Try to unwrap multiple errors using errors.Join result
	type multiError interface {
		Unwrap() []error
	}

	lines := []string{}
	if multiErr, ok := err.(multiError); ok {
		errs := multiErr.Unwrap()
		for _, e := range errs {
			if validationErr, ok := e.(*config.ConfigValidationError); ok {
				lines = append(lines, fmt.Sprintf("  • %s", validationErr.Error()))
			} else {
				lines = append(lines, fmt.Sprintf("  • %s", e.Error()))
			}
		}
		if len(lines) > 0 {
			return joinLines(lines)
		}
	}

	// Fallback to original error message
	return fmt.Sprintf("  %s", err.Error())
}

// joinLines joins error lines with newlines.
func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// enabledStatus returns a human-readable enabled/disabled status.
func enabledStatus(enabled bool) string {
	if enabled {
		return "✅ Enabled"
	}
	return "⭕ Disabled"
}

// checkEnvVar checks if an environment variable is properly set.
func checkEnvVar(name, value string) {
	if value == "" || value == "${"+name+"}" {
		fmt.Printf("   %s: ❌ Not set or not expanded\n", name)
	} else if len(value) < 8 {
		fmt.Printf("   %s: ⚠️ Set but appears too short\n", name)
	} else {
		// Mask the value for display
		masked := maskValue(value)
		fmt.Printf("   %s: ✅ Set (%s)\n", name, masked)
	}
}

// maskValue masks sensitive values for display.
func maskValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
