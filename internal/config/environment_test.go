package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentManager(t *testing.T) {
	// Test environment parsing
	tests := []struct {
		input    string
		expected EnvironmentType
		hasError bool
	}{
		{"dev", EnvironmentDevelopment, false},
		{"development", EnvironmentDevelopment, false},
		{"staging", EnvironmentStaging, false},
		{"prod", EnvironmentProduction, false},
		{"production", EnvironmentProduction, false},
		{"test", EnvironmentTest, false},
		{"invalid", EnvironmentDevelopment, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			env, err := ParseEnvironmentType(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, env)
			}
		})
	}
}

func TestEnvironmentConfigLoading(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create base config
	baseConfig := `
github:
  token: ${GITHUB_TOKEN}
  owner: testowner
  repo: testrepo
  poll_interval: 30s

claude:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-sonnet-4-20250514

agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: info
  enable_correlation: false

metrics:
  enabled: false
`
	
	basePath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(basePath, []byte(baseConfig), 0644)
	require.NoError(t, err)
	
	// Create development overlay
	devOverlay := `
logging:
  level: debug
  enable_correlation: true

agents:
  developer:
    max_concurrent: 1

creativity:
  enabled: true
  idle_threshold_seconds: 60

metrics:
  enabled: true
  collection_interval: 15s
`
	
	devPath := filepath.Join(tempDir, "config.dev.yaml")
	err = os.WriteFile(devPath, []byte(devOverlay), 0644)
	require.NoError(t, err)
	
	// Create production overlay
	prodOverlay := `
logging:
  level: info
  enable_correlation: true
  structured_logging:
    enabled: true

agents:
  developer:
    max_concurrent: 3

creativity:
  enabled: false

metrics:
  enabled: true
  collection_interval: 60s

error_handling:
  retry:
    enabled: true
  circuit_breaker:
    enabled: true
`
	
	prodPath := filepath.Join(tempDir, "config.prod.yaml")
	err = os.WriteFile(prodPath, []byte(prodOverlay), 0644)
	require.NoError(t, err)
	
	// Set required environment variables
	os.Setenv("GITHUB_TOKEN", "ghp_test_token_1234567890")
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-test_key_1234567890")
	defer func() {
		os.Unsetenv("GITHUB_TOKEN")
		os.Unsetenv("ANTHROPIC_API_KEY")
	}()
	
	em := NewEnvironmentManager()
	
	// Test development environment loading
	t.Run("development_environment", func(t *testing.T) {
		cfg, err := em.LoadEnvironmentConfig(basePath, "development")
		require.NoError(t, err)
		
		// Verify base config is loaded
		assert.Equal(t, "testowner", cfg.GitHub.Owner)
		assert.Equal(t, "testrepo", cfg.GitHub.Repo)
		assert.Equal(t, "ghp_test_token_1234567890", cfg.GitHub.Token)
		assert.Equal(t, "sk-ant-api03-test_key_1234567890", cfg.Claude.APIKey)
		
		// Verify environment overlay is applied
		assert.Equal(t, "debug", cfg.Logging.Level)
		assert.True(t, cfg.Logging.EnableCorrelation)
		assert.True(t, cfg.Creativity.Enabled)
		assert.Equal(t, 60, cfg.Creativity.IdleThresholdSeconds)
		assert.True(t, cfg.Metrics.Enabled)
	})
	
	// Test production environment loading
	t.Run("production_environment", func(t *testing.T) {
		cfg, err := em.LoadEnvironmentConfig(basePath, "production")
		require.NoError(t, err)
		
		// Verify base config
		assert.Equal(t, "testowner", cfg.GitHub.Owner)
		assert.Equal(t, "testrepo", cfg.GitHub.Repo)
		
		// Verify production overlay
		assert.Equal(t, "info", cfg.Logging.Level)
		assert.True(t, cfg.Logging.EnableCorrelation)
		assert.True(t, cfg.Logging.StructuredLogging.Enabled)
		assert.Equal(t, 3, cfg.Agents.Developer.MaxConcurrent)
		assert.False(t, cfg.Creativity.Enabled)
		assert.True(t, cfg.Metrics.Enabled)
		assert.True(t, cfg.ErrorHandling.Retry.Enabled)
		assert.True(t, cfg.ErrorHandling.CircuitBreaker.Enabled)
	})
}

