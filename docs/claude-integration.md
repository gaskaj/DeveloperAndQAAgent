# Claude Integration

## Client

The `claude.Client` wraps the Anthropic Go SDK (`anthropic-sdk-go`).

**Location**: `internal/claude/client.go`

### Construction

```go
client := claude.NewClient(apiKey, model, maxTokens).
    WithObservability(structuredLogger, metrics).
    WithErrorHandling(errorManager)
```

### Methods

| Method | Description |
|--------|-------------|
| `SendMessage(ctx, system, messages)` | Single message, no tools |
| `SendMessageWithTools(ctx, system, messages, tools)` | Message with tool definitions |
| `WithObservability(logger, metrics)` | Adds structured logging and metrics |
| `WithErrorHandling(errorManager)` | Adds retry and circuit breaker protection |

Both `SendMessage` and `SendMessageWithTools` record:
- LLM call duration
- Input/output token counts
- Success/failure status

When an `ErrorManager` is configured, calls are wrapped with retry logic and circuit breaker protection.

## Conversation Manager

**Location**: `internal/claude/conversation.go`

The `Conversation` struct manages multi-turn interactions with the tool loop.

### Construction

```go
conv := claude.NewConversation(client, systemPrompt, tools, executor, logger, maxIter)
```

- `maxIter` — maximum API round-trips (defaults to 20 if 0)
- `tools` — pass `nil` for single-turn calls (analysis)
- `executor` — `ToolExecutor` function that dispatches tool calls

### Send Loop

`Send(ctx, userMessage)` runs a loop:

1. Append user message to history
2. Call Claude API (with or without tools)
3. Append assistant response to history
4. If `StopReason != "tool_use"` → return text response (done)
5. Execute all tool calls via `executor`
6. Append tool results as user message
7. Repeat from step 2

Returns `ErrMaxIterations` if the loop exceeds `maxIter`. Check with `claude.IsMaxIterationsError(err)`.

### Result Truncation

Tool results exceeding `maxToolResultLen` (20,000 chars) are truncated to prevent unbounded message history growth.

### Verbose Logging

`summarizeToolCall()` produces human-readable one-line summaries:
- `read_file(internal/agent/agent.go)`
- `edit_file(handler.go, "func handleReq...")`
- `write_file(new_test.go, 1234 bytes)`
- `search_files("functionName")`
- `run_command("go build ./...")`

Each iteration logs: iteration count, remaining budget, tool calls made, and assistant reasoning (truncated to 300 chars).

## Tools

**Location**: `internal/claude/tools.go`

`DevTools()` returns 6 tool definitions:

| Tool | Parameters | Description |
|------|-----------|-------------|
| `read_file` | `path` | Read file contents |
| `edit_file` | `path`, `old_string`, `new_string` | Search-and-replace in a file (old_string must be unique) |
| `write_file` | `path`, `content` | Create or overwrite a file |
| `search_files` | `pattern`, `path` (optional) | Grep across workspace (regex or literal) |
| `list_files` | `path` | List files in a directory |
| `run_command` | `command` | Execute shell command in workspace |

### Tool Executor

**Location**: `internal/developer/workflow.go` (`createToolExecutor`)

The executor dispatches tool calls to `gitops.Repo` methods:

- `read_file` → `repo.ReadFile(path)`
- `edit_file` → `executeEditFile()` — reads file, validates unique match, replaces, writes back
- `write_file` → `repo.WriteFile(path, content)`
- `search_files` → `executeSearchFiles()` — walks workspace, matches regex, returns up to 50 results
- `list_files` → `repo.ListFiles(path)`
- `run_command` → `exec.CommandContext("sh", "-c", command)` in workspace dir

`search_files` only inspects files with known extensions: `.go`, `.yaml`, `.yml`, `.json`, `.md`, `.txt`, `.mod`, `.sum`, `.toml`, `.cfg`, `.conf`, `.sh`.

## Prompt Templates

**Location**: `internal/developer/prompts.go`

| Constant | Purpose |
|----------|---------|
| `SystemPrompt` | Base system prompt — coding guidelines, tool preferences, efficiency rules |
| `AnalyzePrompt` | Issue analysis — produce implementation plan |
| `ImplementPrompt` | Implementation — execute the plan using tools |
| `ComplexityEstimatePrompt` | Appended to analysis when decomposition enabled — estimate API iterations |
| `DecomposePrompt` | Break complex issue into subtasks |
| `ReactiveDecomposePrompt` | Break *remaining* work into subtasks after iteration limit hit |

**Location**: `internal/claude/prompts.go`

| Function | Purpose |
|----------|---------|
| `FormatSystemPrompt(parts...)` | Join prompt sections |
| `FormatIssueContext(number, title, body, labels)` | Format issue for prompts |
| `FormatFileList(files)` | Format file list for prompts |

## SDK Usage Patterns

The project uses `anthropic-sdk-go` directly:

```go
// Creating the SDK client
sdk := anthropic.NewClient(option.WithAPIKey(apiKey))

// Sending messages
params := anthropic.MessageNewParams{
    Model:     anthropic.Model(model),
    MaxTokens: int64(maxTokens),
    Messages:  messages,
    Tools:     tools,  // optional
}
if system != "" {
    params.System = []anthropic.TextBlockParam{{Text: system}}
}
response, err := sdk.Messages.New(ctx, params)

// Processing responses
for _, block := range response.Content {
    switch block.Type {
    case "text":    // final text response
    case "tool_use": // tool call to execute
    }
}
```
