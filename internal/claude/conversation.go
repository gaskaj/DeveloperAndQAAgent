package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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
	client   *Client
	system   string
	messages []anthropic.MessageParam
	tools    []anthropic.ToolUnionParam
	executor ToolExecutor
	logger   *slog.Logger
	maxIter  int
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

		// Check if we need to handle tool use.
		if msg.StopReason != "tool_use" {
			c.logger.Info("conversation complete", "iterations_used", i+1, "iterations_max", c.maxIter)
			return ExtractText(msg), nil
		}

		// Count tool calls in this response for logging.
		toolCount := 0
		var toolNames []string
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				toolCount++
				toolNames = append(toolNames, block.Name)
			}
		}
		c.logger.Info("iteration progress",
			"iteration", i+1,
			"max", c.maxIter,
			"tools_in_response", toolCount,
			"tool_names", toolNames,
		)

		// Process tool calls.
		toolResults, err := c.processToolCalls(ctx, msg)
		if err != nil {
			return "", fmt.Errorf("processing tool calls: %w", err)
		}

		c.messages = append(c.messages, anthropic.NewUserMessage(toolResults...))
	}

	c.logger.Warn("iteration budget exhausted", "iterations_max", c.maxIter)
	return "", fmt.Errorf("%w (%d)", ErrMaxIterations, c.maxIter)
}

func (c *Conversation) processToolCalls(ctx context.Context, msg *anthropic.Message) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion

	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}

		c.logger.Info("executing tool", "name", block.Name, "id", block.ID)

		result, err := c.executor(ctx, block.Name, block.Input)
		if err != nil {
			c.logger.Error("tool execution failed", "name", block.Name, "error", err)
			results = append(results, anthropic.NewToolResultBlock(
				block.ID,
				fmt.Sprintf("error: %v", err),
				true, // is_error
			))
			continue
		}

		results = append(results, anthropic.NewToolResultBlock(block.ID, truncateToolResult(result), false))
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
