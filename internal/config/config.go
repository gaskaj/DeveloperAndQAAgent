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
	GitHub        GitHubConfig        `mapstructure:"github"`
	Claude        ClaudeConfig        `mapstructure:"claude"`
	Agents        AgentsConfig        `mapstructure:"agents"`
	State         StateConfig         `mapstructure:"state"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Metrics       MetricsConfig       `mapstructure:"metrics"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Creativity    CreativityConfig    `mapstructure:"creativity"`
	Decomposition DecompositionConfig `mapstructure:"decomposition"`
	ErrorHandling ErrorHandlingConfig `mapstructure:"error_handling"`
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
	Level              string                    `mapstructure:"level"`
	Format             string                    `mapstructure:"format"`
	EnableCorrelation  bool                      `mapstructure:"enable_correlation"`
	Sampling           LoggingSamplingConfig     `mapstructure:"sampling"`
	Components         map[string]string         `mapstructure:"components"`
}

// LoggingSamplingConfig holds log sampling configuration.
type LoggingSamplingConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	Rate    float64 `mapstructure:"rate"`
}

// MetricsConfig holds metrics collection configuration.
type MetricsConfig struct {
	Enabled            bool                      `mapstructure:"enabled"`
	CollectionInterval time.Duration             `mapstructure:"collection_interval"`
	Export             MetricsExportConfig       `mapstructure:"export"`
}

// MetricsExportConfig holds metrics export configuration.
type MetricsExportConfig struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Logs       LogsExportConfig `mapstructure:"logs"`
}

// PrometheusConfig holds Prometheus metrics export configuration.
type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// LogsExportConfig holds log-based metrics export configuration.
type LogsExportConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Interval time.Duration `mapstructure:"interval"`
}

// ObservabilityConfig holds observability features configuration.
type ObservabilityConfig struct {
	Tracing     TracingConfig     `mapstructure:"tracing"`
	Health      HealthConfig      `mapstructure:"health"`
	Performance PerformanceConfig `mapstructure:"performance"`
}

// TracingConfig holds distributed tracing configuration.
type TracingConfig struct {
	Enabled    bool    `mapstructure:"enabled"`
	SampleRate float64 `mapstructure:"sample_rate"`
}

// HealthConfig holds health check configuration.
type HealthConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// PerformanceConfig holds performance monitoring configuration.
type PerformanceConfig struct {
	TrackDurations     bool          `mapstructure:"track_durations"`
	MemoryMonitoring   bool          `mapstructure:"memory_monitoring"`
	Interval           time.Duration `mapstructure:"interval"`
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

// ErrorHandlingConfig holds configuration for error handling and retry mechanisms.
type ErrorHandlingConfig struct {
	Retry         RetryConfig         `mapstructure:"retry"`
	CircuitBreaker CircuitBreakerGroupConfig `mapstructure:"circuit_breaker"`
}

// RetryConfig holds global retry configuration.
type RetryConfig struct {
	Enabled       bool                       `mapstructure:"enabled"`
	DefaultPolicy RetryPolicyConfig          `mapstructure:"default"`
	Policies      map[string]RetryPolicyConfig `mapstructure:"policies"`
}

// RetryPolicyConfig holds retry policy configuration.
type RetryPolicyConfig struct {
	MaxAttempts     int           `mapstructure:"max_attempts"`
	BaseDelay       time.Duration `mapstructure:"base_delay"`
	MaxDelay        time.Duration `mapstructure:"max_delay"`
	BackoffFactor   float64       `mapstructure:"backoff_factor"`
	JitterFactor    float64       `mapstructure:"jitter_factor"`
	RetryableErrors []string      `mapstructure:"retryable_errors"`
}

// CircuitBreakerGroupConfig holds configuration for multiple circuit breakers.
type CircuitBreakerGroupConfig struct {
	Enabled      bool                              `mapstructure:"enabled"`
	DefaultConfig CircuitBreakerConfigSpec          `mapstructure:"default"`
	Breakers     map[string]CircuitBreakerConfigSpec `mapstructure:"breakers"`
}

// CircuitBreakerConfigSpec holds circuit breaker configuration.
type CircuitBreakerConfigSpec struct {
	MaxFailures  int64         `mapstructure:"max_failures"`
	Timeout      time.Duration `mapstructure:"timeout"`
	MaxRequests  int64         `mapstructure:"max_requests"`
	FailureRatio float64       `mapstructure:"failure_ratio"`
	MinRequests  int64         `mapstructure:"min_requests"`
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
