# Architecture

## System Overview

OpenAgentFramework is an autonomous development agent framework. It polls GitHub for issues labeled `agent:ready`, uses Claude to analyze and implement solutions, then creates pull requests for human review.

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   GitHub     │◄───►│ Orchestrator │────►│  Developer   │
│  (Issues,    │     │ (errgroup)   │     │   Agent      │
│   PRs,       │     └──────────────┘     └──────┬───────┘
│   Labels)    │                                 │
└──────────────┘                           ┌─────▼─────┐
                                           │  Claude    │
       ┌──────────┐                        │  (Analyze, │
       │ Workspace│◄───────────────────────│  Implement)│
       │ (git)    │                        └───────────┘
       └──────────┘
```

## Package Structure

```
cmd/agentctl/          CLI entry point (Cobra)
internal/
  agent/               Agent interface, BaseAgent, Dependencies DI, registry
  orchestrator/        Runs agents concurrently with errgroup
  developer/           Developer agent: workflow, decomposition, prompts
  claude/              Claude API client, conversation manager, tool definitions
  ghub/                GitHub client interface, issue/PR/branch/comment ops, poller
  gitops/              Git clone, checkout, commit, push (go-git)
  config/              Config loading (Viper), validation, defaults
  state/               WorkflowState enum, AgentWorkState, file-based Store
  creativity/          Idle-mode suggestion engine
  errors/              Retry with backoff, circuit breakers, error classification
  observability/       Structured logger, correlation IDs, metrics
configs/               YAML configuration files
docs/                  Project documentation
```

## Data Flow

### Issue Processing

1. **Poller** (`ghub.Poller`) polls GitHub for issues with `agent:ready` label
2. **Handler** (`DeveloperAgent.handleIssues`) receives matched issues
3. **Claim** assigns self, adds `agent:claimed`, removes `agent:ready`
4. **Workspace** clones repo, creates branch `agent/issue-<N>`
5. **Context** `gatherRepoContext()` builds file tree + go.mod for Claude
6. **Analyze** Claude produces implementation plan (single-turn, no tools)
7. **Decompose** (optional) if plan exceeds iteration budget, splits into subtask issues
8. **Implement** Claude executes plan via tool loop (multi-turn with 6 tools)
9. **Commit** stages all, commits `feat: implement #N - <title>`, pushes
10. **PR** creates pull request referencing the issue
11. **Review** adds `agent:in-review` label; human reviews

### Tool Loop

During implementation, the `Conversation` manager runs a loop:

```
User message → Claude response
  ├─ text only → done (return response)
  └─ tool_use  → execute tools → append results → next iteration
```

Each iteration is one API round-trip. Multiple tool calls in the same response count as a single iteration. The loop is capped by `maxIter` (default 20, or `decomposition.max_iteration_budget`).

## Dependency Injection

All components receive shared dependencies through `agent.Dependencies`:

```go
type Dependencies struct {
    Config           *config.Config
    GitHub           ghub.Client
    Claude           *claude.Client
    Store            state.Store
    Logger           *slog.Logger
    StructuredLogger *observability.StructuredLogger
    Metrics          *observability.Metrics
    ErrorManager     *errors.Manager
}
```

This struct is constructed in `cli/start.go` and passed to agent constructors. The initialization order is:

1. Config → Logger → StructuredLogger → Metrics
2. ErrorManager (with observability chained)
3. GitHubClient and ClaudeClient (with error handling and observability chained)
4. FileStore
5. Dependencies assembled → Agent constructors → Orchestrator

### Agent Registry

The `agent.Registry` provides a factory pattern for agent instantiation:

```go
type AgentFactory func(deps Dependencies) (Agent, error)
```

`Registry.Register()` maps `AgentType` → factory function. `Registry.Create()` instantiates an agent by type. Thread-safe via `sync.RWMutex`.

## Concurrency Model

The `Orchestrator` uses `errgroup` to run multiple agents concurrently. Each agent runs its own polling loop in a separate goroutine. Context cancellation propagates to all agents for graceful shutdown.

### Signal Handling

The CLI uses `signal.NotifyContext()` to handle `SIGINT` and `SIGTERM` for graceful shutdown. Each agent also runs a background `Heartbeat()` goroutine (60s interval) for liveness logging.

### Error Resilience

All external API calls (GitHub, Claude) are wrapped with retry and circuit breaker protection through the `errors.Manager`. The pattern used across `ghub` and `claude` packages:

1. Public method checks for `errorManager`
2. If present, wraps the core logic in `agentErrors.Execute()` with a retryer
3. If absent, calls the core logic directly

This allows the error handling to be optional and configured at startup.

## Agent Personas

| Agent | Status | Description |
|-------|--------|-------------|
| Developer (`developer`) | Active | Monitors issues, writes code, creates PRs |
| QA (`qa`) | Planned | Will run tests and validate PRs |
| DevManager (`devmanager`) | Planned | Will coordinate and prioritize work |

Only the Developer agent is implemented. QA and DevManager are defined as `AgentType` constants.

## Label Protocol

Labels drive agent coordination through GitHub:

| Label | Meaning |
|-------|---------|
| `agent:ready` | Issue available for agent to claim |
| `agent:claimed` | Agent has claimed the issue |
| `agent:in-progress` | Agent actively working on implementation |
| `agent:in-review` | PR created, awaiting human review |
| `agent:failed` | Agent encountered an error |
| `agent:suggestion` | Creativity engine suggestion (pending human review) |
| `agent:suggestion-rejected` | Rejected suggestion (remembered to avoid re-suggesting) |
| `agent:subtask` | Child issue from decomposition |
| `agent:epic` | Parent issue that was decomposed |

## Key Libraries

| Library | Purpose |
|---------|---------|
| `anthropic-sdk-go` | Claude API client |
| `go-github/v68` | GitHub API client |
| `go-git/v5` | Git operations |
| `cobra` | CLI framework |
| `viper` | Configuration loading |
| `errgroup` | Concurrent agent execution |
| `testify` | Test assertions and mocks |
