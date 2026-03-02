# Error Recovery and State Consistency

This document describes the comprehensive error recovery and state consistency validation system implemented for the DeveloperAndQAAgent.

## Overview

The error recovery system addresses critical gaps in production readiness by providing:

1. **Orphaned State Recovery**: Detection and cleanup of abandoned work
2. **State Corruption Detection**: Validation of consistency between file-based state and external systems
3. **Workflow Resumption**: Intelligent resumption of interrupted workflows
4. **Cross-System State Drift**: Detection and resolution of state drift

## Components

### State Validator (`internal/state/validator.go`)

The StateValidator provides comprehensive state consistency validation and reconciliation:

```go
type StateValidator struct {
    store  Store
    github ghub.Client
    logger *slog.Logger
}
```

**Key Methods:**
- `ValidateWorkState()`: Validates consistency of a specific agent work state
- `DetectOrphanedWork()`: Scans all stored states to find orphaned work items
- `ReconcileState()`: Attempts to reconcile inconsistencies in work state

**Validation Types:**
- Issue state consistency (GitHub issue vs local state)
- Branch consistency (naming patterns, existence)
- PR consistency (state, references)
- Workspace consistency (directory, git state)

### Recovery Manager (`internal/developer/recovery.go`)

The RecoveryManager handles workflow recovery and cleanup operations:

```go
type RecoveryManager struct {
    deps      agent.Dependencies
    validator *state.StateValidator
    logger    *slog.Logger
}
```

**Key Methods:**
- `AttemptResume()`: Analyzes work state and creates resumption plan
- `CleanupOrphanedWork()`: Cleans up orphaned work items
- `ValidateWorkspaceConsistency()`: Validates workspace directory consistency

**Recovery Types:**
- `RecoveryTypeCleanup`: Safe cleanup for early-stage work
- `RecoveryTypeResume`: Attempt intelligent resumption
- `RecoveryTypeManual`: Require manual intervention

### Startup Validator (`internal/agent/startup.go`)

Enhanced agent initialization with validation and recovery:

```go
type StartupValidator struct {
    deps      Dependencies
    validator *state.StateValidator
    logger    *slog.Logger
}
```

**Key Methods:**
- `ValidateAndRecoverStartup()`: Comprehensive startup validation and recovery
- `PerformPeriodicValidation()`: Background validation of agent states

## Configuration

Recovery behavior is controlled through configuration:

```yaml
agents:
  developer:
    recovery:
      enabled: true
      startup_validation: true
      auto_cleanup_orphaned: true
      max_resume_age: "24h"
      validation_interval: "1h"
      consistency:
        validate_on_startup: true
        validate_periodically: true
        reconcile_drift: true
```

**Configuration Options:**
- `enabled`: Enable recovery system
- `startup_validation`: Run validation at agent startup
- `auto_cleanup_orphaned`: Automatically cleanup orphaned work
- `max_resume_age`: Maximum age for resumable work
- `validation_interval`: Interval for periodic validation
- `reconcile_drift`: Automatically reconcile state drift

## Orphaned Work Detection

Work is considered orphaned if:

1. More than 1 hour since last update and not in terminal state
2. Has error and hasn't been updated in 30+ minutes
3. Stuck in intermediate state (claim/analyze) for 30+ minutes

Terminal states (complete, failed, idle) are never considered orphaned.

## State Drift Types

The system detects several types of state drift:

1. **Issue State Drift**: Local state vs GitHub issue state
2. **Branch State Drift**: Branch existence vs expected state
3. **PR State Drift**: PR state vs local expectations
4. **Git State Drift**: Local git repository vs expected state

## Risk Assessment

Resumption risk is assessed based on:

- Age of work (older = riskier)
- Number and severity of validation issues
- State drifts detected
- Presence of errors
- Advanced workflow states

Risk levels: `RiskLow`, `RiskMedium`, `RiskHigh`, `RiskCritical`

## Metrics and Observability

The system provides comprehensive metrics:

- `orphaned_work_detected_total`: Count of orphaned work detected
- `state_drift_resolved_total`: Count of state drifts resolved  
- `recovery_actions_total`: Count of recovery actions taken
- `validation_runs_total`: Count of validation runs

## Integration Points

- **Orchestrator startup**: Run validation before starting agent pools
- **Developer agent initialization**: Check for resumable work
- **Periodic health checks**: Background validation every N minutes
- **Error boundaries**: Trigger validation after workflow failures

## Example Usage

```go
// Create validator
validator := state.NewStateValidator(store, githubClient, logger)

// Detect orphaned work
orphaned, err := validator.DetectOrphanedWork(ctx)

// Create recovery manager
recoveryManager := developer.NewRecoveryManager(deps, validator)

// Attempt recovery
plan, err := recoveryManager.AttemptResume(ctx, workState)
```

## Benefits

- **Reliability**: Agents can recover from crashes and resume work intelligently
- **Data Integrity**: Automated detection and resolution of state inconsistencies
- **Operational Excellence**: Reduced manual intervention for stuck workflows
- **Debugging**: Clear visibility into recovery actions and state validation results
- **Scalability**: Foundation for multi-agent coordination without state conflicts

## Testing

Comprehensive test coverage includes:

- Unit tests for state validation logic
- Mock implementations for external dependencies
- Integration tests for recovery workflows
- Configuration validation tests

## Future Enhancements

Potential future improvements:

1. **Multi-Agent Coordination**: Cross-agent state validation
2. **Advanced Workspace Validation**: Git repository state checks
3. **Predictive Recovery**: ML-based prediction of failure scenarios
4. **Recovery Analytics**: Detailed analysis of recovery patterns
5. **Custom Recovery Strategies**: Configurable recovery behavior per issue type