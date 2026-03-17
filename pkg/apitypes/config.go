package apitypes

import "time"

// RemoteConfig represents configuration managed remotely via the control plane.
// Pointer fields distinguish "not set" (nil) from "set to zero value".
type RemoteConfig struct {
	GitHub        *GitHubRemoteConfig        `json:"github,omitempty"`
	Claude        *ClaudeRemoteConfig        `json:"claude,omitempty"`
	Agents        *AgentsRemoteConfig        `json:"agents,omitempty"`
	Creativity    *CreativityRemoteConfig    `json:"creativity,omitempty"`
	Decomposition *DecompositionRemoteConfig `json:"decomposition,omitempty"`
	Memory        *MemoryRemoteConfig        `json:"memory,omitempty"`
	ErrorHandling *ErrorHandlingRemoteConfig `json:"error_handling,omitempty"`
	Logging       *LoggingRemoteConfig       `json:"logging,omitempty"`
	Shutdown      *ShutdownRemoteConfig      `json:"shutdown,omitempty"`
}

// GitHubRemoteConfig holds remotely-managed GitHub settings.
type GitHubRemoteConfig struct {
	Token        *string  `json:"token,omitempty"`
	Owner        *string  `json:"owner,omitempty"`
	Repo         *string  `json:"repo,omitempty"`
	PollInterval *string  `json:"poll_interval,omitempty"`
	WatchLabels  []string `json:"watch_labels,omitempty"`
}

// ClaudeRemoteConfig holds remotely-managed Claude AI settings.
type ClaudeRemoteConfig struct {
	APIKey    *string `json:"api_key,omitempty"`
	Model     *string `json:"model,omitempty"`
	MaxTokens *int    `json:"max_tokens,omitempty"`
}

// AgentsRemoteConfig holds remotely-managed agent settings.
type AgentsRemoteConfig struct {
	Developer *DeveloperRemoteConfig `json:"developer,omitempty"`
}

// DeveloperRemoteConfig holds remotely-managed developer agent settings.
type DeveloperRemoteConfig struct {
	Enabled                  *bool   `json:"enabled,omitempty"`
	MaxConcurrent            *int    `json:"max_concurrent,omitempty"`
	WorkspaceDir             *string `json:"workspace_dir,omitempty"`
	AllowPRMerging           *bool   `json:"allow_pr_merging,omitempty"`
	AllowAutoIssueProcessing *bool   `json:"allow_auto_issue_processing,omitempty"`
}

// CreativityRemoteConfig holds remotely-managed creativity engine settings.
type CreativityRemoteConfig struct {
	Enabled                   *bool `json:"enabled,omitempty"`
	IdleThresholdSeconds      *int  `json:"idle_threshold_seconds,omitempty"`
	SuggestionCooldownSeconds *int  `json:"suggestion_cooldown_seconds,omitempty"`
	MaxPendingSuggestions     *int  `json:"max_pending_suggestions,omitempty"`
	MaxRejectionHistory       *int  `json:"max_rejection_history,omitempty"`
}

// DecompositionRemoteConfig holds remotely-managed decomposition settings.
type DecompositionRemoteConfig struct {
	Enabled            *bool `json:"enabled,omitempty"`
	MaxIterationBudget *int  `json:"max_iteration_budget,omitempty"`
	MaxSubtasks        *int  `json:"max_subtasks,omitempty"`
}

// MemoryRemoteConfig holds remotely-managed memory settings.
type MemoryRemoteConfig struct {
	Enabled           *bool `json:"enabled,omitempty"`
	MaxEntries        *int  `json:"max_entries,omitempty"`
	MaxPromptSize     *int  `json:"max_prompt_size,omitempty"`
	ExtractOnComplete *bool `json:"extract_on_complete,omitempty"`
}

// ErrorHandlingRemoteConfig holds remotely-managed error handling settings.
type ErrorHandlingRemoteConfig struct {
	Retry          *RetryRemoteConfig              `json:"retry,omitempty"`
	CircuitBreaker *CircuitBreakerGroupRemoteConfig `json:"circuit_breaker,omitempty"`
}

// RetryRemoteConfig holds remotely-managed retry settings.
type RetryRemoteConfig struct {
	Enabled       *bool                              `json:"enabled,omitempty"`
	DefaultPolicy *RetryPolicyRemoteConfig           `json:"default,omitempty"`
	Policies      map[string]RetryPolicyRemoteConfig `json:"policies,omitempty"`
}

// RetryPolicyRemoteConfig holds remotely-managed retry policy settings.
type RetryPolicyRemoteConfig struct {
	MaxAttempts     *int     `json:"max_attempts,omitempty"`
	BaseDelay       *string  `json:"base_delay,omitempty"`
	MaxDelay        *string  `json:"max_delay,omitempty"`
	BackoffFactor   *float64 `json:"backoff_factor,omitempty"`
	JitterFactor    *float64 `json:"jitter_factor,omitempty"`
	RetryableErrors []string `json:"retryable_errors,omitempty"`
}

// CircuitBreakerGroupRemoteConfig holds remotely-managed circuit breaker group settings.
type CircuitBreakerGroupRemoteConfig struct {
	Enabled       *bool                                 `json:"enabled,omitempty"`
	DefaultConfig *CircuitBreakerRemoteConfig           `json:"default,omitempty"`
	Breakers      map[string]CircuitBreakerRemoteConfig `json:"breakers,omitempty"`
}

