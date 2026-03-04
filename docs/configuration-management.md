# Configuration Management

This document provides a comprehensive guide to the advanced configuration management features in agentctl, including runtime configuration management, environment-specific configurations, hot-reload capabilities, and comprehensive validation.

## Overview

The configuration management system provides:

- **Hot-reload capability** for non-critical configuration changes
- **Environment-specific configurations** with automatic overlays
- **Comprehensive validation** with detailed error reporting
- **Configuration drift detection** and monitoring
- **Runtime configuration management** with subscriber notifications

## Configuration Manager

### Basic Usage

```go
import "github.com/gaskaj/DeveloperAndQAAgent/internal/config"

// Create configuration manager
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
manager := config.NewConfigManager("config.yaml", logger)

// Load initial configuration
cfg, err := manager.LoadInitialConfig()
if err != nil {
    log.Fatal(err)
}

// Start configuration file watching
ctx := context.Background()
err = manager.StartWatching(ctx)
if err != nil {
    log.Fatal(err)
}
```

### Configuration Change Notifications

```go
// Subscribe to configuration changes
manager.Subscribe(func(oldConfig, newConfig *config.Config) error {
    // Handle configuration change
    if oldConfig.Logging.Level != newConfig.Logging.Level {
        // Update logger level
        log.Printf("Log level changed from %s to %s", 
            oldConfig.Logging.Level, newConfig.Logging.Level)
    }
    return nil
})
```

### Configuration Drift Detection

```go
// Check for configuration drift
changes, err := manager.DetectConfigurationDrift()
if err != nil {
    log.Fatal(err)
}

for _, change := range changes {
    fmt.Printf("Configuration drift detected: %s changed from %v to %v\n",
        change.Field, change.OldValue, change.NewValue)
}
```

## Environment-Specific Configuration

### Environment Types

The system supports four predefined environment types:

1. **Development** (`dev`, `development`)
   - Relaxed validation rules
   - Debug logging allowed
   - Fast polling intervals
   - All features enabled for testing

2. **Staging** (`stage`, `staging`)
   - Moderate validation rules
   - Requires metrics
   - Balanced between flexibility and security

3. **Production** (`prod`, `production`)
   - Strict validation rules
   - Requires security features
   - No debug logging
   - Comprehensive error handling required

4. **Test** (`test`, `testing`)
   - Minimal validation
   - Supports mock configurations
   - Optimized for automated testing

### Environment Configuration Files

Create environment-specific overlays:

```yaml
# config.dev.yaml
github:
  poll_interval: 15s  # Faster polling for development

logging:
  level: debug
  enable_correlation: true

agents:
  developer:
    max_concurrent: 1  # Simple for development

creativity:
  enabled: true
  idle_threshold_seconds: 60  # Faster creativity

metrics:
  enabled: true
  collection_interval: 15s
```

```yaml
# config.prod.yaml
github:
  poll_interval: 60s  # Conservative for production

logging:
  level: info
  enable_correlation: true  # Required
  structured_logging:
    enabled: true  # Required
  file_path: /var/log/agentctl/agent.log

agents:
  developer:
    max_concurrent: 3

creativity:
  enabled: false  # Typically disabled

metrics:
  enabled: true  # Required
  collection_interval: 60s

error_handling:
  retry:
    enabled: true  # Required
  circuit_breaker:
    enabled: true  # Required
```

### Loading Environment Configuration

```go
// Load configuration with environment overlay
envManager := config.NewEnvironmentManager()
cfg, err := envManager.LoadEnvironmentConfig("config.yaml", "production")
if err != nil {
    log.Fatal(err)
}
```

### Environment Variable Overrides

The system automatically binds environment variables:

| Environment Variable | Configuration Path |
|---------------------|-------------------|
| `GITHUB_TOKEN` | `github.token` |
| `GITHUB_OWNER` | `github.owner` |
| `GITHUB_REPO` | `github.repo` |
| `ANTHROPIC_API_KEY` | `claude.api_key` |
| `CLAUDE_MODEL` | `claude.model` |
| `WORKSPACE_DIR` | `agents.developer.workspace_dir` |
| `STATE_DIR` | `state.dir` |
| `LOG_LEVEL` | `logging.level` |
| `METRICS_ENABLED` | `metrics.enabled` |

Additional variables follow the pattern `AGENT_SECTION_FIELD` (e.g., `AGENT_LOGGING_FORMAT`).

## Validation System

### Validation Levels

- **Error**: Must be fixed before the configuration can be used
- **Warning**: Should be addressed but won't prevent startup
- **Info**: Informational notices about configuration choices

### Validation Categories

- **Required**: Essential fields that must be present
- **Format**: Correct format for tokens, URLs, etc.
- **Permissions**: File system access and permissions
- **Network**: API connectivity and authentication
- **Limits**: Numeric ranges and resource constraints
- **Compatibility**: Cross-field dependencies
- **Security**: Security-related configuration issues
- **Performance**: Performance impact warnings

### Custom Validation

```go
// Create validator with custom rules
validator := config.NewValidator()

// Skip network validation for faster testing
validator = validator.WithSkipNetwork(true)

// Run validation
report := validator.ValidateConfig(context.Background(), cfg)

// Check results
if report.ErrorCount > 0 {
    for _, result := range report.Failed {
        fmt.Printf("Error in %s: %s\n", result.Rule.Field, result.Issue)
        if result.Fix != "" {
            fmt.Printf("Fix: %s\n", result.Fix)
        }
    }
}
```

## Hot-Reload Configuration

### Hot-Reloadable vs Non-Hot-Reloadable Fields

