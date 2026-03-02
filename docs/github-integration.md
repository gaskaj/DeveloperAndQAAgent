# GitHub Integration

## Client Interface

**Location**: `internal/ghub/client.go`

The `ghub.Client` interface defines all GitHub operations:

```go
type Client interface {
    // Issues
    ListIssues(ctx context.Context, labels []string) ([]*github.Issue, error)
    GetIssue(ctx context.Context, number int) (*github.Issue, error)
    AssignIssue(ctx context.Context, number int, assignees []string) error
    AssignSelfIfNoAssignees(ctx context.Context, number int) error
    AddLabels(ctx context.Context, number int, labels []string) error
    RemoveLabel(ctx context.Context, number int, label string) error

    // Branches
    CreateBranch(ctx context.Context, name string, fromRef string) error

    // Pull Requests
    CreatePR(ctx context.Context, opts PROptions) (*github.PullRequest, error)
    ListPRs(ctx context.Context, state string) ([]*github.PullRequest, error)

    // Issues (create)
    CreateIssue(ctx context.Context, title, body string, labels []string) (*github.Issue, error)

    // Comments
    CreateComment(ctx context.Context, number int, body string) error
    ListComments(ctx context.Context, number int) ([]*github.IssueComment, error)
}
```

Implemented by `GitHubClient` using `go-github/v68`.

### Error Handling

`GitHubClient` supports optional error handling via `WithErrorHandling(errorManager)`, which adds retry and circuit breaker protection to API calls. Retryable operations (`ListIssues`, `GetIssue`, `CreateIssue`) use a core/wrapper pattern:

```go
func (c *GitHubClient) ListIssues(ctx, labels) ([]*github.Issue, error) {
    if c.errorManager != nil {
        retryer := c.errorManager.GetRetryer("github_api")
        return agentErrors.Execute(ctx, retryer, func(...) { return c.listIssuesCore(ctx, labels) })
    }
    return c.listIssuesCore(ctx, labels)
}
```

All errors are classified via `agentErrors.ClassifyError()` for proper retry handling.

### PR Filtering

`ListIssues` automatically filters out pull requests from results, since the GitHub API returns PRs as issues. It checks `issue.PullRequestLinks == nil`.

### Self-Assignment

`AssignSelfIfNoAssignees` (PR #68) assigns the authenticated user to an issue if it has no assignees. It fetches the issue, checks the assignees list, gets the authenticated user via `Users.Get(ctx, "")`, and calls `AssignIssue`. Called during the claim step.

## Polling Model

**Location**: `internal/ghub/poller.go`

The `Poller` continuously polls GitHub for issues matching configured labels:

```go
poller := ghub.NewPoller(client, watchLabels, pollInterval, eventHandler, logger)
poller.IdleHandler = func(ctx context.Context) error { ... }
err := poller.Run(ctx)  // blocks until context cancelled
```

1. Initial poll on startup
2. Subsequent polls on timer (configurable interval, default 30s)
3. When issues found: calls `EventHandler` with the issue list
4. When no issues found: calls `IdleHandler` (if set) — used for creativity mode

The `EventHandler` type: `func(ctx context.Context, issues []*github.Issue) error`

## Label Protocol

Labels are the coordination mechanism between agents and humans.

### Label Transitions

```
Human adds agent:ready
    │
    ▼
agent:ready ──► agent:claimed ──► agent:in-progress ──► agent:in-review
                                        │
                                        ▼
                                   agent:failed
```

### All Labels

| Label | Set By | Meaning |
|-------|--------|---------|
| `agent:ready` | Human | Issue available for agent processing |
| `agent:claimed` | Agent | Agent has taken ownership |
| `agent:in-progress` | Agent | Implementation underway |
| `agent:in-review` | Agent | PR created, awaiting human review |
| `agent:failed` | Agent | Processing failed |
| `agent:suggestion` | Agent | Creativity engine suggestion |
| `agent:suggestion-rejected` | Human | Rejected suggestion (remembered) |
| `agent:subtask` | Agent | Child issue from decomposition |
| `agent:epic` | Agent | Parent issue that was decomposed |

## Comment Protocol

The agent posts structured comments at key workflow points:

| Event | Comment |
|-------|---------|
| Claim | "Developer agent claiming this issue. Starting analysis..." |
| Analysis complete | Analysis plan with estimated iterations |
| Decomposition | Subtask breakdown with issue links |
| Subtask progress | "Processing subtask N/M: #X" |
| Completion | Subtask summary or PR link |
| Failure | "Developer agent failed: \<error\>" |

## Branch Naming

Pattern: `agent/issue-<N>`

Example: Issue #42 → branch `agent/issue-42`

## Pull Request Format

| Field | Format |
|-------|--------|
| Title | `feat: implement #<N> - <issue title>` |
| Body | `Closes #<N>\n\n## Implementation\n\n<plan>` |
| Head | `agent/issue-<N>` |
| Base | `main` |

```go
type PROptions struct {
    Title string
    Body  string
    Head  string
    Base  string
}
```

## Authentication

The GitHub client authenticates using a personal access token (PAT) with `repo` scope:

```go
client := github.NewClient(nil).WithAuthToken(token)
```

The token is configured in `github.token` (typically via `${GITHUB_TOKEN}` environment variable).
