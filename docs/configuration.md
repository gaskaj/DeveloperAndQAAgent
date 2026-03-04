# Configuration

Configuration is loaded from a YAML file using Viper. Environment variables are expanded in all string values using `${VAR}` syntax.

> 📖 **Complete Reference**: See [Configuration Schema Reference](configuration-schema.md) for the complete documentation of all configuration options, defaults, validation rules, and examples.

## Quick Start

For detailed configuration reference, validation rules, and examples, see the [Configuration Schema Reference](configuration-schema.md).

## Loading

```go
cfg, err := config.Load("configs/config.yaml")
```

`Load()` reads the file, expands environment variables, unmarshals into `Config`, and validates required fields.

## Configuration Validation and Management

The comprehensive configuration management system provides validation, hot-reload capabilities, and environment-specific configurations:

### CLI Commands

```bash
# Basic validation
agentctl config validate --config=config.yaml

# Environment-specific validation
agentctl config validate --config=config.yaml --env=prod --strict

# Show all default values in YAML format
agentctl config show-defaults --format=yaml

# Display environment variable mappings
agentctl config env-vars

# Skip network validation for faster checking
agentctl config validate --config=config.yaml --skip-network

# Comprehensive validation with network checks
agentctl config validate --config=config.yaml --full
```

### Configuration Management Features

#### Runtime Configuration Management
- **Hot-reload capability** for non-critical settings (logging levels, polling intervals, etc.)
- **Configuration drift detection** comparing running vs file-based config
- **Safe configuration updates** with rollback capability
- **Configuration change event notifications** for subscribers

#### Environment-Specific Configuration
- **Environment overlays** (dev, staging, prod) with automatic merging
- **Systematic environment variable overrides** with standardized naming
- **Environment validation rules** ensuring prod configs don't leak to dev
- **Security requirement enforcement** for production environments

#### Comprehensive Validation System
- **Required field validation** with helpful error messages
- **Format validation** for tokens, URLs, and durations  
- **Range validation** for numeric values and limits
- **Permission validation** for directory access
- **Network validation** for API connectivity (optional)
- **Cross-field validation** for interdependent settings
- **Environment-specific validation** rules

### Enhanced Error Reporting

The new validation system provides structured error messages with actionable guidance:

```
❌ Configuration validation failed:
  • github.token: token format appears invalid
    Fix: Use a personal access token from GitHub settings
    Example: ghp_xxxxxxxxxxxxxxxxxxxx
  • workspace.limits.min_free_disk_mb: should be larger than max_size_mb to prevent disk exhaustion
    Fix: Set min_free_disk_mb to at least 2048MB
    Example: 2048
```

### Hot-Reloadable Configuration Fields

The following fields can be updated without restarting the agent:

| Field | Description |
|-------|-------------|
| `logging.level` | Log verbosity level |
| `logging.format` | Log output format |
| `logging.enable_correlation` | Correlation ID tracking |
| `github.poll_interval` | Issue polling frequency |
| `github.watch_labels` | Labels to monitor |
| `creativity.idle_threshold_seconds` | Creativity trigger delay |
| `creativity.suggestion_cooldown_seconds` | Suggestion rate limiting |
| `metrics.enabled` | Metrics collection |
| `workspace.cleanup.*` | Workspace cleanup settings |

Critical fields like API keys, repository settings, and agent configuration require a restart.

### Environment-Specific Configuration

#### Configuration File Structure

```bash
configs/
├── config.yaml           # Base configuration
├── config.dev.yaml       # Development overrides
├── config.prod.yaml      # Production overrides
└── config.local.yaml     # Local development overrides (gitignored)
```

#### Loading Environment Configuration

```bash
# Load with environment overlay
agentctl start --config=config.yaml --env=production

# Development environment
agentctl start --config=config.yaml --env=development
```

#### Environment Validation Rules

Each environment has specific validation rules:

**Development:**
- Allows debug logging
- Permits faster polling intervals  
- Relaxed concurrency limits
- All features enabled for testing

**Production:**
- Requires structured logging
- Enforces security features (correlation, error handling)
- Prohibits debug logging
- Requires metrics and observability

**Staging:**
- Balance between dev flexibility and prod security
- Requires metrics but allows moderate debug info
- Intermediate validation strictness

## Full YAML Reference

