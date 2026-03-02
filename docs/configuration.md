# Configuration

Configuration is loaded from a YAML file using Viper. Environment variables are expanded in all string values using `${VAR}` syntax.

## Loading

```go
cfg, err := config.Load("configs/config.yaml")
```

`Load()` reads the file, expands environment variables, unmarshals into `Config`, and validates required fields.

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
  enable_correlation: true             # Enable correlation ID tracking.
  sampling:
    enabled: false                     # Enable log sampling.
    rate: 1.0                          # Sampling rate (0.0–1.0).
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

`config.Validate()` checks:
- Required fields are non-empty (returns aggregated errors via `errors.Join`)
- Applies defaults for optional fields with zero values
- Called automatically by `config.Load()`

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
