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
 │                              │         │     │  commit ──► PR ──► validation ──► review ──► complete
 │                              ▼         │     │                       │
 │                            idle        │     ▼                      │ (fix attempt)
 │                                        │  decompose (reactive)      │
 │                                        │     │                      ▼
 ▼                                        │     ▼              implement ──► commit ──► push
creative_thinking ────────────────────────►│  process children ──► idle
                                          │
                                          └──► failed
```

**States** (defined in `internal/state/models.go`):

`idle` · `claim` · `workspace` · `analyze` · `decompose` · `implement` · `commit` · `pr` · `validation` · `review` · `complete` · `failed` · `creative_thinking`

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

### Step 7: PR Validation

**Location**: `internal/developer/workflow.go` — `validatePRChecks()`

After the PR is created, the agent monitors CI checks and attempts automatic fixes if they fail.

1. **Check monitoring**: Polls the PR using `ValidatePR()` with exponential backoff (30s initial interval, 1.5x backoff, 5m cap, 30m max wait)
2. **Status detection**: Combines both modern Check Runs API and legacy Commit Status API via `GetPRCheckStatus()`
3. **On success**: Posts a completion comment and proceeds to review
4. **On failure**: Analyzes failures using `AnalyzeFailures()` and `GenerateFixPrompt()`, then enters a fix loop:
   - Claude receives the failure analysis plus the original issue context and plan
   - Uses the same 6 tools (`read_file`, `edit_file`, etc.) to apply fixes
   - Fix iteration budget: half of `decomposition.max_iteration_budget`
   - Commits fixes (`fix: address PR check failures...`), pushes, waits 30s for new checks
   - Up to 3 fix attempts (default `PRValidationOptions.MaxRetries`)
5. **On exhaustion**: Posts failure analysis comment and marks the issue as failed

GitHub comments are posted at each stage: monitoring start, check success, check failure with analysis, fix attempt, and fix push.

See [github-integration.md](github-integration.md) for `PRValidationOptions` defaults and validation types.

### Step 8: Review

- Removes `agent:in-progress` label
- Adds `agent:in-review` label
- Returns to idle state

## Creativity Mode

When the poller finds no `agent:ready` issues and creativity is enabled, the `IdleHandler` triggers the creativity engine (`internal/creativity/`):

1. Enters `creative_thinking` state
2. Gathers full project context:
   - **Codebase awareness**: Clones/pulls the repository to read the file tree and key documentation (README.md, CLAUDE.md, docs/*.md)
   - **Open issues**: Fetches `agent:ready` issues to understand planned work
   - **Closed issues**: Fetches completed issues to understand what has already been done
   - **Pending suggestions**: Existing `agent:suggestion` issues to avoid duplicates
   - **Rejection history**: Previously rejected ideas from `agent:suggestion-rejected` issues
3. Builds a comprehensive prompt with explicit review instructions telling Claude to review all context sections before suggesting
4. Claude generates one high-impact improvement suggestion referencing specific files and packages
5. Deduplicates against existing issues, closed issues, and rejected ideas
6. Creates a GitHub issue with `agent:suggestion` label
7. Sleeps for the configured cooldown
8. Exits when real work appears

The repo clone is cached across creativity loop iterations via `ensureRepo()` — first call clones, subsequent calls pull. If cloning fails, the engine continues with issue-only context (graceful degradation).

See [configuration.md](configuration.md) for creativity settings.

## Test Coverage Requirements

All code changes must meet coverage quality gates before merging:

### Coverage Thresholds

**Critical Packages** (85% minimum):
- `claude/` - Claude API integration
- `ghub/` - GitHub operations
- `developer/` - Core workflow logic

**Infrastructure Packages** (80% minimum):
- `config/`, `state/`, `workspace/`, `agent/`, `orchestrator/`

**Utility Packages** (75% minimum):
- `errors/`, `observability/`, `creativity/`, `gitops/`

### Coverage Commands

```bash
# Full coverage analysis
make coverage

# Unit test coverage only
make coverage-unit

# Check quality gates
make coverage-gates

# Generate coverage badge
make coverage-badge
```

### Quality Gates

The automated quality gates enforce:
- Overall project coverage ≥ 80%
- Package-specific threshold compliance
- Critical path coverage validation
- No coverage regression > 2%

See [test-coverage.md](test-coverage.md) and [quality-assurance.md](quality-assurance.md) for detailed coverage requirements.

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
