# CLAUDE.md — Project Orientation

OpenAgentFramework is an autonomous Go agent that monitors GitHub issues, uses Claude to analyze and implement solutions, and creates pull requests. GitHub is the coordination layer (issues, labels, PRs); Claude is the AI engine.

## Quick Reference

```bash
make build                    # Build → bin/agentctl
make test                     # Run tests with -race
make lint                     # golangci-lint + go vet
make fmt                      # gofmt -s -w .
agentctl start --config configs/config.yaml
agentctl status --config configs/config.yaml
```

## Repository Layout

```
cmd/agentctl/                 CLI entry point (main.go → cli.Execute())
internal/
  agent/                      Agent interface, BaseAgent, Dependencies DI, registry
  orchestrator/               Concurrent agent execution (errgroup)
  developer/                  Developer agent: workflow, decomposition, prompts
  claude/                     Claude API client, conversation manager, tool definitions
  ghub/                       GitHub client interface, issues, PRs, branches, comments, poller
  gitops/                     Git clone, checkout, commit, push (go-git)
  config/                     Config loading (Viper), validation, defaults
  state/                      WorkflowState enum, AgentWorkState, file-based Store
  creativity/                 Idle-mode suggestion engine
  integration/                Integration tests: agent communication, handoffs, shared state
  errors/                     Retry with backoff, circuit breakers, error classification
  observability/              Structured logger, correlation IDs, metrics
  cli/                        Cobra commands (start, status)
configs/                      YAML config files (config.yaml, config.example.yaml)
docs/                         Detailed documentation (see links below)
```

## Architecture at a Glance

The Orchestrator runs agents concurrently. The Developer agent polls GitHub for `agent:ready` issues, claims them, clones the repo, asks Claude to analyze and implement, then commits and creates a PR. Complex issues are decomposed into subtask issues. Idle agents generate improvement suggestions via the creativity engine.

**Workflow**: `idle → claim → workspace → analyze → [decompose] → implement → commit → PR → validation → review → complete`

## Key Patterns and Conventions

- **Dependency injection**: All components receive `agent.Dependencies` (Config, GitHub, Claude, Store, Logger, Metrics, ErrorManager)
- **Interface-based design**: `ghub.Client`, `state.Store`, `agent.Agent` — defined where used, not where implemented
- **Error handling**: `fmt.Errorf("doing X: %w", err)` for wrapping; `errors.Join` for aggregation; `ClassifyError()` for typed retry decisions
- **Error resilience**: All external API calls wrapped with retry (exponential backoff) + circuit breakers via `errors.Manager`
- **Logging**: `log/slog` with structured key-value pairs; child loggers via `.With()`
- **Testing**: `testify` assertions, `t.TempDir()` for file tests, mocks alongside interfaces
- **Context propagation**: All I/O operations take `context.Context` carrying correlation data
- **Builder pattern**: `NewClient().WithObservability().WithErrorHandling()`
- **Tool preferences**: Use `edit_file` over `write_file` for modifications; `search_files` over multiple `read_file` calls

## Tool Definitions

The developer agent gives Claude 6 tools (defined in `internal/claude/tools.go`):

| Tool | Purpose |
|------|---------|
| `read_file` | Read file contents |
| `edit_file` | Search-and-replace in a file (old_string must be unique) |
| `write_file` | Create or overwrite a file |
| `search_files` | Grep across workspace (regex or literal, up to 50 results) |
| `list_files` | List directory contents |
| `run_command` | Execute shell command in workspace |

## Labels Protocol

| Label | Meaning |
|-------|---------|
| `agent:ready` | Issue available for agent to claim |
| `agent:claimed` | Agent has taken ownership |
| `agent:in-progress` | Implementation underway |
| `agent:in-review` | PR created, awaiting human review |
| `agent:failed` | Processing failed |
| `agent:suggestion` | Creativity engine suggestion |
| `agent:suggestion-rejected` | Rejected suggestion (remembered) |
| `agent:subtask` | Child issue from decomposition |
| `agent:epic` | Parent issue that was decomposed |

## Workflow States

From `internal/state/models.go`:

`idle` · `claim` · `workspace` · `analyze` · `decompose` · `implement` · `commit` · `pr` · `validation` · `review` · `complete` · `failed` · `creative_thinking`

## Prompt Constants

From `internal/developer/prompts.go`:

| Constant | Purpose |
|----------|---------|
| `SystemPrompt` | Base system prompt — coding guidelines, tool preferences, efficiency |
| `AnalyzePrompt` | Issue analysis → implementation plan |
| `ImplementPrompt` | Execute plan using tools |
| `ComplexityEstimatePrompt` | Estimate API iterations (decomposition) |
| `DecomposePrompt` | Break complex issue into subtasks |
| `ReactiveDecomposePrompt` | Decompose remaining work after iteration limit |

## Configuration

Required env vars: `GITHUB_TOKEN`, `ANTHROPIC_API_KEY`

Config file: `configs/config.yaml` — see [docs/configuration.md](docs/configuration.md) for full reference.

Key sections: `github`, `claude`, `agents`, `state`, `logging`, `creativity`, `decomposition`, `error_handling`

## Deep-Dive Documentation

- [docs/architecture.md](docs/architecture.md) — System design, packages, data flow, agent personas
- [docs/developer-workflow.md](docs/developer-workflow.md) — State machine, decomposition, creativity, self-building
- [docs/claude-integration.md](docs/claude-integration.md) — Client, conversation loop, tools, prompts, SDK patterns
- [docs/github-integration.md](docs/github-integration.md) — Client interface, poller, labels, branches, PRs
- [docs/configuration.md](docs/configuration.md) — Full YAML reference, env vars, defaults, validation
- [docs/code-conventions.md](docs/code-conventions.md) — Error handling, naming, interfaces, logging, testing
- [docs/package-reference.md](docs/package-reference.md) — Per-package API catalog
- [docs/structured-logging.md](docs/structured-logging.md) — Observability, correlation IDs, metrics
- [docs/integration-testing.md](docs/integration-testing.md) — Integration test suite, mock infrastructure, CI pipeline

## Documentation Instructions 

- All new features must be documented in the ./docs folder
- Documentation is to give the LLM and a Human context what the code is doing
- Documentation should be optimized for LLM and avoid duplication of content
- Reference other documentation and/or code with file references
- Avoid duplicating code in the instructions and reference the files the documentation is describing
- Documentation MUST be added with the Issue and PR of the new feature
