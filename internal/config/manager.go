package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// ConfigManager provides runtime configuration management with hot-reload capabilities
type ConfigManager struct {
	currentConfig *Config
	configPath    string
	logger        *slog.Logger
	validator     *Validator
	mu            sync.RWMutex
	subscribers   []ConfigChangeCallback
	watchCancel   context.CancelFunc
}

// ConfigChangeCallback is called when configuration changes
type ConfigChangeCallback func(oldConfig, newConfig *Config) error

// ConfigChangeEvent represents a configuration change
type ConfigChangeEvent struct {
	Type      ConfigChangeType
	Field     string
	OldValue  interface{}
	NewValue  interface{}
	Timestamp time.Time
}

// ConfigChangeType indicates the type of configuration change
type ConfigChangeType int

const (
	ConfigAdded ConfigChangeType = iota
	ConfigModified
	ConfigRemoved
)

// HotReloadableFields defines which configuration fields can be hot-reloaded
var HotReloadableFields = map[string]bool{
	"logging.level":                    true,
	"logging.format":                   true,
	"logging.enable_correlation":       true,
	"logging.sampling.enabled":         true,
	"logging.sampling.rate":            true,
	"github.poll_interval":             true,
	"github.watch_labels":              true,
	"creativity.idle_threshold_seconds": true,
	"creativity.suggestion_cooldown_seconds": true,
	"creativity.max_pending_suggestions": true,
	"metrics.enabled":                  true,
	"metrics.collection_interval":      true,
	"observability.performance.track_durations": true,
	"observability.performance.memory_monitoring": true,
	"workspace.cleanup.enabled":        true,
	"workspace.cleanup.success_retention": true,
	"workspace.cleanup.failure_retention": true,
	"workspace.monitoring.disk_check_interval": true,
	"workspace.monitoring.cleanup_interval": true,
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string, logger *slog.Logger) *ConfigManager {
	return &ConfigManager{
		configPath:  configPath,
		logger:      logger,
		validator:   NewValidator(),
		subscribers: make([]ConfigChangeCallback, 0),
	}
}

// LoadInitialConfig loads the initial configuration
func (cm *ConfigManager) LoadInitialConfig() (*Config, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cfg, err := Load(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("loading initial config: %w", err)
	}

	cm.currentConfig = cfg
	cm.logger.Info("initial configuration loaded", 
		"config_path", cm.configPath,
		"github_repo", fmt.Sprintf("%s/%s", cfg.GitHub.Owner, cfg.GitHub.Repo))

	return cfg, nil
}

// GetConfig returns the current configuration (thread-safe)
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	// Return a copy to prevent external modification
	if cm.currentConfig == nil {
		return nil
	}
	
	configCopy := *cm.currentConfig
	return &configCopy
}

// Subscribe registers a callback for configuration changes
func (cm *ConfigManager) Subscribe(callback ConfigChangeCallback) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.subscribers = append(cm.subscribers, callback)
}

// StartWatching begins monitoring the configuration file for changes
func (cm *ConfigManager) StartWatching(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.watchCancel != nil {
		cm.logger.Warn("configuration watching already started")
		return nil
	}

	watchCtx, cancel := context.WithCancel(ctx)
	cm.watchCancel = cancel

	go cm.watchConfigFile(watchCtx)
	
	cm.logger.Info("started configuration file watching", "config_path", cm.configPath)
	return nil
}

// StopWatching stops monitoring the configuration file
func (cm *ConfigManager) StopWatching() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.watchCancel != nil {
		cm.watchCancel()
		cm.watchCancel = nil
		cm.logger.Info("stopped configuration file watching")
	}
}

// ReloadConfig manually reloads the configuration from file
func (cm *ConfigManager) ReloadConfig() error {
	return cm.reloadFromFile()
}

// ValidateCurrentConfig validates the current configuration
func (cm *ConfigManager) ValidateCurrentConfig(ctx context.Context) *ValidationReport {
	cm.mu.RLock()
	config := cm.currentConfig
	cm.mu.RUnlock()

	if config == nil {
		return &ValidationReport{
			ErrorCount: 1,
			Failed: []*ValidationResult{
				{
					Rule: &ValidationRule{
						Field: "config",
					},
					Issue: "no configuration loaded",
					Fix:   "call LoadInitialConfig first",
				},
			},
		}
	}

	return cm.validator.ValidateConfig(ctx, config)
}

