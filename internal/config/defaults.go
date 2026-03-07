package config

import "time"

// Defaults holds all default configuration values.
type Defaults struct {
	GitHub        GitHubDefaults
	Claude        ClaudeDefaults
	Agents        AgentsDefaults
	State         StateDefaults
	Logging       LoggingDefaults
	Metrics       MetricsDefaults
	Observability ObservabilityDefaults
	Creativity    CreativityDefaults
	Decomposition DecompositionDefaults
	ErrorHandling ErrorHandlingDefaults
	Shutdown      ShutdownDefaults
	Workspace     WorkspaceDefaults
}

// GitHubDefaults holds default GitHub configuration values.
type GitHubDefaults struct {
	PollInterval time.Duration
	WatchLabels  []string
}

// ClaudeDefaults holds default Claude configuration values.
type ClaudeDefaults struct {
	Model     string
	MaxTokens int
}

// AgentsDefaults holds default agent configuration values.
type AgentsDefaults struct {
	Developer DeveloperAgentDefaults
}

// DeveloperAgentDefaults holds default developer agent configuration values.
type DeveloperAgentDefaults struct {
	Enabled       bool
	MaxConcurrent int
	WorkspaceDir  string
	Recovery      RecoveryDefaults
}

// StateDefaults holds default state configuration values.
type StateDefaults struct {
	Backend string
	Dir     string
}

// LoggingDefaults holds default logging configuration values.
type LoggingDefaults struct {
	Level             string
	Format            string
	EnableCorrelation bool
	Sampling          LoggingSamplingDefaults
	Rotation          LogRotationDefaults
	Cleanup           LogCleanupDefaults
}

// LoggingSamplingDefaults holds default log sampling configuration values.
type LoggingSamplingDefaults struct {
	Enabled bool
	Rate    float64
}

// LogRotationDefaults holds default log rotation configuration values.
type LogRotationDefaults struct {
	Enabled       bool
	MaxFileSize   int64
	MaxFiles      int
	MaxAge        time.Duration
	CompressOld   bool
	CheckInterval time.Duration
}

// LogCleanupDefaults holds default log cleanup configuration values.
type LogCleanupDefaults struct {
	Enabled             bool
	RetentionDays       int
	MinFreeDiskMB       int64
	CleanupInterval     time.Duration
	ArchiveBeforeDelete bool
}

// MetricsDefaults holds default metrics configuration values.
type MetricsDefaults struct {
	Enabled            bool
	CollectionInterval time.Duration
	Export             MetricsExportDefaults
}

// MetricsExportDefaults holds default metrics export configuration values.
type MetricsExportDefaults struct {
	Prometheus PrometheusDefaults
	Logs       LogsExportDefaults
}

// PrometheusDefaults holds default Prometheus configuration values.
type PrometheusDefaults struct {
	Enabled bool
	Port    int
	Path    string
}

// LogsExportDefaults holds default logs export configuration values.
type LogsExportDefaults struct {
	Enabled  bool
	Interval time.Duration
}

// ObservabilityDefaults holds default observability configuration values.
type ObservabilityDefaults struct {
	Tracing     TracingDefaults
	Health      HealthDefaults
	Performance PerformanceDefaults
}

// TracingDefaults holds default tracing configuration values.
type TracingDefaults struct {
	Enabled    bool
	SampleRate float64
}

// HealthDefaults holds default health configuration values.
type HealthDefaults struct {
	Enabled bool
	Port    int
	Path    string
}

// PerformanceDefaults holds default performance configuration values.
type PerformanceDefaults struct {
	TrackDurations   bool
	MemoryMonitoring bool
	Interval         time.Duration
}

// CreativityDefaults holds default creativity configuration values.
type CreativityDefaults struct {
	Enabled                   bool
	IdleThresholdSeconds      int
	SuggestionCooldownSeconds int
	MaxPendingSuggestions     int
	MaxRejectionHistory       int
}

