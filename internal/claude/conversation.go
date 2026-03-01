package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
)

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
			return ExtractText(msg), nil
		}

		// Process tool calls.
		toolResults, err := c.processToolCalls(ctx, msg)
		if err != nil {
			return "", fmt.Errorf("processing tool calls: %w", err)
		}

		c.messages = append(c.messages, anthropic.NewUserMessage(toolResults...))
	}

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

		results = append(results, anthropic.NewToolResultBlock(block.ID, result, false))
	}

	return results, nil
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