// DetectConfigurationDrift compares current running config with file-based config
func (cm *ConfigManager) DetectConfigurationDrift() ([]*ConfigChangeEvent, error) {
	cm.mu.RLock()
	currentConfig := cm.currentConfig
	cm.mu.RUnlock()

	if currentConfig == nil {
		return nil, fmt.Errorf("no current configuration loaded")
	}

	// Load config from file without network validation for drift detection
	fileConfig, err := LoadWithOptions(cm.configPath, true)
	if err != nil {
		return nil, fmt.Errorf("loading config from file for drift detection: %w", err)
	}

	// Compare configurations and detect changes
	changes := cm.detectChanges(currentConfig, fileConfig)

	if len(changes) > 0 {
		cm.logger.Warn("configuration drift detected",
			"changes_count", len(changes),
			"config_path", cm.configPath)
	}

	return changes, nil
}

// ApplyHotReloadableChanges applies only the changes that can be hot-reloaded
func (cm *ConfigManager) ApplyHotReloadableChanges(newConfig *Config) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.currentConfig == nil {
		return fmt.Errorf("no current configuration loaded")
	}

	oldConfig := cm.currentConfig
	changes := cm.detectChanges(oldConfig, newConfig)

	// Filter for hot-reloadable changes
	hotReloadableChanges := make([]*ConfigChangeEvent, 0)
	nonReloadableChanges := make([]*ConfigChangeEvent, 0)

	for _, change := range changes {
		if HotReloadableFields[change.Field] {
			hotReloadableChanges = append(hotReloadableChanges, change)
		} else {
			nonReloadableChanges = append(nonReloadableChanges, change)
		}
	}

	// Log non-reloadable changes
	if len(nonReloadableChanges) > 0 {
		cm.logger.Warn("detected non-hot-reloadable configuration changes",
			"changes_count", len(nonReloadableChanges),
			"requires_restart", true)
		
		for _, change := range nonReloadableChanges {
			cm.logger.Info("non-reloadable change detected",
				"field", change.Field,
				"old_value", change.OldValue,
				"new_value", change.NewValue,
				"action", "restart_required")
		}
	}

	// Apply hot-reloadable changes if any
	if len(hotReloadableChanges) > 0 {
		// Create a new config with only hot-reloadable changes applied
		updatedConfig := *oldConfig
		cm.applyChangesToConfig(&updatedConfig, newConfig, hotReloadableChanges)

		// Validate the updated configuration without network checks
		report := cm.validator.WithSkipNetwork(true).ValidateConfig(context.Background(), &updatedConfig)
		if report.ErrorCount > 0 {
			return fmt.Errorf("validation failed for hot-reloadable changes: %w", report.ToError())
		}

		// Notify subscribers
		for _, subscriber := range cm.subscribers {
			if err := subscriber(oldConfig, &updatedConfig); err != nil {
				cm.logger.Error("subscriber failed to handle config change",
					"error", err)
				return fmt.Errorf("subscriber failed: %w", err)
			}
		}

		// Update current configuration
		cm.currentConfig = &updatedConfig

		cm.logger.Info("applied hot-reloadable configuration changes",
			"changes_count", len(hotReloadableChanges))

		for _, change := range hotReloadableChanges {
			cm.logger.Info("configuration changed",
				"field", change.Field,
				"old_value", change.OldValue,
				"new_value", change.NewValue,
				"hot_reloaded", true)
		}
	}

	return nil
}

// GetConfigMetadata returns metadata about the current configuration
func (cm *ConfigManager) GetConfigMetadata() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metadata := map[string]interface{}{
		"config_path": cm.configPath,
		"loaded":      cm.currentConfig != nil,
		"watching":    cm.watchCancel != nil,
		"subscribers": len(cm.subscribers),
	}

	if cm.currentConfig != nil {
		metadata["github_repo"] = fmt.Sprintf("%s/%s", 
			cm.currentConfig.GitHub.Owner, 
			cm.currentConfig.GitHub.Repo)
		metadata["developer_agent_enabled"] = cm.currentConfig.Agents.Developer.Enabled
		metadata["creativity_enabled"] = cm.currentConfig.Creativity.Enabled
		metadata["decomposition_enabled"] = cm.currentConfig.Decomposition.Enabled
	}

	// Check file modification time
	if info, err := os.Stat(cm.configPath); err == nil {
		metadata["file_modified"] = info.ModTime()
	}

	return metadata
}

