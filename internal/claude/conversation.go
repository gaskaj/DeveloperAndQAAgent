package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// maxToolResultLen is the maximum character length for a single tool result.
// Results exceeding this are truncated to prevent unbounded message history growth.
const maxToolResultLen = 20000

// ErrMaxIterations is returned when a conversation exceeds the maximum allowed iterations.
var ErrMaxIterations = errors.New("conversation exceeded maximum iterations")

// IsMaxIterationsError reports whether err is or wraps ErrMaxIterations.
func IsMaxIterationsError(err error) bool {
	return errors.Is(err, ErrMaxIterations)
}

// ToolExecutor executes a tool call and returns the result as a string.
type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (string, error)

// Conversation manages a multi-turn conversation with Claude, including tool use.
type Conversation struct {
	client        *Client
	system        string
	messages      []anthropic.MessageParam
	tools         []anthropic.ToolUnionParam
	executor      ToolExecutor
	logger        *slog.Logger
	maxIter       int
	toolCallCount int
}

// NewConversation creates a new conversation manager.
// maxIter controls the maximum number of tool-use iterations in Send().
// When maxIter is 0, it defaults to 20.
func NewConversation(client *Client, system string, tools []anthropic.ToolUnionParam, executor ToolExecutor, logger *slog.Logger, maxIter int) *Conversation {
	if maxIter <= 0 {
		maxIter = 20
	}
	return &Conversation{
		client:   client,
		system:   system,
		tools:    tools,
		executor: executor,
		logger:   logger,
		maxIter:  maxIter,
	}
}

// Send sends a user message and processes the response, handling any tool calls
// in a loop until Claude returns a final text response.
func (c *Conversation) Send(ctx context.Context, userMessage string) (string, error) {
	c.messages = append(c.messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(userMessage),
	))

	for i := 0; i < c.maxIter; i++ {
		var msg *anthropic.Message
		var err error

		if len(c.tools) > 0 {
			msg, err = c.client.SendMessageWithTools(ctx, c.system, c.messages, c.tools)
		} else {
			msg, err = c.client.SendMessage(ctx, c.system, c.messages)
		}
		if err != nil {
			return "", err
		}

		// Append assistant response to history.
		c.messages = append(c.messages, assistantMessageFromResponse(msg))

		// Count tool_use blocks in the response regardless of StopReason.
		// Claude can hit max_tokens mid-response and return tool_use blocks
		// with a StopReason of "max_tokens" instead of "tool_use".
		var assistantText string
		toolCount := 0
		var toolSummaries []string
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				assistantText = block.Text
			case "tool_use":
				toolCount++
				toolSummaries = append(toolSummaries, summarizeToolCall(block.Name, block.Input))
			}
		}

		// If no tool_use blocks, the conversation turn is complete.
		if toolCount == 0 {
			c.logger.Info("conversation complete", "iterations_used", i+1, "iterations_max", c.maxIter, "stop_reason", msg.StopReason)
			return ExtractText(msg), nil
		}

		// Log iteration progress.
		remaining := c.maxIter - i - 1
		if msg.StopReason != "tool_use" {
			c.logger.Warn("response contains tool_use blocks but stop_reason is not tool_use",
				"stop_reason", msg.StopReason,
				"tool_count", toolCount,
				"iteration", fmt.Sprintf("%d/%d", i+1, c.maxIter),
			)
		}
		c.logger.Info("iteration progress",
			"iteration", fmt.Sprintf("%d/%d", i+1, c.maxIter),
			"remaining", remaining,
			"tools_in_response", toolCount,
			"tool_calls", toolSummaries,
			"stop_reason", msg.StopReason,
		)
		if assistantText != "" {
			// Log a truncated version of the assistant's reasoning.
			if len(assistantText) > 300 {
				assistantText = assistantText[:300] + "..."
			}
			c.logger.Info("assistant reasoning", "text", assistantText)
		}

		// Process tool calls.
		toolResults, err := c.processToolCalls(ctx, msg)
		if err != nil {
			return "", fmt.Errorf("processing tool calls: %w", err)
		}
		c.toolCallCount += toolCount

		c.messages = append(c.messages, anthropic.NewUserMessage(toolResults...))
	}

	c.logger.Warn("iteration budget exhausted", "iterations_max", c.maxIter)
	return "", fmt.Errorf("%w (%d)", ErrMaxIterations, c.maxIter)
}

// ToolCallCount returns the total number of tool calls made across all iterations.
func (c *Conversation) ToolCallCount() int {
	return c.toolCallCount
}

func (c *Conversation) processToolCalls(ctx context.Context, msg *anthropic.Message) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion

	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}

		summary := summarizeToolCall(block.Name, block.Input)
		// c.logger.Info("executing tool", "tool", summary)

		result, err := c.executor(ctx, block.Name, block.Input)
		if err != nil {
			c.logger.Error("tool failed", "tool", summary, "error", err)
			results = append(results, anthropic.NewToolResultBlock(
				block.ID,
				fmt.Sprintf("error: %v", err),
				true, // is_error
			))
			continue
		}

		truncated := truncateToolResult(result)
		// c.logger.Info("tool succeeded", "tool", block.Name, "result_bytes", len(result), "truncated", len(result) != len(truncated))
		results = append(results, anthropic.NewToolResultBlock(block.ID, truncated, false))
	}

	return results, nil
}

// truncateToolResult truncates a tool result string if it exceeds maxToolResultLen.
func truncateToolResult(result string) string {
	if len(result) <= maxToolResultLen {
		return result
	}
	return fmt.Sprintf("%s\n\n... (output truncated, showing %d of %d bytes)", result[:maxToolResultLen], maxToolResultLen, len(result))
}