// DecompositionDefaults holds default decomposition configuration values.
type DecompositionDefaults struct {
	Enabled            bool
	MaxIterationBudget int
	MaxSubtasks        int
}

// ErrorHandlingDefaults holds default error handling configuration values.
type ErrorHandlingDefaults struct {
	Retry          RetryDefaults
	CircuitBreaker CircuitBreakerDefaults
}

// RetryDefaults holds default retry configuration values.
type RetryDefaults struct {
	Enabled       bool
	DefaultPolicy RetryPolicyDefaults
}

// RetryPolicyDefaults holds default retry policy configuration values.
type RetryPolicyDefaults struct {
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	JitterFactor  float64
}

// CircuitBreakerDefaults holds default circuit breaker configuration values.
type CircuitBreakerDefaults struct {
	Enabled      bool
	MaxFailures  int64
	Timeout      time.Duration
	MaxRequests  int64
	FailureRatio float64
	MinRequests  int64
}

// ShutdownDefaults holds default shutdown configuration values.
type ShutdownDefaults struct {
	Timeout           time.Duration
	CleanupWorkspaces bool
	ResetClaims       bool
}

// WorkspaceDefaults holds default workspace configuration values.
type WorkspaceDefaults struct {
	Cleanup    WorkspaceCleanupDefaults
	Limits     WorkspaceLimitsDefaults
	Monitoring WorkspaceMonitoringDefaults
}

// WorkspaceCleanupDefaults holds default workspace cleanup configuration values.
type WorkspaceCleanupDefaults struct {
	Enabled          bool
	SuccessRetention time.Duration
	FailureRetention time.Duration
	MaxConcurrent    int
}

// WorkspaceLimitsDefaults holds default workspace limits configuration values.
type WorkspaceLimitsDefaults struct {
	MaxSizeMB     int64
	MinFreeDiskMB int64
}

// WorkspaceMonitoringDefaults holds default workspace monitoring configuration values.
type WorkspaceMonitoringDefaults struct {
	DiskCheckInterval time.Duration
	CleanupInterval   time.Duration
}

// RecoveryDefaults holds default recovery configuration values.
type RecoveryDefaults struct {
	Enabled             bool
	StartupValidation   bool
	AutoCleanupOrphaned bool
	MaxResumeAge        time.Duration
	ValidationInterval  time.Duration
	Consistency         ConsistencyDefaults
}

// ConsistencyDefaults holds default consistency configuration values.
type ConsistencyDefaults struct {
	ValidateOnStartup    bool
	ValidatePeriodically bool
	ReconcileDrift       bool
}

