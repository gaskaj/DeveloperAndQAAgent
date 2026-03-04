package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// EnvironmentType represents different deployment environments
type EnvironmentType int

const (
	EnvironmentDevelopment EnvironmentType = iota
	EnvironmentStaging
	EnvironmentProduction
	EnvironmentTest
)

func (e EnvironmentType) String() string {
	switch e {
	case EnvironmentDevelopment:
		return "development"
	case EnvironmentStaging:
		return "staging"
	case EnvironmentProduction:
		return "production"
	case EnvironmentTest:
		return "test"
	default:
		return "unknown"
	}
}

// ParseEnvironmentType parses an environment string
func ParseEnvironmentType(env string) (EnvironmentType, error) {
	switch strings.ToLower(env) {
	case "dev", "development", "develop":
		return EnvironmentDevelopment, nil
	case "stage", "staging":
		return EnvironmentStaging, nil
	case "prod", "production":
		return EnvironmentProduction, nil
	case "test", "testing":
		return EnvironmentTest, nil
	default:
		return EnvironmentDevelopment, fmt.Errorf("unknown environment: %s", env)
	}
}

// EnvironmentConfig holds environment-specific configuration overrides
type EnvironmentConfig struct {
	Name                     string
	Type                     EnvironmentType
	ConfigFiles              []string
	RequiredEnvVars          []string
	ValidationRules          []EnvironmentValidationRule
	AllowedFeatures          []string
	DisallowedFeatures       []string
	RequiredSecurityFeatures []string
}

// EnvironmentValidationRule defines environment-specific validation rules
type EnvironmentValidationRule struct {
	Field         string
	Required      bool
	AllowedValues []string
	MinValue      interface{}
	MaxValue      interface{}
	Description   string
}

// EnvironmentManager handles environment-specific configuration loading and validation
type EnvironmentManager struct {
	environments map[string]*EnvironmentConfig
}

// NewEnvironmentManager creates a new environment manager
func NewEnvironmentManager() *EnvironmentManager {
	em := &EnvironmentManager{
		environments: make(map[string]*EnvironmentConfig),
	}
	em.registerDefaultEnvironments()
	return em
}

// LoadEnvironmentConfig loads configuration with environment-specific overlays
func (em *EnvironmentManager) LoadEnvironmentConfig(basePath, environment string) (*Config, error) {
	env, err := ParseEnvironmentType(environment)
	if err != nil {
		return nil, fmt.Errorf("parsing environment type: %w", err)
	}

	envConfig, exists := em.environments[env.String()]
	if !exists {
		return nil, fmt.Errorf("no configuration defined for environment: %s", environment)
	}

	v := viper.New()
	v.SetConfigType("yaml")

	// Load base configuration
	v.SetConfigFile(basePath)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("loading base config %q: %w", basePath, err)
	}

	// Load environment-specific overlays
	for _, overlay := range envConfig.ConfigFiles {
		overlayPath := em.resolveOverlayPath(basePath, overlay)
		if _, err := os.Stat(overlayPath); err == nil {
			v.SetConfigFile(overlayPath)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("merging environment config %q: %w", overlayPath, err)
			}
		}
	}

	// Set up automatic environment variable binding
	em.bindEnvironmentVariables(v)

	// Expand environment variables
	em.expandEnvironmentVariables(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Apply environment-specific validation
	if err := em.validateEnvironmentConfig(&cfg, envConfig); err != nil {
		return nil, fmt.Errorf("environment validation failed: %w", err)
	}

	// Apply defaults after environment validation
	ApplyDefaults(&cfg)

	return &cfg, nil
}

