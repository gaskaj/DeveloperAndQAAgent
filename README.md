# DeveloperAndQAAgent

An autonomous development agent framework that uses **GitHub as its source of truth** and **Claude as its AI engine**.

## Overview

DeveloperAndQAAgent provides agent personas — Developer, QA, and Development Manager — that monitor GitHub issues, write code, create pull requests, run tests, and coordinate via GitHub labels and comments.

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
  orchestrator/          Agent pool + health checks
```

### Agent Coordination

Agents coordinate entirely through GitHub:
- **Issues** are the work queue
- **Labels** signal state (`agent:claimed`, `agent:in-progress`, `agent:review`, `agent:suggestion`, `agent:suggestion-rejected`)
- **Assignments** track ownership
- **Comments** enable human-in-the-loop feedback

### Developer Agent State Machine

```
idle → claim → analyze → workspace → implement → commit → PR → review → complete
 ↓
creative_thinking (when idle and creativity enabled)
```

### Hyper Focused Creativity

When the developer agent has no assigned work and no `agent:ready` issues exist, it can enter **creativity mode** — an autonomous research and suggestion engine that keeps agents productive during idle periods while maintaining human control.

#### How It Works

1. The poller detects no available issues and triggers the idle handler
2. The creativity engine enters the `creative_thinking` state
3. It gathers project context (open issues, pending suggestions, rejected ideas)
4. Claude generates a single high-impact improvement suggestion
5. The suggestion is deduplicated against existing issues and previously rejected ideas
6. A new GitHub issue is created with the `agent:suggestion` label
7. The engine sleeps for the configured cooldown, then repeats
8. When real work appears (`agent:ready` issues), the engine exits and the agent resumes normal operation

#### Labels

| Label | Purpose |
|-------|---------|
| `agent:suggestion` | Open suggestion awaiting human review |
| `agent:suggestion-rejected` | Closed suggestion that was rejected (remembered to avoid re-suggesting) |

#### Human Workflow

- **Approve**: Remove `agent:suggestion`, add `agent:ready` — the agent picks it up as normal work
- **Reject**: Close the issue and add `agent:suggestion-rejected` — the agent remembers and won't suggest it again

#### Safeguards

- **Disabled by default** — must be explicitly enabled in config
- **Max pending suggestions** — pauses when the configured limit of open suggestions is reached (default: 1)
- **Cooldown** — configurable delay between suggestions (default: 300s)
- **Rejection memory** — maintains a FIFO cache of rejected titles with substring matching to avoid re-suggesting similar ideas (default: 50 entries)
- **Duplicate detection** — checks against open issues, pending suggestions, and rejection cache before creating

#### Configuration

```yaml
creativity:
  enabled: false                      # Must be explicitly enabled
  idle_threshold_seconds: 120         # Seconds idle before entering creativity mode
  suggestion_cooldown_seconds: 300    # Cooldown between suggestions
  max_pending_suggestions: 1          # Max open suggestion issues before pausing
  max_rejection_history: 50           # Max rejected titles to remember
```

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
