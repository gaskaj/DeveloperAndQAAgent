package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	Shutdown      ShutdownConfig      `mapstructure:"shutdown"`
	Workspace     WorkspaceConfig     `mapstructure:"workspace"`
	ControlPlane  ControlPlaneConfig  `mapstructure:"controlplane"`
	Memory        MemoryConfig        `mapstructure:"memory"`
}

// MemoryConfig holds configuration for the repository memory system.
type MemoryConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	MaxEntries      int  `mapstructure:"max_entries"`
	MaxPromptSize   int  `mapstructure:"max_prompt_size"`
	ExtractOnComplete bool `mapstructure:"extract_on_complete"`
}

// ControlPlaneConfig holds configuration for reporting to the WebUI control plane.
type ControlPlaneConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	URL               string        `mapstructure:"url"`
	APIKey            string        `mapstructure:"api_key"`
	AgentName         string        `mapstructure:"agent_name"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
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
	Enabled                  bool                     `mapstructure:"enabled"`
	MaxConcurrent            int                      `mapstructure:"max_concurrent"`
	WorkspaceDir             string                   `mapstructure:"workspace_dir"`
	AllowPRMerging           bool                     `mapstructure:"allow_pr_merging"`
	AllowAutoIssueProcessing bool                     `mapstructure:"allow_auto_issue_processing"`
	Recovery                 RecoveryConfig           `mapstructure:"recovery"`
	Workspace                DeveloperWorkspaceConfig `mapstructure:"workspace"`
}

// DeveloperWorkspaceConfig holds workspace-specific configuration for the developer agent.
type DeveloperWorkspaceConfig struct {
	Persistence PersistenceConfig `mapstructure:"persistence"`
}

// PersistenceConfig holds workspace persistence configuration.
type PersistenceConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	SnapshotInterval     time.Duration `mapstructure:"snapshot_interval"`
	MaxSnapshots         int           `mapstructure:"max_snapshots"`
	RetentionHours       int           `mapstructure:"retention_hours"`
	CompressSnapshots    bool          `mapstructure:"compress_snapshots"`
	ResumeOnRestart      bool          `mapstructure:"resume_on_restart"`
	ValidateBeforeResume bool          `mapstructure:"validate_before_resume"`
}

// StateConfig holds state storage configuration.
type StateConfig struct {
	Backend string `mapstructure:"backend"`
	Dir     string `mapstructure:"dir"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level             string                  `mapstructure:"level"`
	Format            string                  `mapstructure:"format"`
	FilePath          string                  `mapstructure:"file_path"`
	EnableCorrelation bool                    `mapstructure:"enable_correlation"`
	Sampling          LoggingSamplingConfig   `mapstructure:"sampling"`
	Components        map[string]string       `mapstructure:"components"`
	StructuredLogging StructuredLoggingConfig `mapstructure:"structured_logging"`
	MultiAgentObserve MultiAgentObservability `mapstructure:"multi_agent_observability"`
	Rotation          LogRotationConfig       `mapstructure:"rotation"`
	Cleanup           LogCleanupConfig        `mapstructure:"cleanup"`
}

// StructuredLoggingConfig holds structured logging configuration
type StructuredLoggingConfig struct {
	Enabled           bool                     `mapstructure:"enabled"`
	Format            string                   `mapstructure:"format"`
	IncludeCaller     bool                     `mapstructure:"include_caller"`
	IncludeStackTrace bool                     `mapstructure:"include_stack_trace"`
	Correlation       CorrelationConfig        `mapstructure:"correlation"`
	WorkflowTracking  WorkflowTrackingConfig   `mapstructure:"workflow_tracking"`
	Performance       PerformanceLoggingConfig `mapstructure:"performance"`
	Filtering         LogFilteringConfig       `mapstructure:"filtering"`
	Export            LogExportConfig          `mapstructure:"export"`
}

// CorrelationConfig holds correlation context configuration
type CorrelationConfig struct {
	Enabled                bool `mapstructure:"enabled"`
	AutoGenerate           bool `mapstructure:"auto_generate"`
	IncludeWorkflowStage   bool `mapstructure:"include_workflow_stage"`
	IncludeAgentMetadata   bool `mapstructure:"include_agent_metadata"`
	PropagateGitHubContext bool `mapstructure:"propagate_github_context"`
}

// WorkflowTrackingConfig holds workflow tracking configuration
type WorkflowTrackingConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	TrackHandoffs      bool `mapstructure:"track_handoffs"`
	TrackDecisions     bool `mapstructure:"track_decisions"`
	IncludePerformance bool `mapstructure:"include_performance"`
	TrackToolUsage     bool `mapstructure:"track_tool_usage"`
}

// PerformanceLoggingConfig holds performance logging configuration
type PerformanceLoggingConfig struct {
	TrackDurations  bool `mapstructure:"track_durations"`
	MemorySnapshots bool `mapstructure:"memory_snapshots"`
	LLMMetrics      bool `mapstructure:"llm_metrics"`
	WorkflowTiming  bool `mapstructure:"workflow_timing"`
}

