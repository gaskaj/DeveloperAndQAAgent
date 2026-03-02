# Package Reference

## cmd/agentctl

CLI entry point.

| File | Description |
|------|-------------|
| `main.go` | Calls `cli.Execute()` |

## internal/agent

Agent interface, base type, and dependency injection.

| File | Key Types / Functions |
|------|----------------------|
| `agent.go` | `Agent` interface (`Type`, `Run`, `Status`), `BaseAgent`, `Dependencies`, `StatusReport`, `AgentType` constants (`TypeDeveloper`, `TypeQA`, `TypeDevManager`) |
| `lifecycle.go` | `Heartbeat()` — periodic agent heartbeat logging; `RunWithContext()` — cancellable context wrapper |
| `registry.go` | `Registry` struct, `NewRegistry()`, `Register()`, `Create()`, `Types()`, `AgentFactory` type |

## internal/orchestrator

Concurrent agent execution.

| File | Key Types / Functions |
|------|----------------------|
| `orchestrator.go` | `Orchestrator` struct, `New()`, `Run()` (uses errgroup), `WithObservability()`, `Status()` |
| `health.go` | Agent health check logic |

## internal/developer

Developer agent implementation — the main workflow engine.

| File | Key Types / Functions |
|------|----------------------|
| `developer.go` | `DeveloperAgent`, `New()`, `Run()`, `Type()`, `Status()` |
| `workflow.go` | `processIssue()`, `claimIssue()`, `analyze()`, `implement()`, `createToolExecutor()`, `gatherRepoContext()`, `extractFilePaths()`, `preReadFiles()`, `executeEditFile()`, `executeSearchFiles()` |
| `decompose.go` | `decompose()`, `reactiveDecompose()`, `processChildIssues()`, `parseSubtasks()`, `parseComplexityResult()`, `parseEstimatedIterations()`, `createChildIssues()` |
| `prompts.go` | `SystemPrompt`, `AnalyzePrompt`, `ImplementPrompt`, `ComplexityEstimatePrompt`, `DecomposePrompt`, `ReactiveDecomposePrompt` |

## internal/claude

Claude API integration.

| File | Key Types / Functions |
|------|----------------------|
| `client.go` | `Client` struct, `NewClient()`, `SendMessage()`, `SendMessageWithTools()`, `WithObservability()`, `WithErrorHandling()`, `ExtractText()` |
| `conversation.go` | `Conversation` struct, `NewConversation()`, `Send()` (tool loop), `ToolExecutor` type, `maxToolResultLen`, `ErrMaxIterations`, `summarizeToolCall()` |
| `prompts.go` | `FormatSystemPrompt()`, `FormatIssueContext()`, `FormatFileList()` |
| `tools.go` | `DevTools()` — returns tool definitions for `read_file`, `edit_file`, `write_file`, `search_files`, `list_files`, `run_command` |

## internal/ghub

GitHub API integration.

| File | Key Types / Functions |
|------|----------------------|
| `client.go` | `Client` interface, `GitHubClient` struct, `NewClient()`, `PROptions`, `WithErrorHandling()` |
| `issues.go` | `ListIssues()`, `GetIssue()`, `CreateIssue()`, `AssignIssue()`, `AssignSelfIfNoAssignees()`, `AddLabels()`, `RemoveLabel()` |
| `pulls.go` | `CreatePR()`, `ListPRs()` |
| `branches.go` | `CreateBranch()` |
| `comments.go` | `CreateComment()`, `ListComments()` |
| `poller.go` | `Poller` struct, `NewPoller()`, `Run()`, `EventHandler` type, `IdleHandler` field |

## internal/gitops

Git operations.

| File | Key Types / Functions |
|------|----------------------|
| `repo.go` | `Repo` struct, `Clone()`, `Open()`, `Dir()`, `CheckoutBranch()`, `Pull()`, `Push()` (RefSpec `+refs/heads/*:refs/heads/*`) |
| `commit.go` | `ReadFile()`, `WriteFile()` (auto-creates dirs), `StageAll()`, `Commit()` (author: `DeveloperAgent <agent@devqaagent.local>`), `ListFiles()` |

## internal/config

Configuration loading and validation.

| File | Key Types / Functions |
|------|----------------------|
| `config.go` | `Config` (top-level), `GitHubConfig`, `ClaudeConfig`, `AgentsConfig`, `StateConfig`, `LoggingConfig`, `CreativityConfig`, `DecompositionConfig`, `ErrorHandlingConfig`, `MetricsConfig`, `ObservabilityConfig`, `Load()` |
| `validate.go` | `Validate()` — checks required fields, sets defaults |

## internal/state

Persistent agent work state.

| File | Key Types / Functions |
|------|----------------------|
| `store.go` | `Store` interface (`Save`, `Load`, `Delete`, `List`) |
| `filestore.go` | `FileStore` implementation — JSON files in `.agentctl/state/` |
| `models.go` | `WorkflowState` enum (12 states), `AgentWorkState` struct |