**Hot-Reloadable** (no restart required):
- Logging configuration (`logging.*`)
- GitHub polling settings (`github.poll_interval`, `github.watch_labels`)
- Creativity settings (`creativity.*`)
- Metrics collection (`metrics.*`)
- Workspace cleanup settings (`workspace.cleanup.*`)
- Observability settings (`observability.performance.*`)

**Non-Hot-Reloadable** (restart required):
- API keys (`github.token`, `claude.api_key`)
- Repository settings (`github.owner`, `github.repo`)
- Agent enablement (`agents.*.enabled`)
- Workspace directories (`agents.*.workspace_dir`)
- State storage configuration (`state.*`)

### Manual Hot-Reload

```go
// Trigger manual reload
err := manager.ReloadConfig()
if err != nil {
    log.Printf("Failed to reload config: %v", err)
}

// Apply specific changes
newConfig := &config.Config{ /* ... */ }
err = manager.ApplyHotReloadableChanges(newConfig)
if err != nil {
    log.Printf("Failed to apply changes: %v", err)
}
```

## CLI Integration

### Configuration Validation

```bash
# Basic validation
agentctl validate --config config.yaml

# Environment-specific validation
agentctl validate --config config.yaml --env production

# Strict mode (warnings as errors)
agentctl validate --config config.yaml --env production --strict

# Skip network checks
agentctl validate --config config.yaml --skip-network

# Full validation with network checks
agentctl validate --config config.yaml --full
```

### Configuration Management Commands

```bash
# Show default values
agentctl config show-defaults --format yaml

# Show environment variable mappings
agentctl config env-vars --format table

# Validate configuration
agentctl config validate --config config.yaml --env prod
```

### Sample Output

```
🔍 Performing comprehensive validation with network checks...
✅ Configuration is valid!
🌍 Environment: production
📊 Validation Summary: 23 rules checked, 21 passed, 2 warnings, 0 errors

📋 Configuration Summary:
   GitHub Repository: myorg/myrepo
   Claude Model: claude-sonnet-4-20250514
   Max Tokens: 8192
   Workspace Directory: /var/lib/agentctl/workspaces
   State Directory: /var/lib/agentctl/state
   Developer Agent: ✅ Enabled
   Creativity Mode: ⭕ Disabled
   Issue Decomposition: ✅ Enabled
   GitHub Poll Interval: 60s
   Watching Labels: [agent:ready]

🔐 Environment Variables:
   GITHUB_TOKEN: ✅ Set (ghp_...7890)
   ANTHROPIC_API_KEY: ✅ Set (sk-a...890)

🌐 Network Connectivity: Validated
```

## Best Practices

### Development Environment

- Use fast polling intervals for rapid feedback
- Enable debug logging and all features
- Use local workspace directories
- Enable creativity mode for testing

```yaml
# config.dev.yaml
github:
  poll_interval: 15s
logging:
  level: debug
  enable_correlation: true
creativity:
  enabled: true
  idle_threshold_seconds: 60
agents:
  developer:
    workspace_dir: ./dev-workspaces
```

### Production Environment

- Use conservative polling intervals
- Require all security features
- Use system directories for storage
- Disable experimental features

```yaml
# config.prod.yaml
github:
  poll_interval: 60s
logging:
  level: info
  enable_correlation: true
  structured_logging:
    enabled: true
  file_path: /var/log/agentctl/agent.log
creativity:
  enabled: false
agents:
  developer:
    workspace_dir: /var/lib/agentctl/workspaces
    max_concurrent: 3
error_handling:
  retry:
    enabled: true
  circuit_breaker:
    enabled: true
```

### Configuration Security

1. **Never commit secrets** to version control
2. **Use environment variables** for sensitive data
3. **Validate configurations** in CI/CD pipelines
4. **Use strict mode** for production validation
5. **Monitor configuration drift** in production

```bash
# CI/CD validation pipeline
agentctl config validate --config config.yaml --env production --strict
```

### Configuration Monitoring

```go
// Monitor configuration changes in production
manager.Subscribe(func(oldConfig, newConfig *config.Config) error {
    // Log all configuration changes
    changes := detectChanges(oldConfig, newConfig)
    for _, change := range changes {
        logConfigChange(change)
    }
    
    // Alert on critical changes
    if isCriticalChange(change) {
        sendAlert(change)
    }
    
    return nil
})
```

## Troubleshooting

### Common Issues

**Configuration validation fails with network errors:**
```bash
# Skip network validation
agentctl config validate --skip-network
```

**Hot-reload not working:**
- Check if the field is hot-reloadable
- Verify file watching is enabled
- Check log output for reload errors

**Environment overlay not applied:**
- Verify overlay file exists
- Check file syntax with `agentctl validate`
- Ensure environment name is correct

**Configuration drift detected:**
- Compare running config with file
- Use `agentctl config validate` to check changes
- Restart if non-hot-reloadable changes are needed

### Debugging Configuration Issues

```go
// Get configuration metadata
metadata := manager.GetConfigMetadata()
fmt.Printf("Configuration metadata: %+v\n", metadata)

// Validate current configuration
report := manager.ValidateCurrentConfig(context.Background())
if report.ErrorCount > 0 {
    fmt.Printf("Validation errors: %d\n", report.ErrorCount)
}

// Check for drift
changes, err := manager.DetectConfigurationDrift()
if len(changes) > 0 {
    fmt.Printf("Configuration drift detected: %d changes\n", len(changes))
}
```

This comprehensive configuration management system provides enterprise-grade capabilities while maintaining ease of use for development environments.