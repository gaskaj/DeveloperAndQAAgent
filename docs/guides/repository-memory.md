# Repository Memory System

The repository memory system persists learnings across issues and creative thinking cycles so Claude becomes more efficient over time when working repeatedly on the same repository.

## Overview

When the developer agent completes work on an issue, it extracts reusable insights (architecture decisions, coding conventions, file locations, gotchas) and stores them in a per-repo memory file. These memories are injected into subsequent prompts for analysis, implementation, and creative thinking.

## How It Works

1. **Memory injection** — Before each `analyze()` and `implement()` call, accumulated memories are formatted and injected into the prompt alongside the repo context. This gives Claude immediate awareness of patterns discovered in previous work.

2. **Memory extraction** — After successful implementation, a separate Claude call extracts up to 5 reusable learnings from the completed work. These are parsed from JSON and stored.

3. **Shared across workflows** — The same memory store is shared between the developer workflow and the creativity engine, so creative suggestions benefit from implementation learnings and vice versa.

## Memory Categories

| Category | Purpose | Example |
|----------|---------|---------|
| `architecture` | High-level design decisions | "Uses dependency injection via agent.Dependencies struct" |
| `convention` | Coding conventions | "Error wrapping uses fmt.Errorf with %w verb" |
| `pattern` | Recurring patterns | "All handlers follow chi middleware chain pattern" |
| `file_map` | Important file locations | "Config types are in internal/config/config.go" |
| `gotcha` | Non-obvious pitfalls | "Must rebuild binary after changing internal/ code" |
| `learning` | Task-specific insights | "GitHub API returns 422 when label already exists" |

## Storage

Memories are stored as JSON at `workspaces/{owner}/{repo}/.memory/memories.json`. This location is:
- **Per-repository** — Each repo has its own memory, preventing cross-contamination
- **Persistent** — Survives agent restarts and workspace cleanup (outside issue-specific directories)
- **Shareable** — Both developer workflow and creativity engine access the same store

## Configuration

See `configs/config.example.yaml` for the `memory:` section:

```yaml
memory:
  enabled: true
  max_entries: 100        # Maximum stored memories per repo
  max_prompt_size: 8000   # Max characters injected into prompts
  extract_on_complete: true  # Extract learnings after implementations
```

## Key Files

- `internal/memory/memory.go` — Store, Entry types, FormatForPrompt, persistence
- `internal/memory/extract.go` — ExtractionPrompt, ParseExtractedMemories
- `internal/config/config.go` — MemoryConfig type
- `internal/developer/workflow.go` — Memory injection in analyze/implement, extraction after completion
- `internal/developer/developer.go` — Memory store initialization
- `internal/creativity/creativity.go` — Memory store passed to creativity engine
- `internal/creativity/context.go` — Memory injected into creativity prompts

## Deduplication and Eviction

- Exact duplicate content is skipped on add
- When max_entries is exceeded, the least-used entry is evicted
- Each time memories are included in a prompt, their use count is incremented
- Frequently used memories survive longer than rarely used ones