// watchConfigFile monitors the configuration file for changes
func (cm *ConfigManager) watchConfigFile(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	var lastModTime time.Time
	if info, err := os.Stat(cm.configPath); err == nil {
		lastModTime = info.ModTime()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(cm.configPath)
			if err != nil {
				cm.logger.Error("failed to stat config file", "error", err, "path", cm.configPath)
				continue
			}

			if info.ModTime().After(lastModTime) {
				lastModTime = info.ModTime()
				cm.logger.Info("configuration file changed, reloading", "path", cm.configPath)
				
				if err := cm.reloadFromFile(); err != nil {
					cm.logger.Error("failed to reload configuration", "error", err)
				}
			}
		}
	}
}

// reloadFromFile reloads configuration from the file
func (cm *ConfigManager) reloadFromFile() error {
	cm.logger.Info("reloading configuration from file", "path", cm.configPath)

	// Load new configuration without network validation for hot reload
	newConfig, err := LoadWithOptions(cm.configPath, true)
	if err != nil {
		cm.logger.Error("failed to load new configuration", "error", err)
		return fmt.Errorf("loading new config: %w", err)
	}

	// Try to apply hot-reloadable changes
	if err := cm.ApplyHotReloadableChanges(newConfig); err != nil {
		cm.logger.Error("failed to apply configuration changes", "error", err)
		return fmt.Errorf("applying configuration changes: %w", err)
	}

	return nil
}

// detectChanges compares two configurations and returns the differences
func (cm *ConfigManager) detectChanges(oldConfig, newConfig *Config) []*ConfigChangeEvent {
	changes := make([]*ConfigChangeEvent, 0)
	now := time.Now()

	// Compare specific fields that we care about
	configComparisons := []struct {
		field    string
		oldValue interface{}
		newValue interface{}
	}{
		{"logging.level", oldConfig.Logging.Level, newConfig.Logging.Level},
		{"logging.format", oldConfig.Logging.Format, newConfig.Logging.Format},
		{"logging.enable_correlation", oldConfig.Logging.EnableCorrelation, newConfig.Logging.EnableCorrelation},
		{"logging.sampling.enabled", oldConfig.Logging.Sampling.Enabled, newConfig.Logging.Sampling.Enabled},
		{"logging.sampling.rate", oldConfig.Logging.Sampling.Rate, newConfig.Logging.Sampling.Rate},
		{"github.poll_interval", oldConfig.GitHub.PollInterval, newConfig.GitHub.PollInterval},
		{"github.token", maskToken(oldConfig.GitHub.Token), maskToken(newConfig.GitHub.Token)},
		{"github.owner", oldConfig.GitHub.Owner, newConfig.GitHub.Owner},
		{"github.repo", oldConfig.GitHub.Repo, newConfig.GitHub.Repo},
		{"claude.api_key", maskToken(oldConfig.Claude.APIKey), maskToken(newConfig.Claude.APIKey)},
		{"claude.model", oldConfig.Claude.Model, newConfig.Claude.Model},
		{"claude.max_tokens", oldConfig.Claude.MaxTokens, newConfig.Claude.MaxTokens},
		{"agents.developer.enabled", oldConfig.Agents.Developer.Enabled, newConfig.Agents.Developer.Enabled},
		{"agents.developer.max_concurrent", oldConfig.Agents.Developer.MaxConcurrent, newConfig.Agents.Developer.MaxConcurrent},
		{"agents.developer.workspace_dir", oldConfig.Agents.Developer.WorkspaceDir, newConfig.Agents.Developer.WorkspaceDir},
		{"creativity.enabled", oldConfig.Creativity.Enabled, newConfig.Creativity.Enabled},
		{"creativity.idle_threshold_seconds", oldConfig.Creativity.IdleThresholdSeconds, newConfig.Creativity.IdleThresholdSeconds},
		{"creativity.suggestion_cooldown_seconds", oldConfig.Creativity.SuggestionCooldownSeconds, newConfig.Creativity.SuggestionCooldownSeconds},
		{"creativity.max_pending_suggestions", oldConfig.Creativity.MaxPendingSuggestions, newConfig.Creativity.MaxPendingSuggestions},
		{"decomposition.enabled", oldConfig.Decomposition.Enabled, newConfig.Decomposition.Enabled},
		{"decomposition.max_iteration_budget", oldConfig.Decomposition.MaxIterationBudget, newConfig.Decomposition.MaxIterationBudget},
		{"decomposition.max_subtasks", oldConfig.Decomposition.MaxSubtasks, newConfig.Decomposition.MaxSubtasks},
		{"metrics.enabled", oldConfig.Metrics.Enabled, newConfig.Metrics.Enabled},
		{"metrics.collection_interval", oldConfig.Metrics.CollectionInterval, newConfig.Metrics.CollectionInterval},
		{"workspace.cleanup.enabled", oldConfig.Workspace.Cleanup.Enabled, newConfig.Workspace.Cleanup.Enabled},
		{"workspace.cleanup.success_retention", oldConfig.Workspace.Cleanup.SuccessRetention, newConfig.Workspace.Cleanup.SuccessRetention},
		{"workspace.cleanup.failure_retention", oldConfig.Workspace.Cleanup.FailureRetention, newConfig.Workspace.Cleanup.FailureRetention},
	}

	for _, comp := range configComparisons {
		if !compareValues(comp.oldValue, comp.newValue) {
			changeType := ConfigModified
			if comp.oldValue == nil || isZeroValue(comp.oldValue) {
				changeType = ConfigAdded
			} else if comp.newValue == nil || isZeroValue(comp.newValue) {
				changeType = ConfigRemoved
			}

			changes = append(changes, &ConfigChangeEvent{
				Type:      changeType,
				Field:     comp.field,
				OldValue:  comp.oldValue,
				NewValue:  comp.newValue,
				Timestamp: now,
			})
		}
	}

	// Special handling for slices like watch_labels
	if !compareStringSlices(oldConfig.GitHub.WatchLabels, newConfig.GitHub.WatchLabels) {
		changes = append(changes, &ConfigChangeEvent{
			Type:      ConfigModified,
			Field:     "github.watch_labels",
			OldValue:  oldConfig.GitHub.WatchLabels,
			NewValue:  newConfig.GitHub.WatchLabels,
			Timestamp: now,
		})
	}

	return changes
}

