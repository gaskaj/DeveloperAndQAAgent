package developer

// SystemPrompt is the base system prompt for the developer agent.
const SystemPrompt = `You are an autonomous developer agent. You write clean, production-quality Go code.

Your workflow:
1. Analyze the GitHub issue requirements carefully.
2. Plan your implementation approach.
3. Write code using the available tools (read_file, write_file, list_files, run_command).
4. Test your changes by running "go build ./..." and "go test ./...".
5. Fix any compilation or test errors.

Guidelines:
- Write idiomatic Go code following standard conventions.
- Include appropriate error handling.
- Keep functions focused and well-named.
- Add tests for new functionality.
- Do not modify files unrelated to the issue.`

// AnalyzePrompt is used when analyzing an issue to create a plan.
const AnalyzePrompt = `Analyze the following GitHub issue and create an implementation plan.

%s

Respond with a concise plan listing:
1. Files to create or modify
2. Key design decisions
3. Testing approach`

// ImplementPrompt is used when implementing the planned changes.
const ImplementPrompt = `Implement the following plan for this issue. Use the available tools to write files, then build and test.

## Issue
%s

## Plan
%s

Write all necessary code, then run "go build ./..." and "go test ./..." to verify.`

// ComplexityEstimatePrompt is appended to the AnalyzePrompt when decomposition is enabled.
// It asks Claude to enumerate each tool-use operation and produce a structured estimate.
const ComplexityEstimatePrompt = `

## Complexity Estimation

After creating your plan, estimate the number of tool-use iterations needed to implement it.
Each of the following counts as one iteration: list_files, read_file, write_file, run_command.

Enumerate every operation step-by-step, for example:

1. list_files to discover project structure
2. read_file to inspect existing handler
3. write_file to create new model
4. write_file to update handler
5. run_command: go build ./...
6. run_command: go test ./...

Then state the total on its own line in exactly this format:

**Estimated iterations**: <N>

The iteration budget is %d. To account for unexpected reads, retries, and test fixes,
apply a 30%% safety margin: if your estimate exceeds %d of the budget (i.e., 70%%),
answer "no".

At the end of your response, include exactly one of:

**Fits within budget**: yes

**Fits within budget**: no

If the answer is "no", also include a decomposition plan using this format:

## Decomposition Plan

### Subtask 1: <title>
<description of what this subtask should accomplish>

### Subtask 2: <title>
<description of what this subtask should accomplish>

(and so on, up to %d subtasks)
`

// DecomposePrompt is used for standalone decomposition calls when the analyze step
// did not include a decomposition plan.
const DecomposePrompt = `The following GitHub issue is too complex to implement in a single pass (iteration budget: %d).

Break it into smaller, independently implementable subtasks.

## Issue
%s

## Plan
%s

Respond with a decomposition plan using this exact format:

## Decomposition Plan

### Subtask 1: <title>
<description of what this subtask should accomplish>

### Subtask 2: <title>
<description of what this subtask should accomplish>

(up to %d subtasks)

Each subtask should be self-contained and result in a working, testable change.`

// ReactiveDecomposePrompt is used when the iteration limit is hit at runtime.
// It asks Claude to decompose the remaining work.
const ReactiveDecomposePrompt = `The implementation of the following issue ran out of iteration budget before completing.

## Original Issue
%s

## Plan
%s

The agent was partway through implementation when the iteration limit was reached. Break the REMAINING work into smaller subtasks that can each be completed independently.

Respond with a decomposition plan using this exact format:

## Decomposition Plan

### Subtask 1: <title>
<description of what this subtask should accomplish>

### Subtask 2: <title>
<description of what this subtask should accomplish>

(up to %d subtasks)

Each subtask should be self-contained and result in a working, testable change. Focus on what still needs to be done, not what was already completed.`
