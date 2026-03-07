# Configuration Schema Reference

This document provides the complete reference for all configuration options available in agentctl, including default values, validation rules, and environment variable mappings.

## Table of Contents

- [Overview](#overview)
- [GitHub Integration](#github-integration)
- [Claude AI](#claude-ai)
- [Agents](#agents)
- [State Management](#state-management)
- [Logging](#logging)
- [Creativity Engine](#creativity-engine)
- [Decomposition](#decomposition)
- [Workspace Management](#workspace-management)
- [Error Handling](#error-handling)
- [Shutdown](#shutdown)
- [Environment Variables](#environment-variables)
- [Validation Rules](#validation-rules)
- [Example Configurations](#example-configurations)

## Overview

The configuration file uses YAML format and supports environment variable expansion using `${VARIABLE}` syntax. All configuration values have sensible defaults, but some core fields like API keys are required.

## GitHub Integration

Configuration for GitHub repository monitoring and interaction.

```yaml
github:
  token: "${GITHUB_TOKEN}"           # Required: GitHub personal access token
  owner: "myorg"                     # Required: Repository owner/organization
  repo: "myrepo"                     # Required: Repository name
  poll_interval: "30s"              # Optional: Issue polling interval (default: 30s)
  watch_labels:                      # Optional: Labels to monitor (default: ["agent:ready"])
    - "agent:ready"
    - "agent:suggestion"
```

### GitHub Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `poll_interval` | `30s` | How often to check for new issues |
| `watch_labels` | `["agent:ready"]` | Issue labels that trigger agent activation |

### GitHub Validation Rules

- **token**: Required, must start with `ghp_` or `github_pat_`
- **owner**: Required, non-empty string
- **repo**: Required, non-empty string
- **poll_interval**: Must be between 5s and 1h
- Network validation checks token scopes and repository access

## Claude AI

Configuration for Anthropic's Claude API integration.

```yaml
claude:
  api_key: "${ANTHROPIC_API_KEY}"    # Required: Anthropic API key
  model: "claude-sonnet-4-20250514"  # Optional: Claude model to use
  max_tokens: 8192                   # Optional: Maximum tokens per request
```

### Claude Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `model` | `claude-sonnet-4-20250514` | Default Claude model for code generation |
| `max_tokens` | `8192` | Maximum tokens per API request |

### Claude Validation Rules

- **api_key**: Required, must start with `sk-ant-`
- **max_tokens**: Must not exceed 200,000 (Anthropic limit)
- Network validation checks API key authentication

## Agents

Configuration for autonomous agents that perform different tasks.

```yaml
agents:
  developer:
    enabled: false                   # Optional: Enable developer agent (default: false)
    max_concurrent: 1                # Optional: Max parallel workflows (default: 1)
    workspace_dir: "./workspaces"    # Optional: Workspace directory (default: "./workspaces")
    recovery:
      enabled: true                  # Optional: Enable recovery features (default: true)
      startup_validation: true       # Optional: Validate state on startup (default: true)
      auto_cleanup_orphaned: false   # Optional: Auto-cleanup orphaned workspaces (default: false)
      max_resume_age: "24h"          # Optional: Max age for resumable workflows (default: 24h)
      validation_interval: "1h"      # Optional: State validation interval (default: 1h)
```

### Agent Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `developer.enabled` | `false` | Developer agent is disabled by default |
| `developer.max_concurrent` | `1` | Conservative concurrency limit |
| `developer.workspace_dir` | `"./workspaces"` | Relative path for workspaces |

### Agent Validation Rules

- **max_concurrent**: Must be between 1-50 when agent is enabled
- **workspace_dir**: Must be writable directory, created if missing
- Path validation checks directory permissions

## State Management

Configuration for persistent state storage.

```yaml
state:
  backend: "file"                    # Optional: Storage backend (default: "file")
  dir: ".agentctl/state"            # Optional: State directory (default: ".agentctl/state")
```

### State Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `backend` | `"file"` | File-based state storage |
| `dir` | `".agentctl/state"` | Local state directory |

### State Validation Rules

- **dir**: Must be writable directory, created if missing
- **backend**: Currently only "file" is supported

## Logging

Comprehensive logging configuration with structured output support.

```yaml
logging:
  level: "info"                      # Optional: Log level (default: "info")
  format: "json"                     # Optional: Output format (default: "json")
  file_path: ""                      # Optional: Log file path (default: stdout)
  enable_correlation: true           # Optional: Enable correlation IDs (default: true)
  
  sampling:
    enabled: false                   # Optional: Enable log sampling (default: false)
    rate: 0.1                       # Optional: Sampling rate (default: 0.1)
    
  rotation:
    enabled: true                    # Optional: Enable log rotation (default: true)
    max_file_size_mb: 100           # Optional: Max file size in MB (default: 100)
    max_files: 10                    # Optional: Max rotated files (default: 10)
    max_age: "168h"                  # Optional: Max age before rotation (default: 7 days)
    compress_old: true               # Optional: Compress old log files (default: true)
    
  cleanup:
    enabled: true                    # Optional: Enable log cleanup (default: true)
    retention_days: 30               # Optional: Keep logs for N days (default: 30)
    min_free_disk_mb: 1024          # Optional: Min free disk space (default: 1GB)
    cleanup_interval: "24h"          # Optional: Cleanup check interval (default: 24h)
```

## Creativity Engine

Configuration for autonomous creative suggestion generation.

```yaml
creativity:
  enabled: false                     # Optional: Enable creativity engine (default: false)
  idle_threshold_seconds: 120        # Optional: Idle time before suggestions (default: 120)
  suggestion_cooldown_seconds: 300   # Optional: Cooldown between suggestions (default: 300)
  max_pending_suggestions: 1         # Optional: Max open suggestions (default: 1)
  max_rejection_history: 50          # Optional: Remember N rejections (default: 50)
```

### Creativity Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `enabled` | `false` | Creativity is disabled by default |
| `idle_threshold_seconds` | `120` | 2 minutes of inactivity triggers creativity |
| `suggestion_cooldown_seconds` | `300` | 5 minute cooldown between suggestions |
| `max_pending_suggestions` | `1` | One suggestion at a time |
| `max_rejection_history` | `50` | Remember 50 recent rejections |

### Creativity Validation Rules

- **idle_threshold_seconds**: Must be at least 30 seconds
- **suggestion_cooldown_seconds**: Must be at least 60 seconds
- **max_pending_suggestions**: Must be positive when enabled

## Decomposition

Configuration for complex issue decomposition into subtasks.

```yaml
decomposition:
  enabled: false                     # Optional: Enable decomposition (default: false)
  max_iteration_budget: 25          # Optional: Max iterations per workflow (default: 25)
  max_subtasks: 5                   # Optional: Max subtasks per issue (default: 5)
```

### Decomposition Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `enabled` | `false` | Decomposition is disabled by default |
| `max_iteration_budget` | `25` | Conservative iteration limit |
| `max_subtasks` | `5` | Reasonable subtask limit |

### Decomposition Validation Rules

- **max_subtasks**: Must be positive when enabled, should not exceed 20
- **max_iteration_budget**: Must be positive when enabled, should not exceed 100

## Workspace Management

Configuration for workspace lifecycle and resource limits.

```yaml
workspace:
  cleanup:
    enabled: true                    # Optional: Enable workspace cleanup (default: true)
    success_retention: "24h"         # Optional: Keep successful workspaces (default: 24h)
    failure_retention: "168h"        # Optional: Keep failed workspaces (default: 1 week)
    max_concurrent: 5                # Optional: Max concurrent cleanups (default: 5)
    
  limits:
    max_size_mb: 1024               # Optional: Max workspace size (default: 1GB)
    min_free_disk_mb: 2048          # Optional: Min free disk space (default: 2GB)
    
  monitoring:
    disk_check_interval: "5m"        # Optional: Disk space check interval (default: 5m)
    cleanup_interval: "1h"           # Optional: Cleanup check interval (default: 1h)
```

### Workspace Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `cleanup.enabled` | `true` | Automatic cleanup is enabled |
| `cleanup.success_retention` | `24h` | Keep successful workspaces for 1 day |
| `cleanup.failure_retention` | `168h` | Keep failed workspaces for 1 week |
| `limits.max_size_mb` | `1024` | 1GB workspace size limit |
| `limits.min_free_disk_mb` | `2048` | 2GB minimum free disk space |

### Workspace Validation Rules

- **min_free_disk_mb**: Must be larger than max_size_mb to prevent exhaustion
- **success_retention/failure_retention**: Must be at least 1 hour
- **max_size_mb**: Should not exceed 10GB (10,240MB)

## Error Handling

Configuration for retry mechanisms and circuit breakers.

```yaml
error_handling:
  retry:
    enabled: true                    # Optional: Enable retry logic (default: true)
    default:
      max_attempts: 3                # Optional: Max retry attempts (default: 3)
      base_delay: "1s"              # Optional: Initial delay (default: 1s)
      max_delay: "30s"              # Optional: Maximum delay (default: 30s)
      backoff_factor: 2.0           # Optional: Exponential backoff (default: 2.0)
      jitter_factor: 0.1            # Optional: Random jitter (default: 0.1)
      
  circuit_breaker:
    enabled: true                    # Optional: Enable circuit breakers (default: true)
    default:
      max_failures: 5               # Optional: Failure threshold (default: 5)
      timeout: "60s"                # Optional: Circuit open time (default: 60s)
      max_requests: 10              # Optional: Half-open test requests (default: 10)
      failure_ratio: 0.5            # Optional: Failure ratio threshold (default: 0.5)
```

## Shutdown

Configuration for graceful shutdown behavior.

```yaml
shutdown:
  timeout: "30s"                     # Optional: Shutdown timeout (default: 30s)
  cleanup_workspaces: true           # Optional: Clean up on shutdown (default: true)
  reset_claims: true                 # Optional: Reset issue claims (default: true)
```

## Environment Variables

All configuration values can be overridden using environment variables. Use `agentctl config env-vars` to see the complete mapping.

### Required Environment Variables

| Variable | Config Key | Description |
|----------|------------|-------------|
| `GITHUB_TOKEN` | `github.token` | GitHub personal access token |
| `GITHUB_OWNER` | `github.owner` | Repository owner/organization |
| `GITHUB_REPO` | `github.repo` | Repository name |
| `ANTHROPIC_API_KEY` | `claude.api_key` | Claude API key |

### Optional Environment Variables

| Variable | Config Key | Default | Description |
|----------|------------|---------|-------------|
| `CLAUDE_MODEL` | `claude.model` | `claude-sonnet-4-20250514` | Claude model |
| `WORKSPACE_DIR` | `agents.developer.workspace_dir` | `./workspaces` | Workspace directory |
| `STATE_DIR` | `state.dir` | `.agentctl/state` | State directory |
| `LOG_LEVEL` | `logging.level` | `info` | Log level |

## Validation Rules

Configuration validation includes:

1. **Required Field Validation**: Ensures essential fields are present
2. **Format Validation**: Validates token formats, URLs, durations
3. **Range Validation**: Checks numeric values are within acceptable limits
4. **Permission Validation**: Verifies directory write access
5. **Network Validation**: Tests API connectivity (optional)
6. **Logical Validation**: Ensures related settings make sense together

Use `agentctl config validate --config=config.yaml` to validate your configuration.

## Example Configurations

### Minimal Configuration

```yaml
# config.yaml - minimal required settings
github:
  token: "${GITHUB_TOKEN}"
  owner: "myorg"
  repo: "myrepo"
  
claude:
  api_key: "${ANTHROPIC_API_KEY}"
  
agents:
  developer:
    enabled: true
```

### Development Configuration

```yaml
# config-dev.yaml - development setup with debugging
github:
  token: "${GITHUB_TOKEN}"
  owner: "myorg"
  repo: "myrepo"
  poll_interval: "10s"  # More frequent polling for development
  
claude:
  api_key: "${ANTHROPIC_API_KEY}"
  model: "claude-sonnet-4-20250514"
  
agents:
  developer:
    enabled: true
    max_concurrent: 1
    workspace_dir: "./dev-workspaces"
    
logging:
  level: "debug"
  format: "text"  # More readable for development
  
workspace:
  cleanup:
    success_retention: "1h"   # Keep workspaces shorter for development
    failure_retention: "24h"
```

### Production Configuration

```yaml
# config-prod.yaml - production setup with monitoring
github:
  token: "${GITHUB_TOKEN}"
  owner: "myorg"
  repo: "myrepo"
  poll_interval: "60s"
  
claude:
  api_key: "${ANTHROPIC_API_KEY}"
  model: "claude-sonnet-4-20250514"
  max_tokens: 8192
  
agents:
  developer:
    enabled: true
    max_concurrent: 3
    workspace_dir: "/var/lib/agentctl/workspaces"
    recovery:
      enabled: true
      startup_validation: true
      auto_cleanup_orphaned: true
      
state:
  dir: "/var/lib/agentctl/state"
  
logging:
  level: "info"
  format: "json"
  file_path: "/var/log/agentctl/agentctl.log"
  enable_correlation: true
  rotation:
    enabled: true
    max_file_size_mb: 100
    max_files: 10
    max_age: "168h"
    
workspace:
  limits:
    max_size_mb: 2048      # 2GB workspace limit
    min_free_disk_mb: 5120 # 5GB minimum free space
  cleanup:
    enabled: true
    success_retention: "24h"
    failure_retention: "168h"  # 1 week for debugging
    
creativity:
  enabled: true
  idle_threshold_seconds: 300    # 5 minutes
  suggestion_cooldown_seconds: 900  # 15 minutes
  
decomposition:
  enabled: true
  max_iteration_budget: 30
  max_subtasks: 5

shutdown:
  timeout: "60s"  # Longer timeout for production cleanup
  cleanup_workspaces: true
  reset_claims: true
```

### High-Volume Configuration

```yaml
# config-highvolume.yaml - high-throughput setup
github:
  token: "${GITHUB_TOKEN}"
  owner: "bigorg"
  repo: "bigproject"
  poll_interval: "15s"   # Faster polling for high activity
  
claude:
  api_key: "${ANTHROPIC_API_KEY}"
  model: "claude-sonnet-4-20250514"
  max_tokens: 4096  # Smaller tokens for faster responses
  
agents:
  developer:
    enabled: true
    max_concurrent: 10  # Higher concurrency
    workspace_dir: "/mnt/fast-ssd/agentctl/workspaces"
    
workspace:
  limits:
    max_size_mb: 512      # Smaller workspaces for faster cleanup
    min_free_disk_mb: 10240  # 10GB minimum free space
  cleanup:
    enabled: true
    success_retention: "2h"   # Quick cleanup for high volume
    failure_retention: "24h"
    max_concurrent: 10        # Parallel cleanup
  monitoring:
    disk_check_interval: "1m"  # More frequent monitoring
    cleanup_interval: "15m"    # More frequent cleanup
    
logging:
  level: "warn"  # Reduce log volume
  sampling:
    enabled: true
    rate: 0.1    # Sample 10% of debug logs
```

## Troubleshooting

### Common Validation Errors

1. **"github.token: required field is empty"**
   - Set the `GITHUB_TOKEN` environment variable
   - Or specify `token` directly in the config file

2. **"claude.api_key: API key format appears invalid"**
   - Ensure the key starts with `sk-ant-`
   - Get a new key from https://console.anthropic.com/

3. **"workspace.limits.min_free_disk_mb: should be larger than max_size_mb"**
   - Increase `min_free_disk_mb` or decrease `max_size_mb`
   - Recommended ratio is 2:1 (min_free should be twice max_size)

4. **"agents.developer.workspace_dir: directory is not writable"**
   - Check directory permissions: `chmod 755 /path/to/workspace`
   - Ensure the directory exists or can be created

### Validation Commands

```bash
# Validate configuration without starting agents
agentctl config validate --config=config.yaml

# Show all default values
agentctl config show-defaults

# Display environment variable mappings
agentctl config env-vars

# Skip network validation for faster checking
agentctl config validate --config=config.yaml --skip-network
```

## Database Configuration

Configuration for PostgreSQL database connections and performance optimization.

```yaml
database:
  host: "${DB_HOST}"                 # Optional: Database host (default: "localhost")
  port: 5432                         # Optional: Database port (default: 5432)
  name: "${DB_NAME}"                 # Required: Database name
  user: "${DB_USER}"                 # Required: Database user
  password: "${DB_PASSWORD}"         # Required: Database password
  sslmode: "require"                 # Optional: SSL mode (default: "disable")
  
  # Legacy connection pool settings (deprecated - use pool.* instead)
  max_open_conns: 25                 # Optional: Max connections (default: 25)
  max_idle_conns: 5                  # Optional: Max idle connections (default: 5)
  conn_max_lifetime: "5m"            # Optional: Connection lifetime (default: 5m)
  
  # Advanced connection pool configuration
  pool:
    max_connections: 50              # Optional: Maximum concurrent connections (default: 25)
    min_connections: 10              # Optional: Minimum idle connections (default: 5)
    max_idle_time: "30m"            # Optional: Close idle connections after (default: 30m)
    health_check_period: "1m"        # Optional: Health check interval (default: 1m)
    max_conn_lifetime: "1h"          # Optional: Maximum connection lifetime (default: 1h)
    max_conn_idle_time: "15m"        # Optional: Maximum idle time per connection (default: 15m)
    
  # Query performance monitoring
  performance:
    slow_query_threshold: "100ms"    # Optional: Log queries slower than this (default: 100ms)
    query_timeout: "30s"             # Optional: Maximum query execution time (default: 30s)
    enable_query_log: true           # Optional: Enable query performance logging (default: true)
    enable_metrics: true             # Optional: Enable Prometheus metrics (default: true)
```

### Database Environment Variables

| Variable | Config Key | Required | Description |
|----------|------------|----------|-------------|
| `DB_HOST` | `database.host` | No | Database host |
| `DB_PORT` | `database.port` | No | Database port |
| `DB_NAME` | `database.name` | Yes | Database name |
| `DB_USER` | `database.user` | Yes | Database username |
| `DB_PASSWORD` | `database.password` | Yes | Database password |

### Database Defaults

| Field | Default Value | Description |
|-------|---------------|-------------|
| `host` | `"localhost"` | Local database connection |
| `port` | `5432` | Standard PostgreSQL port |
| `sslmode` | `"disable"` | No SSL for local development |
| `pool.max_connections` | `25` | Conservative connection limit |
| `pool.min_connections` | `5` | Maintain minimum availability |
| `pool.max_idle_time` | `30m` | Clean up idle connections |
| `pool.health_check_period` | `1m` | Regular health checks |
| `performance.slow_query_threshold` | `100ms` | Log slow queries |
| `performance.query_timeout` | `30s` | Prevent runaway queries |

### Database Validation Rules

- **Connection Pool Settings**:
  - `max_connections` must be greater than `min_connections`
  - `max_connections` should not exceed 200 (PostgreSQL limits)
  - `min_connections` must be at least 1
  
- **Timeout Settings**:
  - `query_timeout` must be at least 1 second
  - `slow_query_threshold` must be positive
  - `max_idle_time` must be at least 1 minute
  
- **SSL Configuration**:
  - `sslmode` must be one of: "disable", "require", "verify-ca", "verify-full"
  - Production environments should use "require" or stronger

### Database Example Configurations

#### Development Database Configuration

```yaml
database:
  host: "localhost"
  name: "agentframework_dev"
  user: "developer"
  password: "${DB_PASSWORD}"
  sslmode: "disable"  # OK for local development
  
  pool:
    max_connections: 10     # Small pool for development
    min_connections: 2
    max_idle_time: "15m"
    
  performance:
    slow_query_threshold: "50ms"  # Stricter for development
    enable_query_log: true
    enable_metrics: true
```

#### Production Database Configuration

```yaml
database:
  host: "${DB_HOST}"
  port: 5432
  name: "${DB_NAME}"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  sslmode: "require"      # Require SSL in production
  
  pool:
    max_connections: 50         # Higher capacity for production
    min_connections: 10         # Ensure availability
    max_idle_time: "15m"        # Aggressive cleanup
    health_check_period: "30s"  # Frequent health checks
    max_conn_lifetime: "2h"     # Periodic rotation
    max_conn_idle_time: "10m"   # Quick idle cleanup
    
  performance:
    slow_query_threshold: "100ms"
    query_timeout: "30s"
    enable_query_log: true
    enable_metrics: true
```

#### High-Performance Database Configuration

```yaml
database:
  host: "${DB_HOST}"
  name: "${DB_NAME}"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  sslmode: "require"
  
  pool:
    max_connections: 100        # High capacity
    min_connections: 20         # Large baseline
    max_idle_time: "10m"        # Fast cleanup
    health_check_period: "15s"  # Very frequent checks
    max_conn_lifetime: "1h"     # Faster rotation
    max_conn_idle_time: "5m"    # Minimal idle time
    
  performance:
    slow_query_threshold: "50ms"   # Stricter performance
    query_timeout: "15s"           # Faster timeout
    enable_query_log: true
    enable_metrics: true
```

### Database Troubleshooting

#### Connection Pool Issues

1. **"failed to acquire connection from pool"**
   ```bash
   # Check pool configuration
   curl -s http://localhost:8080/health | jq '.database.pool'
   
   # Increase max_connections if utilization > 80%
   ```

2. **High connection pool utilization**
   ```yaml
   database:
     pool:
       max_connections: 100  # Increase from 25
       min_connections: 20   # Increase proportionally
   ```

#### Query Performance Issues

1. **Too many slow queries**
   ```bash
   # Check slow query logs
   grep "slow database query" logs/app.log
   
   # Reduce slow_query_threshold to identify more queries
   ```

2. **Database timeouts**
   ```yaml
   database:
     performance:
       query_timeout: "60s"  # Increase from 30s
       slow_query_threshold: "200ms"  # Relax threshold
   ```

#### SSL/TLS Issues

1. **SSL connection failures in production**
   ```yaml
   database:
     sslmode: "require"  # Ensure SSL is required
     # Or for stricter validation:
     sslmode: "verify-full"
   ```

2. **Development SSL certificate issues**
   ```yaml
   database:
     sslmode: "disable"  # OK for local development only
   ```