func TestEnvironmentValidation(t *testing.T) {
	em := NewEnvironmentManager()
	tempDir := t.TempDir()
	
	// Create a minimal config for production
	prodConfig := `
github:
  token: ghp_test_token_1234567890
  owner: testowner
  repo: testrepo

claude:
  api_key: sk-ant-api03-test_key_1234567890

agents:
  developer:
    enabled: true
    max_concurrent: 2
    workspace_dir: ` + t.TempDir() + `

state:
  dir: ` + t.TempDir() + `

logging:
  level: debug  # This should fail production validation
  enable_correlation: false  # This should fail production validation

metrics:
  enabled: false  # This should fail production validation

error_handling:
  retry:
    enabled: false  # This should fail production validation
  circuit_breaker:
    enabled: false  # This should fail production validation
`
	
	basePath := filepath.Join(tempDir, "prod-config.yaml")
	err := os.WriteFile(basePath, []byte(prodConfig), 0644)
	require.NoError(t, err)
	
	// Set required environment variables for test
	os.Setenv("GITHUB_TOKEN", "ghp_test_token_1234567890")
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-test_key_1234567890")
	defer func() {
		os.Unsetenv("GITHUB_TOKEN")
		os.Unsetenv("ANTHROPIC_API_KEY")
	}()

	// This should fail validation for production
	t.Run("production_validation_failures", func(t *testing.T) {
		_, err := em.LoadEnvironmentConfig(basePath, "production")
		assert.Error(t, err, "Production config with debug logging should fail validation")
		assert.Contains(t, err.Error(), "environment validation")
	})
	
	// Test development environment (should be more permissive)
	t.Run("development_validation_passes", func(t *testing.T) {
		_, err := em.LoadEnvironmentConfig(basePath, "development")
		assert.NoError(t, err, "Development should allow debug logging")
	})
}