// LogFilteringConfig holds log filtering configuration
type LogFilteringConfig struct {
	DebugSamplingRate float64  `mapstructure:"debug_sampling_rate"`
	IncludeErrors     bool     `mapstructure:"include_errors"`
	IncludeWarnings   bool     `mapstructure:"include_warnings"`
	IncludeEvents     []string `mapstructure:"include_events"`
}

// LogExportConfig holds log export configuration
type LogExportConfig struct {
	Enabled       bool                         `mapstructure:"enabled"`
	FieldMappings map[string]map[string]string `mapstructure:"field_mappings"`
}

// MultiAgentObservability holds multi-agent observability configuration
type MultiAgentObservability struct {
	CrossAgentTracking    bool                     `mapstructure:"cross_agent_tracking"`
	CommunicationPatterns bool                     `mapstructure:"communication_patterns"`
	PerformanceComparison bool                     `mapstructure:"performance_comparison"`
	WorkflowEfficiency    bool                     `mapstructure:"workflow_efficiency"`
	Alerting              MultiAgentAlertingConfig `mapstructure:"alerting"`
}

// MultiAgentAlertingConfig holds alerting configuration for multi-agent issues
type MultiAgentAlertingConfig struct {
	LostCorrelationThreshold   float64 `mapstructure:"lost_correlation_threshold"`
	HandoffTimeoutSeconds      int     `mapstructure:"handoff_timeout_seconds"`
	StageStallThresholdSeconds int     `mapstructure:"stage_stall_threshold_seconds"`
	ToolFailureRateThreshold   float64 `mapstructure:"tool_failure_rate_threshold"`
}

// LoggingSamplingConfig holds log sampling configuration.
type LoggingSamplingConfig struct {
	Enabled bool    `mapstructure:"enabled"`
	Rate    float64 `mapstructure:"rate"`
}

// LogRotationConfig holds configuration for log rotation
type LogRotationConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	MaxFileSize   int64         `mapstructure:"max_file_size_mb"` // MB
	MaxFiles      int           `mapstructure:"max_files"`
	MaxAge        time.Duration `mapstructure:"max_age"`
	CompressOld   bool          `mapstructure:"compress_old"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

// LogCleanupConfig holds configuration for log cleanup and disk space management
type LogCleanupConfig struct {
	Enabled             bool          `mapstructure:"enabled"`
	RetentionDays       int           `mapstructure:"retention_days"`
	MinFreeDiskMB       int64         `mapstructure:"min_free_disk_mb"` // MB
	CleanupInterval     time.Duration `mapstructure:"cleanup_interval"`
	ArchiveBeforeDelete bool          `mapstructure:"archive_before_delete"`
}

// MetricsConfig holds metrics collection configuration.
type MetricsConfig struct {
	Enabled            bool                `mapstructure:"enabled"`
	CollectionInterval time.Duration       `mapstructure:"collection_interval"`
	Export             MetricsExportConfig `mapstructure:"export"`
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
	TrackDurations   bool          `mapstructure:"track_durations"`
	MemoryMonitoring bool          `mapstructure:"memory_monitoring"`
	Interval         time.Duration `mapstructure:"interval"`
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

// ShutdownConfig holds configuration for graceful shutdown behavior.
type ShutdownConfig struct {
	Timeout           time.Duration `mapstructure:"timeout"`
	CleanupWorkspaces bool          `mapstructure:"cleanup_workspaces"`
	ResetClaims       bool          `mapstructure:"reset_claims"`
}

// ErrorHandlingConfig holds configuration for error handling and retry mechanisms.
type ErrorHandlingConfig struct {
	Retry          RetryConfig               `mapstructure:"retry"`
	CircuitBreaker CircuitBreakerGroupConfig `mapstructure:"circuit_breaker"`
}

// RetryConfig holds global retry configuration.
type RetryConfig struct {
	Enabled       bool                         `mapstructure:"enabled"`
	DefaultPolicy RetryPolicyConfig            `mapstructure:"default"`
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
	Enabled       bool                                `mapstructure:"enabled"`
	DefaultConfig CircuitBreakerConfigSpec            `mapstructure:"default"`
	Breakers      map[string]CircuitBreakerConfigSpec `mapstructure:"breakers"`
}

// CircuitBreakerConfigSpec holds circuit breaker configuration.
type CircuitBreakerConfigSpec struct {
	MaxFailures  int64         `mapstructure:"max_failures"`
	Timeout      time.Duration `mapstructure:"timeout"`
	MaxRequests  int64         `mapstructure:"max_requests"`
	FailureRatio float64       `mapstructure:"failure_ratio"`
	MinRequests  int64         `mapstructure:"min_requests"`
}

// WorkspaceConfig holds workspace management configuration.
type WorkspaceConfig struct {
	Cleanup    WorkspaceCleanupConfig    `mapstructure:"cleanup"`
	Limits     WorkspaceLimitsConfig     `mapstructure:"limits"`
	Monitoring WorkspaceMonitoringConfig `mapstructure:"monitoring"`
}

// WorkspaceCleanupConfig holds workspace cleanup configuration.
type WorkspaceCleanupConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	SuccessRetention time.Duration `mapstructure:"success_retention"`
	FailureRetention time.Duration `mapstructure:"failure_retention"`
	MaxConcurrent    int           `mapstructure:"max_concurrent"`
}

// WorkspaceLimitsConfig holds workspace resource limits.
type WorkspaceLimitsConfig struct {
	MaxSizeMB     int64 `mapstructure:"max_size_mb"`
	MinFreeDiskMB int64 `mapstructure:"min_free_disk_mb"`
}

// WorkspaceMonitoringConfig holds workspace monitoring configuration.
type WorkspaceMonitoringConfig struct {
	DiskCheckInterval time.Duration `mapstructure:"disk_check_interval"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval"`
}