// registerDefaultEnvironments sets up the default environment configurations
func (em *EnvironmentManager) registerDefaultEnvironments() {
	// Development environment
	em.environments["development"] = &EnvironmentConfig{
		Name: "development",
		Type: EnvironmentDevelopment,
		ConfigFiles: []string{
			"config.dev.yaml",
			"config.local.yaml", // For local overrides
		},
		RequiredEnvVars: []string{
			"GITHUB_TOKEN",
			"ANTHROPIC_API_KEY",
		},
		ValidationRules: []EnvironmentValidationRule{
			{
				Field:         "logging.level",
				AllowedValues: []string{"debug", "info", "warn"},
				Description:   "Development should use debug or info logging",
			},
			{
				Field:       "agents.developer.max_concurrent",
				MinValue:    1,
				MaxValue:    3,
				Description: "Development should use limited concurrency",
			},
		},
		AllowedFeatures:          []string{"creativity", "decomposition", "debug_logging"},
		RequiredSecurityFeatures: []string{}, // Relaxed for development
	}

	// Staging environment
	em.environments["staging"] = &EnvironmentConfig{
		Name: "staging",
		Type: EnvironmentStaging,
		ConfigFiles: []string{
			"config.staging.yaml",
		},
		RequiredEnvVars: []string{
			"GITHUB_TOKEN",
			"ANTHROPIC_API_KEY",
		},
		ValidationRules: []EnvironmentValidationRule{
			{
				Field:         "logging.level",
				AllowedValues: []string{"info", "warn", "error"},
				Description:   "Staging should use info or higher logging",
			},
			{
				Field:       "agents.developer.max_concurrent",
				MinValue:    1,
				MaxValue:    5,
				Description: "Staging can handle moderate concurrency",
			},
			{
				Field:       "metrics.enabled",
				Required:    true,
				Description: "Staging must have metrics enabled",
			},
		},
		AllowedFeatures:          []string{"creativity", "decomposition", "metrics"},
		RequiredSecurityFeatures: []string{"correlation_logging"},
	}

	// Production environment
	em.environments["production"] = &EnvironmentConfig{
		Name: "production",
		Type: EnvironmentProduction,
		ConfigFiles: []string{
			"config.prod.yaml",
		},
		RequiredEnvVars: []string{
			"GITHUB_TOKEN",
			"ANTHROPIC_API_KEY",
		},
		ValidationRules: []EnvironmentValidationRule{
			{
				Field:         "logging.level",
				AllowedValues: []string{"info", "warn", "error"},
				Description:   "Production must not use debug logging",
			},
			{
				Field:       "agents.developer.max_concurrent",
				MinValue:    1,
				MaxValue:    10,
				Description: "Production can use higher concurrency",
			},
			{
				Field:       "metrics.enabled",
				Required:    true,
				Description: "Production must have metrics enabled",
			},
			{
				Field:       "logging.enable_correlation",
				Required:    true,
				Description: "Production must enable correlation logging",
			},
			{
				Field:       "error_handling.retry.enabled",
				Required:    true,
				Description: "Production must have retry enabled",
			},
			{
				Field:       "error_handling.circuit_breaker.enabled",
				Required:    true,
				Description: "Production must have circuit breakers enabled",
			},
		},
		AllowedFeatures:    []string{"metrics", "retry", "circuit_breaker", "observability"},
		DisallowedFeatures: []string{"debug_logging"},
		RequiredSecurityFeatures: []string{
			"correlation_logging",
			"structured_logging",
			"error_handling",
		},
	}

	// Test environment
	em.environments["test"] = &EnvironmentConfig{
		Name: "test",
		Type: EnvironmentTest,
		ConfigFiles: []string{
			"config.test.yaml",
		},
		RequiredEnvVars: []string{}, // Tests may use mock credentials
		ValidationRules: []EnvironmentValidationRule{
			{
				Field:         "logging.level",
				AllowedValues: []string{"debug", "info", "warn", "error"},
				Description:   "Test environment allows all log levels",
			},
			{
				Field:       "agents.developer.max_concurrent",
				MinValue:    1,
				MaxValue:    2,
				Description: "Test should use minimal concurrency",
			},
		},
		AllowedFeatures:          []string{"creativity", "decomposition", "debug_logging", "mock_apis"},
		RequiredSecurityFeatures: []string{}, // Relaxed for testing
	}
}

