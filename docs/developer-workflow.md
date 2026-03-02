# Developer Workflow

## State Machine

```
                    ┌──────────────────────┐
                    │                      │
                    ▼                      │
idle ──► claim ──► workspace ──► analyze ──┤
 │                                        │
 │                              ┌─────────┼──────────┐
 │                              ▼         │          ▼
 │                          decompose     │     implement
 │                              │         │          │
 │                              ▼         │     ┌────┤
 │                     process children   │     │    ▼
 │                              │         │     │  commit ──► PR ──► review ──► complete
 │                              ▼         │     │
 │                            idle        │     ▼
 │                                        │  decompose (reactive)
 │                                        │     │
 ▼                                        │     ▼
creative_thinking ────────────────────────►│  process children ──► idle
                                          │
                                          └──► failed
```

**States** (defined in `internal/state/models.go`):

`idle` · `claim` · `workspace` · `analyze` · `decompose` · `implement` · `commit` · `pr` · `review` · `complete` · `failed` · `creative_thinking`

## processIssue() Walkthrough

The core workflow lives in `internal/developer/workflow.go`.

### Step 1: Claim

- Assigns self to the issue (`AssignSelfIfNoAssignees`)
- Adds `agent:claimed` label
- Removes `agent:ready` label (prevents re-processing on restart)
- Posts claiming comment

### Step 2: Workspace Setup

- Creates branch: `agent/issue-<N>`
- Clones repository to `workspaces/issue-<N>`
- Checks out the new branch

### Context Gathering

**`gatherRepoContext()`** — builds a file tree and go.mod summary injected into Claude prompts. This eliminates wasted iterations on discovery.

**`preReadFiles()`** — extracts Go file paths from the plan using `extractFilePaths()`, reads up to 8 files (max 15,000 chars each), and injects them into the implementation prompt. Claude can start writing immediately without reading files first.

### Step 3: Analyze

Single-turn Claude call (no tools). Produces:
- Files to create or modify
- Key design decisions
- Testing approach

When decomposition is enabled, the prompt also requests a complexity estimate with `ComplexityEstimatePrompt`. Claude enumerates each API round-trip and reports whether the task fits within the iteration budget.

The analysis plan is posted as a GitHub comment.

### Step 3.5: Proactive Decomposition

Triggered when analysis reports `**Fits within budget**: no`.

1. Parses subtasks from the plan (or makes a separate `DecomposePrompt` call)
2. Creates child GitHub issues with `agent:subtask` + `agent:ready` labels
3. Labels parent as `agent:epic`
4. Processes child issues sequentially via `processChildIssues()`

### Step 4: Implement

Multi-turn Claude call with 6 tools (see [claude-integration.md](claude-integration.md)):

- Claude receives: system prompt, issue context, plan, repo context, pre-read files
- Each response may contain tool calls; results are appended and sent back
- Loop continues until Claude returns text-only response (done) or budget exhausted
- Iteration limit: `decomposition.max_iteration_budget` (default 250) or 20 if decomposition disabled

Adds `agent:in-progress` label during implementation.

### Reactive Decomposition

If `ErrMaxIterations` is returned during implementation and decomposition is enabled:

1. Uses `ReactiveDecomposePrompt` to decompose *remaining* work
2. Creates child issues for unfinished work
3. Labels parent as `agent:epic` + `agent:failed`

### Step 5: Commit

- `repo.StageAll()` — stages all changes
- `repo.Commit("feat: implement #N - <title>")` — commits as `DeveloperAgent <agent@devqaagent.local>`
- `repo.Push()` — pushes branch (RefSpec: `+refs/heads/*:refs/heads/*`)

### Step 6: Create PR

Creates a pull request with:
- **Title**: `feat: implement #N - <title>`
- **Body**: `Closes #N` + implementation plan
- **Head**: `agent/issue-<N>`
- **Base**: `main`

### Step 7: Review

- Removes `agent:in-progress` label
- Adds `agent:in-review` label
- Returns to idle state

## Creativity Mode

When the poller finds no `agent:ready` issues and creativity is enabled, the `IdleHandler` triggers the creativity engine:

1. Enters `creative_thinking` state
2. Gathers project context (open issues, pending suggestions, rejection history)
3. Claude generates one high-impact improvement suggestion
4. Deduplicates against existing issues and rejected ideas
5. Creates a GitHub issue with `agent:suggestion` label
6. Sleeps for the configured cooldown
7. Exits when real work appears

See [configuration.md](configuration.md) for creativity settings.

## Self-Building Bootstrap

The agent can improve its own codebase. When a human creates an issue describing a feature or fix for the agent itself and labels it `agent:ready`, the agent:

1. Clones its own repository
2. Analyzes the issue against its own code
3. Implements the changes
4. Runs `go build ./...` and `go test ./...` to verify
5. Creates a PR for human review

This creates a feedback loop where the agent evolves its own capabilities under human supervision.

## Error Handling (failIssue)

When any step fails:

1. Logs the error
2. Updates state to `failed` with error message
3. Posts failure comment on the GitHub issue
4. Adds `agent:failed` label
5. Returns to idle state