func TestEnvironmentOverrideValidation(t *testing.T) {
	em := NewEnvironmentManager()
	
	tests := []struct {
		name        string
		environment string
		overrides   map[string]interface{}
		expectError bool
		errorText   string
	}{
		{
			name:        "valid_dev_override",
			environment: "development",
			overrides: map[string]interface{}{
				"logging.level": "debug",
				"creativity.enabled": true,
			},
			expectError: false,
		},
		{
			name:        "invalid_prod_debug_logging",
			environment: "production",
			overrides: map[string]interface{}{
				"logging.level": "debug",
			},
			expectError: true,
			errorText:   "must be one of",
		},
		{
			name:        "disallowed_feature_in_prod",
			environment: "production",
			overrides: map[string]interface{}{
				"debug_logging": true,
			},
			expectError: true,
			errorText:   "disallowed feature",
		},
		{
			name:        "valid_prod_override",
			environment: "production",
			overrides: map[string]interface{}{
				"logging.level": "info",
				"metrics.enabled": true,
			},
			expectError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := em.ValidateEnvironmentOverride(tt.environment, tt.overrides)
			
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnvironmentInfo(t *testing.T) {
	em := NewEnvironmentManager()
	info := em.GetEnvironmentInfo()
	
	// Should have all default environments
	expectedEnvs := []string{"development", "staging", "production", "test"}
	for _, env := range expectedEnvs {
		assert.Contains(t, info, env, "Environment %s should be registered", env)
	}
	
	// Check development environment details
	devInfo, ok := info["development"].(map[string]interface{})
	require.True(t, ok)
	
	assert.Equal(t, "development", devInfo["type"])
	assert.Contains(t, devInfo["config_files"], "config.dev.yaml")
	assert.Contains(t, devInfo["required_env_vars"], "GITHUB_TOKEN")
	assert.Contains(t, devInfo["required_env_vars"], "ANTHROPIC_API_KEY")
	assert.Contains(t, devInfo["allowed_features"], "creativity")
	assert.Contains(t, devInfo["allowed_features"], "debug_logging")
	
	// Check production environment details
	prodInfo, ok := info["production"].(map[string]interface{})
	require.True(t, ok)
	
	assert.Equal(t, "production", prodInfo["type"])
	assert.Contains(t, prodInfo["config_files"], "config.prod.yaml")
	assert.Contains(t, prodInfo["required_security_features"], "correlation_logging")
	assert.Contains(t, prodInfo["required_security_features"], "structured_logging")
	assert.Contains(t, prodInfo["disallowed_features"], "debug_logging")
}

func TestGetConfigValue(t *testing.T) {
	em := NewEnvironmentManager()
	
	cfg := &Config{
		Logging: LoggingConfig{
			Level:             "debug",
			EnableCorrelation: true,
		},
		Agents: AgentsConfig{
			Developer: DeveloperAgentConfig{
				MaxConcurrent: 3,
				Enabled:       true,
			},
		},
		Metrics: MetricsConfig{
			Enabled: true,
		},
		ErrorHandling: ErrorHandlingConfig{
			Retry: RetryConfig{
				Enabled: true,
			},
			CircuitBreaker: CircuitBreakerGroupConfig{
				Enabled: false,
			},
		},
	}
	
	tests := []struct {
		field    string
		expected interface{}
	}{
		{"logging.level", "debug"},
		{"logging.enable_correlation", true},
		{"agents.developer.max_concurrent", 3},
		{"agents.developer.enabled", true},
		{"metrics.enabled", true},
		{"error_handling.retry.enabled", true},
		{"error_handling.circuit_breaker.enabled", false},
		{"nonexistent.field", nil},
	}
	
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value := em.getConfigValue(cfg, tt.field)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestIsEmptyValue(t *testing.T) {
	em := NewEnvironmentManager()
	
	tests := []struct {
		value    interface{}
		expected bool
	}{
		{nil, true},
		{"", true},
		{"non-empty", false},
		{0, true},
		{1, false},
		{0.0, true},
		{1.5, false},
		{false, false}, // false is a valid boolean value
		{true, false},
	}
	
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			result := em.isEmptyValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvironmentSecurityValidation(t *testing.T) {
	em := NewEnvironmentManager()
	
	// Get production environment config
	envConfig := em.environments["production"]
	require.NotNil(t, envConfig)
	
	// Test configuration that meets security requirements
	t.Run("valid_security_config", func(t *testing.T) {
		cfg := &Config{
			Logging: LoggingConfig{
				EnableCorrelation: true,
				StructuredLogging: StructuredLoggingConfig{
					Enabled: true,
				},
			},
			ErrorHandling: ErrorHandlingConfig{
				Retry: RetryConfig{
					Enabled: true,
				},
				CircuitBreaker: CircuitBreakerGroupConfig{
					Enabled: true,
				},
			},
		}
		
		err := em.validateSecurityRequirements(cfg, envConfig)
		assert.NoError(t, err)
	})
	
	// Test configuration that fails security requirements
	t.Run("invalid_security_config", func(t *testing.T) {
		cfg := &Config{
			Logging: LoggingConfig{
				EnableCorrelation: false, // Fails requirement
				StructuredLogging: StructuredLoggingConfig{
					Enabled: false, // Fails requirement
				},
			},
			ErrorHandling: ErrorHandlingConfig{
				Retry: RetryConfig{
					Enabled: false, // Fails requirement
				},
				CircuitBreaker: CircuitBreakerGroupConfig{
					Enabled: false, // Fails requirement
				},
			},
		}
		
		err := em.validateSecurityRequirements(cfg, envConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "correlation logging")
		assert.Contains(t, err.Error(), "structured logging")
		assert.Contains(t, err.Error(), "error handling")
	})
}