// CircuitBreakerRemoteConfig holds remotely-managed circuit breaker settings.
type CircuitBreakerRemoteConfig struct {
	MaxFailures  *int64   `json:"max_failures,omitempty"`
	Timeout      *string  `json:"timeout,omitempty"`
	MaxRequests  *int64   `json:"max_requests,omitempty"`
	FailureRatio *float64 `json:"failure_ratio,omitempty"`
	MinRequests  *int64   `json:"min_requests,omitempty"`
}

// LoggingRemoteConfig holds remotely-managed logging settings.
type LoggingRemoteConfig struct {
	Level *string `json:"level,omitempty"`
}

// ShutdownRemoteConfig holds remotely-managed shutdown settings.
type ShutdownRemoteConfig struct {
	Timeout           *string `json:"timeout,omitempty"`
	CleanupWorkspaces *bool   `json:"cleanup_workspaces,omitempty"`
	ResetClaims       *bool   `json:"reset_claims,omitempty"`
}

// ApplyServerDefaults fills in nil fields with sensible default values.
// This ensures agents receive complete configuration even when only some
// fields were explicitly set in the control plane UI. Sections that are
// entirely absent are initialized with full defaults.
func (rc *RemoteConfig) ApplyServerDefaults() {
	if rc.Agents == nil {
		rc.Agents = &AgentsRemoteConfig{}
	}
	if rc.Agents.Developer == nil {
		rc.Agents.Developer = &DeveloperRemoteConfig{}
	}
	rc.Agents.Developer.applyDefaults()

	if rc.Creativity == nil {
		rc.Creativity = &CreativityRemoteConfig{}
	}
	rc.Creativity.applyDefaults()

	if rc.Decomposition == nil {
		rc.Decomposition = &DecompositionRemoteConfig{}
	}
	rc.Decomposition.applyDefaults()

	if rc.Memory == nil {
		rc.Memory = &MemoryRemoteConfig{}
	}
	rc.Memory.applyDefaults()
}

func (d *DeveloperRemoteConfig) applyDefaults() {
	if d.Enabled == nil {
		v := true
		d.Enabled = &v
	}
	if d.MaxConcurrent == nil {
		v := 1
		d.MaxConcurrent = &v
	}
	if d.WorkspaceDir == nil {
		v := "./workspaces"
		d.WorkspaceDir = &v
	}
	if d.AllowPRMerging == nil {
		v := false
		d.AllowPRMerging = &v
	}
	if d.AllowAutoIssueProcessing == nil {
		v := false
		d.AllowAutoIssueProcessing = &v
	}
}

func (c *CreativityRemoteConfig) applyDefaults() {
	if c.Enabled == nil {
		v := true
		c.Enabled = &v
	}
	if c.IdleThresholdSeconds == nil {
		v := 120
		c.IdleThresholdSeconds = &v
	}
	if c.SuggestionCooldownSeconds == nil {
		v := 300
		c.SuggestionCooldownSeconds = &v
	}
	if c.MaxPendingSuggestions == nil {
		v := 5
		c.MaxPendingSuggestions = &v
	}
	if c.MaxRejectionHistory == nil {
		v := 50
		c.MaxRejectionHistory = &v
	}
}

func (d *DecompositionRemoteConfig) applyDefaults() {
	if d.Enabled == nil {
		v := true
		d.Enabled = &v
	}
	if d.MaxIterationBudget == nil {
		v := 250
		d.MaxIterationBudget = &v
	}
	if d.MaxSubtasks == nil {
		v := 5
		d.MaxSubtasks = &v
	}
}

func (m *MemoryRemoteConfig) applyDefaults() {
	if m.Enabled == nil {
		v := true
		m.Enabled = &v
	}
	if m.MaxEntries == nil {
		v := 100
		m.MaxEntries = &v
	}
	if m.MaxPromptSize == nil {
		v := 8000
		m.MaxPromptSize = &v
	}
	if m.ExtractOnComplete == nil {
		v := true
		m.ExtractOnComplete = &v
	}
}

// ConfigResponse is returned to agents when they poll for configuration.
type ConfigResponse struct {
	Config  RemoteConfig `json:"config"`
	Version int64        `json:"version"`
}

// AgentTypeConfigInfo is returned by the management API.
type AgentTypeConfigInfo struct {
	ID          string       `json:"id"`
	OrgID       string       `json:"org_id"`
	AgentType   string       `json:"agent_type"`
	Config      RemoteConfig `json:"config"`
	Version     int64        `json:"version"`
	Description string       `json:"description,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// AgentConfigOverrideInfo is returned by the management API.
type AgentConfigOverrideInfo struct {
	ID          string       `json:"id"`
	AgentID     string       `json:"agent_id"`
	Config      RemoteConfig `json:"config"`
	Version     int64        `json:"version"`
	Description string       `json:"description,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// ConfigAuditEntry represents a config change audit trail entry.
type ConfigAuditEntry struct {
	ID             string        `json:"id"`
	TargetType     string        `json:"target_type"`
	TargetID       string        `json:"target_id"`
	ChangedBy      string        `json:"changed_by,omitempty"`
	PreviousConfig *RemoteConfig `json:"previous_config,omitempty"`
	NewConfig      RemoteConfig  `json:"new_config"`
	Version        int64         `json:"version"`
	CreatedAt      time.Time     `json:"created_at"`
}