```yaml
github:
  token: ${GITHUB_TOKEN}              # Required. GitHub PAT with repo scope.
  owner: myorg                         # Required. Repository owner.
  repo: myrepo                         # Required. Repository name.
  poll_interval: 30s                   # Polling interval (default: 30s).
  watch_labels:                        # Labels to watch for new issues.
    - agent:ready

claude:
  api_key: ${ANTHROPIC_API_KEY}        # Required. Anthropic API key.
  model: claude-sonnet-4-20250514      # Model ID (default: claude-sonnet-4-20250514).
  max_tokens: 8192                     # Max output tokens (default: 8192).

agents:
  developer:
    enabled: true                      # Enable the developer agent.
    max_concurrent: 1                  # Max concurrent issue processing (default: 1).
    workspace_dir: ./workspaces        # Directory for cloned repos (default: ./workspaces).

state:
  backend: file                        # State backend (default: file).
  dir: .agentctl/state                 # State directory (default: .agentctl/state).

logging:
  level: info                          # Log level: debug, info, warn, error.
  format: text                         # Log format: text or json.
  file_path: ./logs/agent.log          # Log file location (optional).
  enable_correlation: true             # Enable correlation ID tracking.
  sampling:
    enabled: false                     # Enable log sampling.
    rate: 1.0                          # Sampling rate (0.0–1.0).
  rotation:
    enabled: true                      # Enable log rotation.
    max_file_size_mb: 100              # Rotate when file exceeds size (MB).
    max_files: 10                      # Keep N rotated files.
    max_age: 720h                      # Delete files older than duration (30d).
    compress_old: true                 # Gzip rotated files.
    check_interval: 1h                 # Rotation check frequency.
  cleanup:
    enabled: true                      # Enable log cleanup.
    retention_days: 90                 # Delete logs older than N days.
    min_free_disk_mb: 1024             # Cleanup when disk space below threshold (MB).
    cleanup_interval: 24h              # Cleanup check frequency.
    archive_before_delete: true        # Compress before deletion.
  structured_logging:
    enabled: true                      # Enable structured logging.
    format: json                       # Structured log format.
    include_caller: false              # Include caller file:line.
    include_stack_trace: false          # Include stack traces.
    correlation:
      enabled: true                    # Enable correlation context.
      auto_generate: true              # Auto-generate correlation IDs.
      include_workflow_stage: true     # Include workflow stage in logs.
      include_agent_metadata: true     # Include agent metadata.
      propagate_github_context: true   # Propagate GitHub context.
    workflow_tracking:
      enabled: true                    # Track workflow transitions.
      track_handoffs: true             # Track agent handoffs.
      track_decisions: true            # Track decision points.
      include_performance: true        # Include performance data.
      track_tool_usage: true           # Track tool usage.
    performance:
      track_durations: true            # Track operation durations.
      memory_snapshots: true           # Enable memory snapshots.
      llm_metrics: true                # Track LLM call metrics.
      workflow_timing: true            # Track workflow stage timing.
    filtering:
      debug_sampling_rate: 1.0         # Debug log sampling rate.
      include_errors: true             # Always include error logs.
      include_warnings: true           # Always include warning logs.
    export:
      enabled: false                   # Enable log export.
      field_mappings:                  # Field mappings for external systems.
        elk:
          timestamp: "@timestamp"
          correlation_id: "trace.id"
        datadog:
          correlation_id: "dd.trace_id"
  multi_agent_observability:
    cross_agent_tracking: true         # Track across agents.
    communication_patterns: true       # Log communication patterns.
    performance_comparison: true       # Compare agent performance.
    workflow_efficiency: true          # Track workflow efficiency.
    alerting:
      lost_correlation_threshold: 0.1  # Alert when correlation loss exceeds %.
      handoff_timeout_seconds: 300     # Alert on slow handoffs.
      stage_stall_threshold_seconds: 600  # Alert on stalled stages.
      tool_failure_rate_threshold: 0.5 # Alert on high tool failure rate.

metrics:
  enabled: true                        # Enable metrics collection.
  collection_interval: 30s             # Collection interval.
  export:
    prometheus:
      enabled: false                   # Enable Prometheus endpoint.
      port: 9090                       # Prometheus port.
      path: /metrics                   # Metrics path.
    logs:
      enabled: true                    # Export metrics to logs.
      interval: 60s                    # Export interval.

observability:
  tracing:
    enabled: false                     # Enable distributed tracing.
    sample_rate: 1.0                   # Trace sampling rate.
  health:
    enabled: true                      # Enable health endpoint.
    port: 8080                         # Health check port.
    path: /healthz                     # Health check path.
  performance:
    track_durations: true              # Track operation durations.
    memory_monitoring: false           # Monitor memory usage.
    interval: 30s                      # Monitoring interval.

creativity:
  enabled: false                       # Must be explicitly enabled.
  idle_threshold_seconds: 120          # Seconds idle before creativity mode (default: 120).
  suggestion_cooldown_seconds: 300     # Cooldown between suggestions (default: 300).
  max_pending_suggestions: 1           # Max open suggestion issues (default: 1).
  max_rejection_history: 50            # Max rejected titles to remember (default: 50).

decomposition:
  enabled: true                        # Enable issue decomposition.
  max_iteration_budget: 250            # Max API iterations per issue (default: 25).
  max_subtasks: 5                      # Max subtasks per decomposition (default: 5).

error_handling:
  retry:
    enabled: true                      # Enable retry logic.
    default:                           # Default retry policy.
      max_attempts: 3
      base_delay: 1s
      max_delay: 30s
      backoff_factor: 2.0
      jitter_factor: 0.1
    policies:                          # Per-operation policies (override default).
      claude_api:
        max_attempts: 5
        base_delay: 2s
        max_delay: 60s
  circuit_breaker:
    enabled: true                      # Enable circuit breakers.
    default:                           # Default circuit breaker config.
      max_failures: 5
      timeout: 60s
      max_requests: 1
      failure_ratio: 0.5
      min_requests: 3
    breakers:                          # Per-service circuit breakers.
      github_api:
        max_failures: 10
        timeout: 120s
```