// RecoveryConfig holds configuration for recovery operations.
type RecoveryConfig struct {
	Enabled             bool              `mapstructure:"enabled"`
	StartupValidation   bool              `mapstructure:"startup_validation"`
	AutoCleanupOrphaned bool              `mapstructure:"auto_cleanup_orphaned"`
	MaxResumeAge        time.Duration     `mapstructure:"max_resume_age"`
	ValidationInterval  time.Duration     `mapstructure:"validation_interval"`
	Consistency         ConsistencyConfig `mapstructure:"consistency"`
}

// ConsistencyConfig holds configuration for consistency validation.
type ConsistencyConfig struct {
	ValidateOnStartup    bool `mapstructure:"validate_on_startup"`
	ValidatePeriodically bool `mapstructure:"validate_periodically"`
	ReconcileDrift       bool `mapstructure:"reconcile_drift"`
}

// GetRepoPath returns the owner/repo path for organizing files by repository.
// Returns empty string if GitHub configuration is incomplete.
func (c *Config) GetRepoPath() string {
	if c.GitHub.Owner == "" || c.GitHub.Repo == "" {
		return ""
	}
	return filepath.Join(c.GitHub.Owner, c.GitHub.Repo)
}

// GetLogPath returns the log directory path for the configured repository.
// Falls back to the base log directory if GitHub configuration is incomplete.
func (c *Config) GetLogPath(baseLogDir string) string {
	repoPath := c.GetRepoPath()
	if repoPath == "" {
		return baseLogDir
	}
	return filepath.Join(baseLogDir, repoPath)
}

// GetWorkspacePath returns the workspace directory path for the configured repository.
// Falls back to the base workspace directory if GitHub configuration is incomplete.
func (c *Config) GetWorkspacePath(baseWorkspaceDir string) string {
	repoPath := c.GetRepoPath()
	if repoPath == "" {
		return baseWorkspaceDir
	}
	return filepath.Join(baseWorkspaceDir, repoPath)
}

// Load reads configuration from the given file path, expanding environment variables.
func Load(path string) (*Config, error) {
	return LoadWithOptions(path, false)
}

// LoadWithOptions loads configuration with optional network validation.
func LoadWithOptions(path string, skipNetworkValidation bool) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Enable strict unmarshaling to catch unknown fields
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	// Expand environment variables in all string values.
	expandedKeys := make(map[string]bool)
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.Contains(val, "${") {
			expanded := os.Expand(val, os.Getenv)
			v.Set(key, expanded)
			expandedKeys[key] = true
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config from %q: %w", path, err)
	}

	// Validate configuration with enhanced validation
	ctx := context.Background()
	if err := ValidateWithContext(ctx, &cfg, skipNetworkValidation); err != nil {
		return nil, fmt.Errorf("validating config from %q: %w", path, err)
	}

	return &cfg, nil
}

// LoadWithSchemaValidation loads configuration with strict schema validation to catch typos.
func LoadWithSchemaValidation(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	// Expand environment variables
	for _, key := range v.AllKeys() {
		val := v.GetString(key)
		if strings.Contains(val, "${") {
			expanded := os.Expand(val, os.Getenv)
			v.Set(key, expanded)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		// Try to provide helpful error messages for common typos
		if strings.Contains(err.Error(), "cannot unmarshal") {
			return nil, fmt.Errorf("config parsing error in %q: %w\nHint: Check for typos in field names or incorrect value types", path, err)
		}
		return nil, fmt.Errorf("unmarshaling config from %q: %w", path, err)
	}

	// Validate with full checks including network validation
	ctx := context.Background()
	if err := ValidateWithContext(ctx, &cfg, false); err != nil {
		return nil, fmt.Errorf("validating config from %q: %w", path, err)
	}

	return &cfg, nil
}

// LoadWithEnvironment loads configuration with environment-specific overlays
func LoadWithEnvironment(basePath string, environment string) (*Config, error) {
	envManager := NewEnvironmentManager()
	cfg, err := envManager.LoadEnvironmentConfig(basePath, environment)
	if err != nil {
		return nil, fmt.Errorf("loading environment config: %w", err)
	}

	// Run standard validation
	ctx := context.Background()
	if err := ValidateWithContext(ctx, cfg, false); err != nil {
		return nil, fmt.Errorf("validating environment config: %w", err)
	}

	return cfg, nil
}