// applyChangesToConfig applies specific changes to a configuration
func (cm *ConfigManager) applyChangesToConfig(target *Config, source *Config, changes []*ConfigChangeEvent) {
	for _, change := range changes {
		switch change.Field {
		case "logging.level":
			target.Logging.Level = source.Logging.Level
		case "logging.format":
			target.Logging.Format = source.Logging.Format
		case "logging.enable_correlation":
			target.Logging.EnableCorrelation = source.Logging.EnableCorrelation
		case "logging.sampling.enabled":
			target.Logging.Sampling.Enabled = source.Logging.Sampling.Enabled
		case "logging.sampling.rate":
			target.Logging.Sampling.Rate = source.Logging.Sampling.Rate
		case "github.poll_interval":
			target.GitHub.PollInterval = source.GitHub.PollInterval
		case "github.watch_labels":
			target.GitHub.WatchLabels = append([]string(nil), source.GitHub.WatchLabels...)
		case "creativity.idle_threshold_seconds":
			target.Creativity.IdleThresholdSeconds = source.Creativity.IdleThresholdSeconds
		case "creativity.suggestion_cooldown_seconds":
			target.Creativity.SuggestionCooldownSeconds = source.Creativity.SuggestionCooldownSeconds
		case "creativity.max_pending_suggestions":
			target.Creativity.MaxPendingSuggestions = source.Creativity.MaxPendingSuggestions
		case "metrics.enabled":
			target.Metrics.Enabled = source.Metrics.Enabled
		case "metrics.collection_interval":
			target.Metrics.CollectionInterval = source.Metrics.CollectionInterval
		case "workspace.cleanup.enabled":
			target.Workspace.Cleanup.Enabled = source.Workspace.Cleanup.Enabled
		case "workspace.cleanup.success_retention":
			target.Workspace.Cleanup.SuccessRetention = source.Workspace.Cleanup.SuccessRetention
		case "workspace.cleanup.failure_retention":
			target.Workspace.Cleanup.FailureRetention = source.Workspace.Cleanup.FailureRetention
		case "workspace.monitoring.disk_check_interval":
			target.Workspace.Monitoring.DiskCheckInterval = source.Workspace.Monitoring.DiskCheckInterval
		case "workspace.monitoring.cleanup_interval":
			target.Workspace.Monitoring.CleanupInterval = source.Workspace.Monitoring.CleanupInterval
		case "observability.performance.track_durations":
			target.Observability.Performance.TrackDurations = source.Observability.Performance.TrackDurations
		case "observability.performance.memory_monitoring":
			target.Observability.Performance.MemoryMonitoring = source.Observability.Performance.MemoryMonitoring
		}
	}
}

// Utility functions

func compareValues(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func isZeroValue(v interface{}) bool {
	switch val := v.(type) {
	case string:
		return val == ""
	case int:
		return val == 0
	case bool:
		return !val
	case time.Duration:
		return val == 0
	default:
		return v == nil
	}
}