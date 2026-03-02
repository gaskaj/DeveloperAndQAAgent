# Structured Logging with Correlation IDs

## Overview

This document covers the structured logging and observability framework built into the agent system. It enables full traceability across the Developer → QA → Human feedback loop using correlation IDs, workflow tracking, and agent handoff chains.

For configuration options, see [configuration.md](configuration.md) (the `logging`, `metrics`, and `observability` sections). For the `observability` package API, see [package-reference.md](package-reference.md).

## Observability Package

**Location**: `internal/observability/`

The `observability` package provides three core components:

| Component | File | Purpose |
|-----------|------|---------|
| `CorrelationContext` | `correlation.go` | Context-based correlation ID propagation with workflow stages, handoff chains, and metadata |
| `StructuredLogger` | `logger.go` | Structured event logging (agent lifecycle, workflow transitions, handoffs, decisions, LLM calls) |
| `Metrics` | `metrics.go` | Performance metrics collection (LLM call durations, token counts, workflow timing) |

### Key Functions

| Function | Description |
|----------|-------------|
| `EnsureCorrelationContext(ctx, agentType, issueID)` | Create or reuse correlation context |
| `WithWorkflowStage(ctx, stage)` | Set the current workflow stage |
| `WithHandoff(ctx, from, to, trigger, payloadSize)` | Record an agent handoff |
| `WithMetadata(ctx, key, value)` | Add metadata to correlation context |

## Key Features

### 1. Enhanced Correlation Context

The system now tracks enriched correlation context throughout the entire workflow:

```go
type CorrelationContext struct {
    CorrelationID string                `json:"correlation_id"`
    CreatedAt     time.Time             `json:"created_at"`
    AgentType     string                `json:"agent_type"`
    WorkflowStage WorkflowStage         `json:"workflow_stage"`
    IssueID       int                   `json:"issue_id"`
    HandoffChain  []HandoffInfo         `json:"handoff_chain"`
    Metadata      map[string]string     `json:"metadata"`
    StageEntries  []StageEntry         `json:"stage_entries"`
}
```

### 2. Workflow Stage Tracking

All 12 workflow stages are tracked with timing information:

```
start → claim → analyze → decompose → implement → commit → pr → review → complete
                                                                          ↓
idle ←──────────────────────────────────────────────────────────────── handoff
                                                                          ↓
                                                                       error
```

**WorkflowStage constants** (from `correlation.go`): `Start`, `Claim`, `Analyze`, `Decompose`, `Implement`, `Commit`, `PR`, `Review`, `Complete`, `Handoff`, `Idle`, `Error`

### Additional Logger Methods (PRs #77, #79, #83)

Beyond the core logging methods, `StructuredLogger` also provides:

| Method | Purpose |
|--------|---------|
| `LogToolUsage(ctx, tool, duration, success)` | Track tool execution with timing |
| `LogPerformanceMetric(ctx, name, value, unit, labels)` | Arbitrary performance metrics |
| `LogRetryAttempt(ctx, operation, attempt, max, err)` | Retry attempt tracking |
| `LogRetrySuccess(ctx, operation, attempt)` | Successful retry |
| `LogRetryExhausted(ctx, operation, attempts, err)` | All retries exhausted |
| `LogRetryNonRetryable(ctx, operation, attempt, err)` | Non-retryable error encountered |
| `LogRetryDelay(ctx, operation, attempt, delay, err)` | Delay before next retry |
| `LogCircuitBreakerStateChange(ctx, name, from, to)` | Circuit breaker state transition |
| `LogCircuitBreakerRejection(ctx, name, state)` | Request rejected by circuit breaker |

### Correlation ID Format

Correlation IDs are generated as 16-character hex strings (8 random bytes). Falls back to `"unknown"` if random generation fails.

### 3. Agent Handoff Traceability

Complete audit trail of agent-to-agent handoffs:

```json
{
  "event_type": "agent_handoff",
  "from_agent": "developer",
  "to_agent": "qa",
  "trigger": "implementation_complete",
  "payload_size_bytes": 1024,
  "handoff_sequence": 2,
  "previous_handoffs": ["orchestrator->developer", "developer->qa"]
}
```

## Usage Examples

### Basic Correlation Context Setup

```go
// Create correlation context for a new workflow
ctx := observability.NewCorrelationContext("developer", 123)
ctx = observability.WithMetadata(ctx, "issue_title", "Fix authentication bug")
ctx = observability.WithMetadata(ctx, "priority", "high")
```

### Workflow Stage Transitions

```go
// Transition between workflow stages
ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageAnalyze)
// ... perform analysis work ...
ctx = observability.WithWorkflowStage(ctx, observability.WorkflowStageImplement)
```

### Agent Handoffs

```go
// Record agent handoff with payload information
ctx = observability.WithHandoff(ctx, "developer", "qa", "ready_for_review", payloadSize)
```

### Structured Logging

```go
// Enhanced structured logging with correlation context
structuredLogger.LogAgentStart(ctx, "developer", "processing issue #123")
structuredLogger.LogWorkflowTransition(ctx, 123, "analyze", "implement", "requirements_clear")
structuredLogger.LogDecisionPoint(ctx, "developer", "proceed_to_implementation", "all tests pass", metadata)
```

