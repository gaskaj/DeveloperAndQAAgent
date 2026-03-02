package claude

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	agentErrors "github.com/gaskaj/DeveloperAndQAAgent/internal/errors"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
)

// Client wraps the Anthropic SDK for Claude API interactions.
type Client struct {
	sdk              anthropic.Client
	model            string
	maxTokens        int
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
	errorManager     *agentErrors.Manager
}

// NewClient creates a new Claude API client.
// Optional request options (e.g. option.WithBaseURL) can be passed for testing.
func NewClient(apiKey, model string, maxTokens int, opts ...option.RequestOption) *Client {
	allOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)
	sdk := anthropic.NewClient(allOpts...)
	return &Client{
		sdk:       sdk,
		model:     model,
		maxTokens: maxTokens,
	}
}

// WithObservability adds observability features to the client
func (c *Client) WithObservability(logger *observability.StructuredLogger, metrics *observability.Metrics) *Client {
	c.structuredLogger = logger
	c.metrics = metrics
	return c
}

// WithErrorHandling adds error handling capabilities to the client
func (c *Client) WithErrorHandling(errorManager *agentErrors.Manager) *Client {
	c.errorManager = errorManager
	return c
}

// SendMessage sends a single message to Claude and returns the text response.
func (c *Client) SendMessage(ctx context.Context, system string, messages []anthropic.MessageParam) (*anthropic.Message, error) {
	if c.errorManager != nil {
		// Use retry with circuit breaker protection
		retryer := c.errorManager.GetRetryer("claude_api")
		circuitBreaker := c.errorManager.GetCircuitBreaker("claude_api")

		var result *anthropic.Message
		err := circuitBreaker.Execute(ctx, func(ctx context.Context) error {
			var err error
			result, err = agentErrors.Execute(ctx, retryer, func(ctx context.Context, attempt int) (*anthropic.Message, error) {
				return c.sendMessageCore(ctx, system, messages)
			})
			return err
		})

		return result, err
	}

	return c.sendMessageCore(ctx, system, messages)
}

// sendMessageCore contains the core message sending logic
func (c *Client) sendMessageCore(ctx context.Context, system string, messages []anthropic.MessageParam) (*anthropic.Message, error) {
	start := time.Now()

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(c.maxTokens),
		Messages:  messages,
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: system},
		}
	}

	response, err := c.sdk.Messages.New(ctx, params)
	duration := time.Since(start)

	// Calculate token counts (approximation for input)
	inputTokens := c.estimateTokens(system, messages)
	outputTokens := 0
	if response != nil {
		if response.Usage.OutputTokens > 0 {
			outputTokens = int(response.Usage.OutputTokens)
		}
		// Use actual input tokens if available
		if response.Usage.InputTokens > 0 {
			inputTokens = int(response.Usage.InputTokens)
		}
	}

	// Record metrics and structured logs
	success := err == nil
	if c.metrics != nil {
		c.metrics.RecordLLMCall(ctx, c.model, inputTokens, outputTokens, duration, success)
	}
	if c.structuredLogger != nil {
		c.structuredLogger.LogLLMCall(ctx, c.model, inputTokens, outputTokens, duration, err)
	}

	if err != nil {
		// Classify the error for proper retry handling
		return nil, agentErrors.ClassifyError(fmt.Errorf("claude API call: %w", err))
	}
	return response, nil
}

// SendMessageWithTools sends a message with tool definitions and returns the response.
func (c *Client) SendMessageWithTools(ctx context.Context, system string, messages []anthropic.MessageParam, tools []anthropic.ToolUnionParam) (*anthropic.Message, error) {
	if c.errorManager != nil {
		// Use retry protection
		retryer := c.errorManager.GetRetryer("claude_api")
		return agentErrors.Execute(ctx, retryer, func(ctx context.Context, attempt int) (*anthropic.Message, error) {
			return c.sendMessageWithToolsCore(ctx, system, messages, tools)
		})
	}

	return c.sendMessageWithToolsCore(ctx, system, messages, tools)
}

// sendMessageWithToolsCore contains the core message with tools sending logic
func (c *Client) sendMessageWithToolsCore(ctx context.Context, system string, messages []anthropic.MessageParam, tools []anthropic.ToolUnionParam) (*anthropic.Message, error) {
	start := time.Now()

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(c.maxTokens),
		Messages:  messages,
		Tools:     tools,
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: system},
		}
	}

	response, err := c.sdk.Messages.New(ctx, params)
	duration := time.Since(start)

	// Calculate token counts
	inputTokens := c.estimateTokens(system, messages)
	outputTokens := 0
	if response != nil {
		if response.Usage.OutputTokens > 0 {
			outputTokens = int(response.Usage.OutputTokens)
		}
		if response.Usage.InputTokens > 0 {
			inputTokens = int(response.Usage.InputTokens)
		}
	}

	// Record metrics and structured logs
	success := err == nil
	if c.metrics != nil {
		c.metrics.RecordLLMCall(ctx, c.model, inputTokens, outputTokens, duration, success)
	}
	if c.structuredLogger != nil {
		c.structuredLogger.LogLLMCall(ctx, c.model, inputTokens, outputTokens, duration, err)
	}

	if err != nil {
		// Classify the error for proper retry handling
		return nil, agentErrors.ClassifyError(fmt.Errorf("claude API call with tools: %w", err))
	}
	return response, nil
}

// estimateTokens provides a rough estimate of input token count
func (c *Client) estimateTokens(system string, messages []anthropic.MessageParam) int {
	// Simple estimation: ~4 characters per token
	count := len(system) / 4
	for range messages {
		// For simplicity, just estimate based on JSON serialization
		// In practice, you'd want more sophisticated token counting
		count += 50 // rough estimate per message
	}
	return count
}

// ExtractText extracts the text content from a Claude message response.
func ExtractText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}