// resolveOverlayPath resolves the full path for an environment overlay file
func (em *EnvironmentManager) resolveOverlayPath(basePath, overlay string) string {
	baseDir := filepath.Dir(basePath)
	return filepath.Join(baseDir, overlay)
}

// bindEnvironmentVariables sets up automatic environment variable binding
func (em *EnvironmentManager) bindEnvironmentVariables(v *viper.Viper) {
	// Set up prefix for environment variables
	v.SetEnvPrefix("AGENT")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind specific environment variables to configuration keys
	envBindings := map[string]string{
		"GITHUB_TOKEN":          "github.token",
		"GITHUB_OWNER":          "github.owner",
		"GITHUB_REPO":           "github.repo",
		"ANTHROPIC_API_KEY":     "claude.api_key",
		"CLAUDE_MODEL":          "claude.model",
		"CLAUDE_MAX_TOKENS":     "claude.max_tokens",
		"WORKSPACE_DIR":         "agents.developer.workspace_dir",
		"STATE_DIR":             "state.dir",
		"LOG_LEVEL":             "logging.level",
		"LOG_FORMAT":            "logging.format",
		"LOG_FILE":              "logging.file_path",
		"METRICS_ENABLED":       "metrics.enabled",
		"POLL_INTERVAL":         "github.poll_interval",
		"MAX_CONCURRENT":        "agents.developer.max_concurrent",
		"CREATIVITY_ENABLED":    "creativity.enabled",
		"DECOMPOSITION_ENABLED": "decomposition.enabled",
	}

	for envVar, configKey := range envBindings {
		v.BindEnv(configKey, envVar)
	}
}

// expandEnvironmentVariables expands ${VAR} syntax in configuration values
func (em *EnvironmentManager) expandEnvironmentVariables(v *viper.Viper) {
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.Contains(val, "${") {
			expanded := os.ExpandEnv(val)
			v.Set(key, expanded)
		}
	}
}

// validateEnvironmentConfig validates configuration against environment-specific rules
func (em *EnvironmentManager) validateEnvironmentConfig(cfg *Config, envConfig *EnvironmentConfig) error {
	var errors []error

	// Check required environment variables
	for _, envVar := range envConfig.RequiredEnvVars {
		if value := os.Getenv(envVar); value == "" {
			errors = append(errors, fmt.Errorf("required environment variable %s is not set for %s environment", envVar, envConfig.Name))
		}
	}

	// Validate environment-specific rules
	for _, rule := range envConfig.ValidationRules {
		if err := em.validateRule(cfg, rule); err != nil {
			errors = append(errors, fmt.Errorf("environment validation rule '%s': %w", rule.Field, err))
		}
	}

	// Check required security features for production
	if envConfig.Type == EnvironmentProduction {
		if err := em.validateSecurityRequirements(cfg, envConfig); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("environment validation failed with %d errors: %v", len(errors), errors)
	}

	return nil
}

// validateRule validates a specific environment rule against configuration
func (em *EnvironmentManager) validateRule(cfg *Config, rule EnvironmentValidationRule) error {
	value := em.getConfigValue(cfg, rule.Field)

	if rule.Required && em.isEmptyValue(value) {
		return fmt.Errorf("field %s is required in this environment", rule.Field)
	}

	if len(rule.AllowedValues) > 0 {
		stringVal := fmt.Sprintf("%v", value)
		found := false
		for _, allowed := range rule.AllowedValues {
			if stringVal == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field %s has value %v, must be one of: %v", rule.Field, value, rule.AllowedValues)
		}
	}

	// Add more validation logic for min/max values as needed

	return nil
}