// GetDefaults returns the complete default configuration.
func GetDefaults() Defaults {
	return Defaults{
		GitHub: GitHubDefaults{
			PollInterval: 30 * time.Second,
			WatchLabels:  []string{"agent:ready"},
		},
		Claude: ClaudeDefaults{
			Model:     "claude-sonnet-4-20250514",
			MaxTokens: 8192,
		},
		Agents: AgentsDefaults{
			Developer: DeveloperAgentDefaults{
				Enabled:       false,
				MaxConcurrent: 1,
				WorkspaceDir:  "./workspaces",
				Recovery: RecoveryDefaults{
					Enabled:             true,
					StartupValidation:   true,
					AutoCleanupOrphaned: false,
					MaxResumeAge:        24 * time.Hour,
					ValidationInterval:  1 * time.Hour,
					Consistency: ConsistencyDefaults{
						ValidateOnStartup:    true,
						ValidatePeriodically: false,
						ReconcileDrift:       false,
					},
				},
			},
		},
		State: StateDefaults{
			Backend: "file",
			Dir:     ".agentctl/state",
		},
		Logging: LoggingDefaults{
			Level:             "info",
			Format:            "json",
			EnableCorrelation: true,
			Sampling: LoggingSamplingDefaults{
				Enabled: false,
				Rate:    0.1,
			},
			Rotation: LogRotationDefaults{
				Enabled:       true,
				MaxFileSize:   100, // 100MB
				MaxFiles:      10,
				MaxAge:        7 * 24 * time.Hour, // 7 days
				CompressOld:   true,
				CheckInterval: 1 * time.Hour,
			},
			Cleanup: LogCleanupDefaults{
				Enabled:             true,
				RetentionDays:       30,
				MinFreeDiskMB:       1024, // 1GB
				CleanupInterval:     24 * time.Hour,
				ArchiveBeforeDelete: true,
			},
		},
		Metrics: MetricsDefaults{
			Enabled:            true,
			CollectionInterval: 30 * time.Second,
			Export: MetricsExportDefaults{
				Prometheus: PrometheusDefaults{
					Enabled: false,
					Port:    8080,
					Path:    "/metrics",
				},
				Logs: LogsExportDefaults{
					Enabled:  true,
					Interval: 60 * time.Second,
				},
			},
		},
		Observability: ObservabilityDefaults{
			Tracing: TracingDefaults{
				Enabled:    false,
				SampleRate: 0.1,
			},
			Health: HealthDefaults{
				Enabled: true,
				Port:    8081,
				Path:    "/health",
			},
			Performance: PerformanceDefaults{
				TrackDurations:   true,
				MemoryMonitoring: true,
				Interval:         30 * time.Second,
			},
		},
		Creativity: CreativityDefaults{
			Enabled:                   false,
			IdleThresholdSeconds:      120,
			SuggestionCooldownSeconds: 300,
			MaxPendingSuggestions:     1,
			MaxRejectionHistory:       50,
		},
		Decomposition: DecompositionDefaults{
			Enabled:            false,
			MaxIterationBudget: 25,
			MaxSubtasks:        5,
		},
		ErrorHandling: ErrorHandlingDefaults{
			Retry: RetryDefaults{
				Enabled: true,
				DefaultPolicy: RetryPolicyDefaults{
					MaxAttempts:   3,
					BaseDelay:     1 * time.Second,
					MaxDelay:      30 * time.Second,
					BackoffFactor: 2.0,
					JitterFactor:  0.1,
				},
			},
			CircuitBreaker: CircuitBreakerDefaults{
				Enabled:      true,
				MaxFailures:  5,
				Timeout:      60 * time.Second,
				MaxRequests:  10,
				FailureRatio: 0.5,
				MinRequests:  3,
			},
		},
		Shutdown: ShutdownDefaults{
			Timeout:           30 * time.Second,
			CleanupWorkspaces: true,
			ResetClaims:       true,
		},
		Workspace: WorkspaceDefaults{
			Cleanup: WorkspaceCleanupDefaults{
				Enabled:          true,
				SuccessRetention: 24 * time.Hour,
				FailureRetention: 168 * time.Hour, // 1 week
				MaxConcurrent:    5,
			},
			Limits: WorkspaceLimitsDefaults{
				MaxSizeMB:     1024, // 1GB
				MinFreeDiskMB: 2048, // 2GB
			},
			Monitoring: WorkspaceMonitoringDefaults{
				DiskCheckInterval: 5 * time.Minute,
				CleanupInterval:   1 * time.Hour,
			},
		},
	}
}

