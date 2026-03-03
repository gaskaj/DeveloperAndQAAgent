# Code Conventions

## Go Conventions

### Error Handling

- Return errors with context using `fmt.Errorf("doing X: %w", err)`
- Use `errors.Join()` for aggregating multiple validation errors (see `config/validate.go`)
- Use `errors.Is()` for sentinel error checks (see `claude.IsMaxIterationsError`)
- Non-critical failures log and continue; critical failures return the error

### Error Classification (PR #79)

External API errors are classified into typed errors for retry decisions:

| ErrorType | Retryable | Examples |
|-----------|-----------|----------|
| `network` | Yes | Connection refused, host unreachable |
| `timeout` | Yes | Context deadline exceeded, net.Error timeout |
| `rate_limit` | Yes | HTTP 429 |
| `api` | Yes | HTTP 5xx server errors |
| `temporary` | Yes | Unknown errors (default) |
| `authentication` | No | HTTP 401, 403 |
| `permanent` | No | HTTP 404, 400 |

`ClassifyError()` inspects HTTP status codes, timeout/network error patterns, and error messages to produce an `AgentCommunicationError` with the appropriate type and retry hint.

### Core/Wrapper Pattern

GitHub and Claude client methods that hit external APIs follow a pattern:

```go
func (c *Client) DoThing(ctx, ...) (Result, error) {
    if c.errorManager != nil {
        return agentErrors.Execute(ctx, retryer, func(...) { return c.doThingCore(ctx, ...) })
    }
    return c.doThingCore(ctx, ...)
}
```

This keeps error handling optional and composable.

### Naming

- Interfaces: verb-noun (`Client`, `Store`, `Agent`)
- Implementations: prefix with package context (`GitHubClient`, `DeveloperAgent`)
- Constructors: `New()` or `NewX()` returning the interface type
- Methods: short, descriptive names (`Run`, `Send`, `Poll`)

### Interfaces

Interfaces are defined in the package that *uses* them, not the package that implements them:

- `ghub.Client` — defined in `ghub/client.go`, implemented by `GitHubClient`
- `state.Store` — defined in `state/store.go`, implemented by `FileStore`
- `agent.Agent` — defined in `agent/agent.go`, implemented by `DeveloperAgent`

### Context Propagation

All long-running and I/O operations accept `context.Context` as the first parameter. Context carries:
- Cancellation signals (from orchestrator shutdown)
- Correlation context (from `observability.EnsureCorrelationContext`)
- Workflow stage metadata

## Logging

Uses `log/slog` throughout:

```go
d.logger().Info("processing issue", "number", issueNum, "title", issueTitle)
d.logger().Error("failed to process issue", "issue", issue.GetNumber(), "error", err)
```

- Each agent creates a child logger: `d.Deps.Logger.With("agent", "developer")`
- Components add their own context: `logger.With("component", "creativity")`
- Structured loggers (`observability.StructuredLogger`) are used for workflow events, handoffs, and decision points

See [structured-logging.md](structured-logging.md) for the observability framework.

## Testing

### Framework

Uses `testify` for assertions:

```go
assert.NoError(t, err)
assert.Equal(t, expected, actual)
require.NotNil(t, result)  // stops test on failure
```

### Coverage Standards

All code must meet minimum coverage thresholds:

| Package Type | Minimum Coverage |
|--------------|------------------|
| Critical (`claude`, `ghub`, `developer`) | 85% |
| Infrastructure (`config`, `state`, `workspace`) | 80% |
| Utility (`errors`, `observability`) | 75% |
| Default | 70% |

### Coverage Best Practices

- **Test both success and error paths**
- **Cover all public functions and methods**
- **Test critical state transitions**
- **Include edge cases and boundary conditions**
- **Use table-driven tests for multiple scenarios**

```go
func TestWorkflowTransition(t *testing.T) {
    tests := []struct {
        name          string
        currentState  State
        event         Event
        expectedState State
        shouldError   bool
    }{
        {
            name:          "valid transition",
            currentState:  StateIdle,
            event:         EventClaim,
            expectedState: StateClaim,
            shouldError:   false,
        },
        {
            name:          "invalid transition",
            currentState:  StateIdle,
            event:         EventCommit,
            expectedState: StateIdle,
            shouldError:   true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            workflow := NewWorkflow()
            workflow.SetState(tt.currentState)
            
            err := workflow.ProcessEvent(tt.event)
            
            if tt.shouldError {
                assert.Error(t, err)
                assert.Equal(t, tt.currentState, workflow.State())
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expectedState, workflow.State())
            }
        })
    }
}
```

### Critical Path Testing

These code paths require 100% coverage:
- Error handling and retry logic
- State machine transitions
- API error responses
- Data persistence and recovery

### Mocks

Test doubles are defined alongside interfaces or in test files. The `ghub` tests use mock implementations of `Client`. The `creativity` package defines `GitHubClient` and `AIClient` interfaces with adapter types for testing.

### Temp Directories

Use `t.TempDir()` for file-based tests (see `config/config_test.go`, `state/filestore_test.go`).

### Test Files

Test files live next to their source files:
- `developer/workflow_test.go`
- `config/config_test.go`
- `claude/conversation_test.go`
- `errors/retry_test.go`

## Package Patterns

### Interface + Implementation

Each package typically exports an interface and a concrete implementation:

```go
// Interface in client.go
type Client interface { ... }

// Implementation in the same or separate file
type GitHubClient struct { ... }
func NewClient(...) *GitHubClient { ... }
```

### Builder Pattern

Several types use method chaining for optional configuration:

```go
client := claude.NewClient(apiKey, model, maxTokens).
    WithObservability(logger, metrics).
    WithErrorHandling(errorManager)
```

### Dependency Injection

The `agent.Dependencies` struct bundles all shared dependencies. This avoids global state and makes testing straightforward — inject mock dependencies in tests.

## Git Conventions

- Branch naming: `agent/issue-<N>`
- Commit messages: `feat: implement #<N> - <issue title>`
- Commit author: `DeveloperAgent <agent@devqaagent.local>`
- PR title matches commit message
- PR body includes `Closes #<N>` and the implementation plan
- Clone uses `x-access-token` username for HTTP auth with GitHub PAT

## Makefile Targets

### Core Development

| Target | Description |
|--------|-------------|
| `build` | Build the binary |
| `test` | Run unit tests with race detector |
| `lint` | Run linter (requires `golangci-lint`) |
| `fmt` | Format all Go files |
| `run` | Build and run with example config |
| `clean` | Remove build artifacts and coverage files |

### Coverage Analysis

| Target | Description |
|--------|-------------|
| `coverage` | Complete coverage analysis (unit + integration + reports) |
| `coverage-unit` | Run unit tests with coverage |
| `coverage-integration` | Run integration tests with coverage |
| `coverage-html` | Generate HTML coverage report |
| `coverage-report` | Generate detailed coverage analysis |
| `coverage-badge` | Generate coverage badge for README |
| `coverage-gates` | Run quality gate validation |
| `test-coverage` | Complete coverage analysis with quality gates |

### Example Usage

```bash
# Before committing
make fmt lint test

# Full development cycle
make coverage

# Check if your changes meet quality gates
make coverage-gates
```