// validateSecurityRequirements ensures production security requirements are met
func (em *EnvironmentManager) validateSecurityRequirements(cfg *Config, envConfig *EnvironmentConfig) error {
	var errors []error

	for _, feature := range envConfig.RequiredSecurityFeatures {
		switch feature {
		case "correlation_logging":
			if !cfg.Logging.EnableCorrelation {
				errors = append(errors, fmt.Errorf("correlation logging is required in %s environment", envConfig.Name))
			}
		case "structured_logging":
			if !cfg.Logging.StructuredLogging.Enabled {
				errors = append(errors, fmt.Errorf("structured logging is required in %s environment", envConfig.Name))
			}
		case "error_handling":
			if !cfg.ErrorHandling.Retry.Enabled || !cfg.ErrorHandling.CircuitBreaker.Enabled {
				errors = append(errors, fmt.Errorf("error handling (retry and circuit breaker) is required in %s environment", envConfig.Name))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("security validation failed: %v", errors)
	}

	return nil
}

// getConfigValue retrieves a configuration value by field path
func (em *EnvironmentManager) getConfigValue(cfg *Config, fieldPath string) interface{} {
	parts := strings.Split(fieldPath, ".")

	switch parts[0] {
	case "logging":
		if len(parts) < 2 {
			return nil
		}
		switch parts[1] {
		case "level":
			return cfg.Logging.Level
		case "format":
			return cfg.Logging.Format
		case "enable_correlation":
			return cfg.Logging.EnableCorrelation
		}
	case "agents":
		if len(parts) < 3 {
			return nil
		}
		if parts[1] == "developer" {
			switch parts[2] {
			case "max_concurrent":
				return cfg.Agents.Developer.MaxConcurrent
			case "enabled":
				return cfg.Agents.Developer.Enabled
			}
		}
	case "metrics":
		if len(parts) < 2 {
			return nil
		}
		switch parts[1] {
		case "enabled":
			return cfg.Metrics.Enabled
		}
	case "error_handling":
		if len(parts) < 3 {
			return nil
		}
		switch parts[1] {
		case "retry":
			if parts[2] == "enabled" {
				return cfg.ErrorHandling.Retry.Enabled
			}
		case "circuit_breaker":
			if parts[2] == "enabled" {
				return cfg.ErrorHandling.CircuitBreaker.Enabled
			}
		}
	}

	return nil
}

// isEmptyValue checks if a value is considered empty
func (em *EnvironmentManager) isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return v == ""
	case bool:
		return false // false is a valid value for boolean fields
	case int:
		return v == 0
	case float64:
		return v == 0.0
	default:
		return false
	}
}

// GetEnvironmentInfo returns information about available environments
func (em *EnvironmentManager) GetEnvironmentInfo() map[string]interface{} {
	info := make(map[string]interface{})

	for name, env := range em.environments {
		info[name] = map[string]interface{}{
			"type":                       env.Type.String(),
			"config_files":               env.ConfigFiles,
			"required_env_vars":          env.RequiredEnvVars,
			"allowed_features":           env.AllowedFeatures,
			"disallowed_features":        env.DisallowedFeatures,
			"required_security_features": env.RequiredSecurityFeatures,
			"validation_rules_count":     len(env.ValidationRules),
		}
	}

	return info
}

// ValidateEnvironmentOverride validates that environment-specific overrides are safe
func (em *EnvironmentManager) ValidateEnvironmentOverride(environment string, overrides map[string]interface{}) error {
	env, err := ParseEnvironmentType(environment)
	if err != nil {
		return err
	}

	envConfig, exists := em.environments[env.String()]
	if !exists {
		return fmt.Errorf("unknown environment: %s", environment)
	}

	// Check that overrides don't violate environment constraints
	for key, value := range overrides {
		// Check if this feature is disallowed in this environment
		for _, disallowed := range envConfig.DisallowedFeatures {
			if strings.Contains(key, disallowed) {
				return fmt.Errorf("override %s contains disallowed feature %s for %s environment", key, disallowed, environment)
			}
		}

		// Validate against environment rules
		for _, rule := range envConfig.ValidationRules {
			if rule.Field == key {
				if len(rule.AllowedValues) > 0 {
					stringVal := fmt.Sprintf("%v", value)
					found := false
					for _, allowed := range rule.AllowedValues {
						if stringVal == allowed {
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("override %s has value %v, must be one of: %v", key, value, rule.AllowedValues)
					}
				}
			}
		}
	}

	return nil
}