// ApplyDefaults applies default values to a config instance.
func ApplyDefaults(cfg *Config) {
	defaults := GetDefaults()

	// GitHub defaults
	if cfg.GitHub.PollInterval == 0 {
		cfg.GitHub.PollInterval = defaults.GitHub.PollInterval
	}
	if len(cfg.GitHub.WatchLabels) == 0 {
		cfg.GitHub.WatchLabels = defaults.GitHub.WatchLabels
	}

	// Claude defaults
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = defaults.Claude.Model
	}
	if cfg.Claude.MaxTokens == 0 {
		cfg.Claude.MaxTokens = defaults.Claude.MaxTokens
	}

	// State defaults
	if cfg.State.Backend == "" {
		cfg.State.Backend = defaults.State.Backend
	}
	if cfg.State.Dir == "" {
		cfg.State.Dir = defaults.State.Dir
	}

	// Agent defaults
	if cfg.Agents.Developer.MaxConcurrent == 0 {
		cfg.Agents.Developer.MaxConcurrent = defaults.Agents.Developer.MaxConcurrent
	}
	if cfg.Agents.Developer.WorkspaceDir == "" {
		cfg.Agents.Developer.WorkspaceDir = defaults.Agents.Developer.WorkspaceDir
	}

	// Creativity defaults
	if cfg.Creativity.IdleThresholdSeconds == 0 {
		cfg.Creativity.IdleThresholdSeconds = defaults.Creativity.IdleThresholdSeconds
	}
	if cfg.Creativity.SuggestionCooldownSeconds == 0 {
		cfg.Creativity.SuggestionCooldownSeconds = defaults.Creativity.SuggestionCooldownSeconds
	}
	if cfg.Creativity.MaxPendingSuggestions == 0 {
		cfg.Creativity.MaxPendingSuggestions = defaults.Creativity.MaxPendingSuggestions
	}
	if cfg.Creativity.MaxRejectionHistory == 0 {
		cfg.Creativity.MaxRejectionHistory = defaults.Creativity.MaxRejectionHistory
	}

	// Decomposition defaults
	if cfg.Decomposition.MaxIterationBudget == 0 {
		cfg.Decomposition.MaxIterationBudget = defaults.Decomposition.MaxIterationBudget
	}
	if cfg.Decomposition.MaxSubtasks == 0 {
		cfg.Decomposition.MaxSubtasks = defaults.Decomposition.MaxSubtasks
	}

	// Workspace defaults
	if cfg.Workspace.Limits.MaxSizeMB == 0 {
		cfg.Workspace.Limits.MaxSizeMB = defaults.Workspace.Limits.MaxSizeMB
	}
	if cfg.Workspace.Limits.MinFreeDiskMB == 0 {
		cfg.Workspace.Limits.MinFreeDiskMB = defaults.Workspace.Limits.MinFreeDiskMB
	}
	if cfg.Workspace.Cleanup.MaxConcurrent == 0 {
		cfg.Workspace.Cleanup.MaxConcurrent = defaults.Workspace.Cleanup.MaxConcurrent
	}
	if cfg.Workspace.Cleanup.SuccessRetention == 0 {
		cfg.Workspace.Cleanup.SuccessRetention = defaults.Workspace.Cleanup.SuccessRetention
	}
	if cfg.Workspace.Cleanup.FailureRetention == 0 {
		cfg.Workspace.Cleanup.FailureRetention = defaults.Workspace.Cleanup.FailureRetention
	}
	if cfg.Workspace.Monitoring.DiskCheckInterval == 0 {
		cfg.Workspace.Monitoring.DiskCheckInterval = defaults.Workspace.Monitoring.DiskCheckInterval
	}
	if cfg.Workspace.Monitoring.CleanupInterval == 0 {
		cfg.Workspace.Monitoring.CleanupInterval = defaults.Workspace.Monitoring.CleanupInterval
	}

	// Logging defaults
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaults.Logging.Level
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = defaults.Logging.Format
	}

	// Recovery defaults
	if cfg.Agents.Developer.Recovery.MaxResumeAge == 0 {
		cfg.Agents.Developer.Recovery.MaxResumeAge = defaults.Agents.Developer.Recovery.MaxResumeAge
	}
	if cfg.Agents.Developer.Recovery.ValidationInterval == 0 {
		cfg.Agents.Developer.Recovery.ValidationInterval = defaults.Agents.Developer.Recovery.ValidationInterval
	}

	// Shutdown defaults
	if cfg.Shutdown.Timeout == 0 {
		cfg.Shutdown.Timeout = defaults.Shutdown.Timeout
	}

	// Memory defaults
	if cfg.Memory.MaxEntries == 0 {
		cfg.Memory.MaxEntries = 100
	}
	if cfg.Memory.MaxPromptSize == 0 {
		cfg.Memory.MaxPromptSize = 8000
	}
}