## internal/creativity

Autonomous suggestion engine for idle periods.

| File | Key Types / Functions |
|------|----------------------|
| `interfaces.go` | `GitHubClient` interface (`ListIssuesByLabel`, `CreateIssue`, `AddLabels`, `RemoveLabel`), `AIClient` interface (`GenerateSuggestion`), `GitHubAdapter`, `ClaudeAdapter`, `Issue` simplified type, `Suggestion` type, `parseSuggestion()` |
| `creativity.go` | `CreativityEngine`, `NewCreativityEngine()`, `Run()` — main loop: load rejections → check work → check pending → gather context → generate → deduplicate → create issue |
| `context.go` | `ProjectContext` struct (`OpenIssues`, `RejectedIdeas`, `PendingIdeas`), `gatherContext()`, `buildPrompt()` |
| `suggestion.go` | `generateSuggestion()`, `isDuplicate()` (case-insensitive containment), `createSuggestionIssue()` |
| `work_check.go` | `checkForAvailableWork()`, `hasPendingSuggestion()`, label constants (`labelReady`, `labelSuggestion`, `labelSuggestionRejected`) |
| `rejection_cache.go` | `RejectionCache` — FIFO cache with `Contains()`, `Add()`, max size |

## internal/errors

Centralized error handling.

| File | Key Types / Functions |
|------|----------------------|
| `manager.go` | `Manager` struct, `NewManager()`, `WithObservability()`, `GetRetryer()`, `GetCircuitBreaker()`, `GetCombinedDecorator()`, `GetStats()`, `IsEnabled()` |
| `retry.go` | `Retryer` struct, `NewRetryer()`, `Execute()`, `RetryPolicy`, `DefaultRetryPolicy()`, `NoRetryPolicy()`, `RetryDecorator()`, `WithOperationName()`, `WithObservability()` |
| `circuit_breaker.go` | `CircuitBreaker` struct, `NewCircuitBreaker()`, `Execute()`, `Stats()`, `WithObservability()`, `CircuitBreakerConfig`, `CircuitBreakerStats` |
| `agent_errors.go` | `ErrorType` constants (7 types), `AgentCommunicationError` struct, `ClassifyError()`, constructors: `NewNetworkError()`, `NewRateLimitError()`, `NewAuthError()`, `NewTimeoutError()`, `NewAPIError()`, `NewPermanentError()`, sentinels: `ErrNetworkTimeout`, `ErrRateLimited`, `ErrAuthentication`, `ErrPermanentFailure` |

## internal/observability

Structured logging, correlation IDs, and metrics.

| File | Key Types / Functions |
|------|----------------------|
| `correlation.go` | `CorrelationContext`, `HandoffInfo`, `StageEntry`, `WorkflowStage` constants (12 stages incl. `Decompose`, `Handoff`, `Idle`, `Error`), `EnsureCorrelationContext()`, `EnsureCorrelationID()`, `WithCorrelationID()`, `GetCorrelationID()`, `WithCorrelationContext()`, `GetCorrelationContext()`, `WithWorkflowStage()`, `WithHandoff()`, `WithMetadata()`, `GetWorkflowDuration()`, `GetStageDuration()`, `GetHandoffCount()`, `IsCurrentStage()` |
| `logger.go` | `StructuredLogger`, `NewStructuredLogger()`, `LogAgentStart()`, `LogAgentStop()`, `LogWorkflowTransition()`, `LogAgentHandoff()`, `LogDecisionPoint()`, `LogToolUsage()`, `LogLLMCall()`, `LogPerformanceMetric()`, `LogRetryAttempt()`, `LogRetrySuccess()`, `LogRetryExhausted()`, `LogRetryNonRetryable()`, `LogRetryDelay()`, `LogCircuitBreakerStateChange()`, `LogCircuitBreakerRejection()` |
| `metrics.go` | `Metrics`, `NewMetrics()`, `Timer` (`Stop()`, `StopWithContext()`), `Histogram`, `Inc()`, `Add()`, `Set()`, `Observe()`, `RecordAgentOperation()`, `RecordLLMCall()`, `RecordWorkflowTransition()`, `GetCounters()`, `GetGauges()`, `GetHistogramSummary()` |

## internal/cli

Cobra CLI commands.

| File | Key Types / Functions |
|------|----------------------|
| `root.go` | Root command setup, `Execute()`, `--config` flag |
| `start.go` | `agentctl start` — loads config, initializes observability → error manager → clients → dependencies → agents → orchestrator; signal handling (`SIGINT`, `SIGTERM`) for graceful shutdown |
| `status.go` | `agentctl status` — displays agent status reports as JSON; human-readable state descriptions for `creative_thinking` and `decompose` states |
