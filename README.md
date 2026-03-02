# DeveloperAndQAAgent

An autonomous development agent framework that uses **GitHub as its source of truth** and **Claude as its AI engine**.

## Overview

DeveloperAndQAAgent provides agent personas — Developer, QA, and Development Manager — that monitor GitHub issues, write code, create pull requests, run tests, and coordinate via GitHub labels and comments.

## Documentation

| Document | Description |
|----------|-------------|
| [CLAUDE.md](CLAUDE.md) | LLM-first project orientation |
| [docs/architecture.md](docs/architecture.md) | System design, packages, data flow |
| [docs/developer-workflow.md](docs/developer-workflow.md) | State machine, decomposition, creativity |
| [docs/claude-integration.md](docs/claude-integration.md) | Claude client, tools, prompts, SDK patterns |
| [docs/github-integration.md](docs/github-integration.md) | GitHub client, poller, labels, PRs |
| [docs/configuration.md](docs/configuration.md) | Full YAML reference, env vars, defaults |
| [docs/code-conventions.md](docs/code-conventions.md) | Go patterns, testing, git conventions |
| [docs/package-reference.md](docs/package-reference.md) | Per-package API catalog |
| [docs/structured-logging.md](docs/structured-logging.md) | Observability, correlation IDs, metrics |

## Architecture

```
cmd/agentctl/            CLI entry point
internal/
  cli/                   Cobra CLI commands
  config/                Configuration loading (Viper)
  ghub/                  GitHub API integration
  claude/                Claude AI integration
  gitops/                Git operations (go-git)
  agent/                 Agent interface + registry
  state/                 Persistent agent work state
  developer/             Developer agent implementation
  creativity/            Autonomous suggestion engine (idle mode)
  errors/                Retry, circuit breakers, error classification
  observability/         Structured logging, correlation IDs, metrics
  orchestrator/          Agent pool + health checks
```

### Agent Coordination

Agents coordinate entirely through GitHub:
- **Issues** are the work queue
- **Labels** signal state (`agent:ready`, `agent:claimed`, `agent:in-progress`, `agent:in-review`, `agent:suggestion`)
- **Assignments** track ownership
- **Comments** enable human-in-the-loop feedback

### Developer Agent Workflow

```
idle → claim → workspace → analyze → [decompose] → implement → commit → PR → review → complete
```

When idle and creativity is enabled, the agent enters `creative_thinking` mode — generating improvement suggestions as GitHub issues. Complex issues are automatically decomposed into subtasks. See [docs/developer-workflow.md](docs/developer-workflow.md) for details.

## Setup

### Prerequisites

- Go 1.22+
- GitHub personal access token (repo scope)
- Anthropic API key

### Configuration

```bash
cp configs/config.example.yaml configs/config.yaml
# Edit configs/config.yaml with your tokens and repo details
```

See [docs/configuration.md](docs/configuration.md) for the full YAML reference.

### Build & Run

```bash
make build          # Build the agentctl binary
make test           # Run tests
make run            # Build and run with example config
```

### Commands

```bash
agentctl start --config configs/config.yaml   # Start agent loop
agentctl status --config configs/config.yaml  # Show agent status
```

## License

MIT