## Log Output Examples

### Agent Lifecycle Logs

```json
{
  "timestamp": "2026-03-01T23:35:57.416Z",
  "level": "INFO",
  "msg": "agent_lifecycle",
  "correlation_id": "b5dd0689a7fa0e1a",
  "event_type": "agent_start",
  "agent_type": "developer",
  "message": "processing issue #123",
  "issue_id": 123,
  "workflow_stage": "start",
  "handoff_count": 0,
  "meta_issue_title": "Fix authentication bug",
  "meta_priority": "high"
}
```

### Workflow Transitions

```json
{
  "timestamp": "2026-03-01T23:36:15.234Z",
  "level": "INFO", 
  "msg": "workflow_transition",
  "correlation_id": "b5dd0689a7fa0e1a",
  "event_type": "workflow_transition",
  "issue_id": 123,
  "from_state": "analyze",
  "to_state": "implement",
  "reason": "requirements_clear",
  "agent_type": "developer",
  "workflow_duration_ms": 18234,
  "stage_duration_ms": 5500,
  "total_stages": 3,
  "completed_stages": 2
}
```

### Agent Handoffs

```json
{
  "timestamp": "2026-03-01T23:37:42.156Z",
  "level": "INFO",
  "msg": "agent_handoff", 
  "correlation_id": "b5dd0689a7fa0e1a",
  "event_type": "agent_handoff",
  "from_agent": "developer",
  "to_agent": "qa",
  "trigger": "implementation_complete",
  "payload_size_bytes": 2048,
  "issue_id": 123,
  "current_workflow_stage": "review",
  "handoff_sequence": 1,
  "workflow_duration_ms": 87156,
  "previous_handoffs": ["orchestrator->developer"]
}
```

### Decision Points

```json
{
  "timestamp": "2026-03-01T23:36:08.445Z",
  "level": "INFO",
  "msg": "decision_point",
  "correlation_id": "b5dd0689a7fa0e1a", 
  "agent_type": "developer",
  "decision": "proceed_to_implementation",
  "reasoning": "all unit tests pass, requirements are clear",
  "issue_id": 123,
  "complexity_score": 7.5,
  "estimated_duration_ms": 300000
}
```

### Performance Metrics

```json
{
  "timestamp": "2026-03-01T23:37:25.678Z",
  "level": "INFO",
  "msg": "performance_metric",
  "correlation_id": "b5dd0689a7fa0e1a",
  "metric_name": "llm_call_duration", 
  "value": 2500,
  "unit": "ms",
  "label_model": "claude-3-sonnet",
  "label_operation": "code_generation",
  "label_status": "success"
}
```

## Configuration

### Basic Configuration (configs/structured_logging.yaml)

```yaml
structured_logging:
  enabled: true
  format: json
  correlation:
    enabled: true
    auto_generate: true
    include_workflow_stage: true
    include_agent_metadata: true
  
  workflow_tracking:
    enabled: true
    track_handoffs: true
    track_decisions: true
    include_performance: true
    
  performance:
    track_durations: true
    memory_snapshots: true
    llm_metrics: true
    workflow_timing: true
```

### Integration with Logging Systems

#### ELK Stack Field Mappings
```yaml
export:
  field_mappings:
    elk:
      timestamp: "@timestamp"
      level: "log.level" 
      correlation_id: "trace.id"
      agent_type: "agent.type"
      workflow_stage: "workflow.stage"
```

#### Datadog Integration
```yaml
export:
  field_mappings:
    datadog:
      correlation_id: "dd.trace_id"
      agent_type: "service.name"
      workflow_stage: "resource"
```

## Benefits

### 1. End-to-End Traceability
- Track complete workflows from initial issue to final output
- Follow requests through Developer → QA → Human feedback loop  
- Correlate logs from different agents working on the same task

### 2. Performance Monitoring
- Identify bottlenecks between agent handoffs
- Track operation durations per agent and workflow stage
- Monitor LLM API call performance and token usage

### 3. Debugging & Troubleshooting
- Quickly find all logs related to a specific issue or workflow
- Understand decision points and reasoning at each stage
- Identify where workflows get stuck or fail

### 4. Operational Observability  
- Monitor agent health and performance
- Track success/failure rates across workflow stages
- Generate alerts for stuck workflows or performance degradation

### 5. Compliance & Auditing
- Maintain complete audit trails for agent decisions
- Export structured logs to monitoring systems (ELK, Datadog, etc.)
- Generate reports on agent performance and workflow efficiency

## Best Practices

1. **Always use correlation context**: Ensure every workflow starts with `EnsureCorrelationContext()`

2. **Add meaningful metadata**: Include issue titles, priorities, and other context in metadata

3. **Log decision points**: Use `LogDecisionPoint()` for important agent decisions with reasoning

4. **Track workflow transitions**: Log every stage transition with clear reasons

5. **Monitor performance**: Use performance metrics to identify optimization opportunities

6. **Structure error handling**: Include correlation IDs in error logs for easier debugging

This implementation provides the foundation for comprehensive observability in multi-agent systems while maintaining backward compatibility with existing logging infrastructure.