## Environment Variables

All string values support `${VAR}` expansion. Common variables:

| Variable | Used In |
|----------|---------|
| `GITHUB_TOKEN` | `github.token` |
| `ANTHROPIC_API_KEY` | `claude.api_key` |

## Required Fields

| Field | Description |
|-------|-------------|
| `github.token` | GitHub personal access token |
| `github.owner` | Repository owner |
| `github.repo` | Repository name |
| `claude.api_key` | Anthropic API key |

## Defaults

Applied by `config.Validate()` when fields are zero-valued:

| Field | Default |
|-------|---------|
| `claude.model` | `claude-sonnet-4-20250514` |
| `claude.max_tokens` | `8192` |
| `github.poll_interval` | `30s` |
| `state.backend` | `file` |
| `state.dir` | `.agentctl/state` |
| `agents.developer.max_concurrent` | `1` |
| `agents.developer.workspace_dir` | `./workspaces` |
| `creativity.idle_threshold_seconds` | `120` |
| `creativity.suggestion_cooldown_seconds` | `300` |
| `creativity.max_pending_suggestions` | `1` |
| `creativity.max_rejection_history` | `50` |
| `decomposition.max_iteration_budget` | `25` |
| `decomposition.max_subtasks` | `5` |

## Validation

The configuration system provides comprehensive validation with multiple levels:

### Basic Validation (`config.Validate()`)
- ✅ Required fields are non-empty  
- ✅ Token format validation (GitHub, Claude)
- ✅ Interdependency checks (e.g., `max_concurrent > 0` when agent enabled)
- ✅ Applies defaults for optional fields
- ✅ Returns structured error messages with fixes and examples

### Enhanced Validation (`config.ValidateWithContext()`)
- ✅ All basic validation checks
- ✅ Workspace directory permissions (create/write tests)
- ✅ State directory accessibility  
- ✅ Optional network validation (GitHub/Claude API connectivity)
- ✅ Token scope verification (repo, read:user)
- ✅ Repository access verification

### Validation Methods

```go
// Basic validation (fast, no network calls)
cfg, err := config.Load(configPath)

// Skip network validation for faster startup
cfg, err := config.LoadWithOptions(configPath, true)

// Full validation with network checks
cfg, err := config.LoadWithSchemaValidation(configPath)
```

### CLI Validation

```bash
# Basic validation
agentctl validate --config configs/config.yaml

# Full validation with network connectivity tests
agentctl validate --config configs/config.yaml --full

# Skip network tests (faster)
agentctl validate --config configs/config.yaml --skip-network
```

### Error Messages

Structured validation errors provide actionable feedback:

```
github.token: token format appears invalid. Fix: Use a personal access token from GitHub settings. Example: ghp_xxxxxxxxxxxxxxxxxxxx

agents.developer.max_concurrent: must be greater than 0 when developer agent is enabled. Fix: Set to a positive integer (recommended: 1-3). Example: 1
```

### Troubleshooting Common Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `github.token is required` | Missing environment variable | Set `GITHUB_TOKEN` environment variable |
| `token format appears invalid` | Wrong token format | Use personal access token (ghp_xxx) or fine-grained token (github_pat_xxx) |
| `repository not accessible` | Permission or existence issue | Verify repo exists and token has access |
| `directory is not writable` | Filesystem permissions | Fix directory permissions or choose different path |
| `API key authentication failed` | Invalid Claude key | Check API key in Anthropic Console |

## Configuration Files

The `configs/` directory contains:

| File | Purpose |
|------|---------|
| `config.yaml` | Main configuration (uses env vars, gitignored) |
| `config.example.yaml` | Template with all options documented |
| `logging.yaml` | Logging-specific configuration |
| `structured_logging.yaml` | Structured logging and multi-agent observability config |

## State Persistence

The `FileStore` (state backend `file`) stores agent state as JSON files in the configured state directory (default `.agentctl/state/`). Files are named `{agentType}.json`. `Load()` returns `nil` (not an error) if the file doesn't exist, allowing fresh starts.

The `WorkflowState` enum has 13 states (see `internal/state/models.go`): `idle`, `claim`, `workspace`, `analyze`, `decompose`, `implement`, `commit`, `pr`, `validation`, `review`, `complete`, `failed`, `creative_thinking`. The `validation` state was added in PR #96 for PR check monitoring and auto-fix. PR validation uses hardcoded defaults via `DefaultPRValidationOptions()` — no YAML config entries are needed.