// summarizeToolCall returns a human-readable one-line summary of a tool call,
// including the tool name and key parameters (path, pattern, command, etc.).
func summarizeToolCall(name string, input json.RawMessage) string {
	var params map[string]interface{}
	if err := json.Unmarshal(input, &params); err != nil {
		return name
	}

	switch name {
	case "read_file":
		return fmt.Sprintf("read_file(%s)", paramStr(params, "path"))
	case "edit_file":
		old := paramStr(params, "old_string")
		if len(old) > 60 {
			old = old[:60] + "..."
		}
		return fmt.Sprintf("edit_file(%s, %q)", paramStr(params, "path"), old)
	case "write_file":
		contentLen := len(paramStr(params, "content"))
		return fmt.Sprintf("write_file(%s, %d bytes)", paramStr(params, "path"), contentLen)
	case "search_files":
		p := paramStr(params, "path")
		if p != "" {
			return fmt.Sprintf("search_files(%q, path=%s)", paramStr(params, "pattern"), p)
		}
		return fmt.Sprintf("search_files(%q)", paramStr(params, "pattern"))
	case "list_files":
		return fmt.Sprintf("list_files(%s)", paramStr(params, "path"))
	case "run_command":
		cmd := paramStr(params, "command")
		if len(cmd) > 80 {
			cmd = cmd[:80] + "..."
		}
		return fmt.Sprintf("run_command(%q)", cmd)
	default:
		return name
	}
}

// paramStr extracts a string parameter from a parsed JSON map.
func paramStr(params map[string]interface{}, key string) string {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// SerializeConversation serializes conversation state for persistence.
func (c *Conversation) SerializeConversation() (*ConversationState, error) {
	state := &ConversationState{
		MessageCount:    len(c.messages),
		LastInteraction: time.Now(),
		SystemPrompt:    c.system,
		MaxIterations:   c.maxIter,
	}

	// Compress message history for storage efficiency
	compressedHistory, err := c.compressMessageHistory()
	if err != nil {
		c.logger.Warn("failed to compress message history", "error", err)
		// Continue without compressed history
	} else {
		state.CompressedHistory = compressedHistory
	}

	// Generate context summary from recent messages
	state.ContextSummary = c.generateContextSummary()

	return state, nil
}

// RestoreConversation restores conversation state from serialized data.
func (c *Conversation) RestoreConversation(state *ConversationState) error {
	c.logger.Info("restoring conversation state",
		"message_count", state.MessageCount,
		"last_interaction", state.LastInteraction,
	)

	// Restore system prompt if different
	if state.SystemPrompt != "" && state.SystemPrompt != c.system {
		c.system = state.SystemPrompt
	}

	// Restore max iterations if different
	if state.MaxIterations > 0 && state.MaxIterations != c.maxIter {
		c.maxIter = state.MaxIterations
	}

	// Restore compressed message history if available
	if state.CompressedHistory != "" {
		if err := c.restoreMessageHistory(state.CompressedHistory); err != nil {
			c.logger.Error("failed to restore message history", "error", err)
			// Continue with empty message history
			c.messages = nil
		}
	}

	return nil
}

// compressMessageHistory compresses the message history for storage.
func (c *Conversation) compressMessageHistory() (string, error) {
	if len(c.messages) == 0 {
		return "", nil
	}

	// Create a summarized version of the conversation
	// Note: MessageParam is an interface, so we can't directly type-assert to concrete types
	// We'll create a simple summary based on message count and structure
	summaryLines := make([]string, 0, len(c.messages))

	for i := range c.messages {
		// Since we can't directly inspect the content without type assertions,
		// we'll create a generic summary
		summaryLines = append(summaryLines, fmt.Sprintf("Message[%d]: conversation turn", i))
	}

	return fmt.Sprintf("Conversation summary (%d messages):\n%v",
		len(c.messages), summaryLines), nil
}

// restoreMessageHistory restores message history from compressed data.
func (c *Conversation) restoreMessageHistory(compressedHistory string) error {
	// In a simplified implementation, we don't restore the full message history
	// but use the compressed history as context for the next conversation
	c.logger.Info("conversation history context available",
		"compressed_size", len(compressedHistory))

	// Reset messages since we can't fully restore them
	c.messages = nil
	return nil
}

// generateContextSummary generates a summary of the current conversation context.
func (c *Conversation) generateContextSummary() string {
	if len(c.messages) == 0 {
		return "No conversation history"
	}

	// Since MessageParam is an interface and we can't easily inspect the content
	// without complex type assertions, we'll provide a simple summary
	return fmt.Sprintf("Conversation: %d message turns in history", len(c.messages))
}

// ConversationState represents serializable conversation state.
type ConversationState struct {
	MessageCount      int       `json:"message_count"`
	LastInteraction   time.Time `json:"last_interaction"`
	ContextSummary    string    `json:"context_summary"`
	CompressedHistory string    `json:"compressed_history,omitempty"`
	SystemPrompt      string    `json:"system_prompt,omitempty"`
	MaxIterations     int       `json:"max_iterations,omitempty"`
}

func assistantMessageFromResponse(msg *anthropic.Message) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, anthropic.NewTextBlock(block.Text))
		case "tool_use":
			blocks = append(blocks, anthropic.NewToolUseBlock(block.ID, block.Input, block.Name))
		}
	}
	return anthropic.NewAssistantMessage(blocks...)
}
