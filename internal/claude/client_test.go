package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"log/slog"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	agentErrors "github.com/gaskaj/OpenAgentFramework/internal/errors"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
)

// newTestServer creates an httptest server that returns a valid Claude API response.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, option.RequestOption) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, option.WithBaseURL(ts.URL)
}

// successHandler returns a handler that responds with a simple text message.
func successHandler(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":    "msg_test123",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// toolUseHandler returns a handler that responds with a tool_use response.
func toolUseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":    "msg_tool123",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Let me read that file."},
				{
					"type":  "tool_use",
					"id":    "toolu_123",
					"name":  "read_file",
					"input": map[string]interface{}{"path": "main.go"},
				},
			},
			"stop_reason": "tool_use",
			"usage": map[string]interface{}{
				"input_tokens":  120,
				"output_tokens": 80,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// errorHandler returns a handler that responds with an HTTP error.
func errorHandler(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"type":"api_error","message":"test error"}}`, code)
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096)
	require.NotNil(t, c)
	assert.Equal(t, "claude-sonnet-4-20250514", c.model)
	assert.Equal(t, 4096, c.maxTokens)
	assert.Nil(t, c.structuredLogger)
	assert.Nil(t, c.metrics)
	assert.Nil(t, c.errorManager)
}

func TestNewClient_WithOptions(t *testing.T) {
	ts, baseURLOpt := newTestServer(t, successHandler("hello"))
	_ = ts
	c := NewClient("test-key", "claude-sonnet-4-20250514", 8192, baseURLOpt)
	require.NotNil(t, c)
	assert.Equal(t, 8192, c.maxTokens)
}

func TestWithObservability(t *testing.T) {
	c := NewClient("key", "model", 1024)
	logger := observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
	metrics := observability.NewMetrics(logger)

	result := c.WithObservability(logger, metrics)
	assert.Same(t, c, result, "WithObservability should return same client for chaining")
	assert.Same(t, logger, c.structuredLogger)
	assert.Same(t, metrics, c.metrics)
}

func TestWithErrorHandling(t *testing.T) {
	c := NewClient("key", "model", 1024)
	em := agentErrors.NewManager(nil, slog.Default())

	result := c.WithErrorHandling(em)
	assert.Same(t, c, result, "WithErrorHandling should return same client for chaining")
	assert.Same(t, em, c.errorManager)
}

func TestBuilderChaining(t *testing.T) {
	logger := observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
	metrics := observability.NewMetrics(logger)
	em := agentErrors.NewManager(nil, slog.Default())

	c := NewClient("key", "model", 4096).
		WithObservability(logger, metrics).
		WithErrorHandling(em)

	assert.NotNil(t, c.structuredLogger)
	assert.NotNil(t, c.metrics)
	assert.NotNil(t, c.errorManager)
}

func TestSendMessage_Success(t *testing.T) {
	_, baseURL := newTestServer(t, successHandler("Hello from Claude"))
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	msg, err := c.SendMessage(context.Background(), "You are helpful.", nil)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "Hello from Claude", ExtractText(msg))
}

func TestSendMessage_WithSystem(t *testing.T) {
	var capturedBody map[string]interface{}
	_, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		successHandler("response")(w, r)
	})
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	_, err := c.SendMessage(context.Background(), "system prompt", nil)
	require.NoError(t, err)
	assert.NotNil(t, capturedBody["system"], "system field should be present when system prompt is non-empty")
}

func TestSendMessage_EmptySystem(t *testing.T) {
	var capturedBody map[string]interface{}
	_, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		successHandler("response")(w, r)
	})
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	_, err := c.SendMessage(context.Background(), "", nil)
	require.NoError(t, err)
}

func TestSendMessage_APIError(t *testing.T) {
	_, baseURL := newTestServer(t, errorHandler(500))
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	msg, err := c.SendMessage(context.Background(), "", nil)
	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "claude API call")
}

func TestSendMessage_ContextCancelled(t *testing.T) {
	_, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Slow handler - the context should cancel first
		select {}
	})
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.SendMessage(ctx, "", nil)
	assert.Error(t, err)
}

func TestSendMessage_WithObservability(t *testing.T) {
	_, baseURL := newTestServer(t, successHandler("Hi"))
	logger := observability.NewStructuredLogger(config.LoggingConfig{Level: "info"})
	metrics := observability.NewMetrics(logger)

	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL).
		WithObservability(logger, metrics)

	msg, err := c.SendMessage(context.Background(), "system", nil)
	require.NoError(t, err)
	require.NotNil(t, msg)
}

func TestSendMessage_WithErrorHandling(t *testing.T) {
	_, baseURL := newTestServer(t, successHandler("Hi"))
	em := agentErrors.NewManager(nil, slog.Default())

	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL).
		WithErrorHandling(em)

	msg, err := c.SendMessage(context.Background(), "system", nil)
	require.NoError(t, err)
	require.NotNil(t, msg)
}

func TestSendMessageWithTools_Success(t *testing.T) {
	_, baseURL := newTestServer(t, toolUseHandler())
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	tools := DevTools()
	msg, err := c.SendMessageWithTools(context.Background(), "system", nil, tools)
	require.NoError(t, err)
	require.NotNil(t, msg)
}

func TestSendMessageWithTools_APIError(t *testing.T) {
	_, baseURL := newTestServer(t, errorHandler(429))
	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL)

	tools := DevTools()
	msg, err := c.SendMessageWithTools(context.Background(), "system", nil, tools)
	assert.Error(t, err)
	assert.Nil(t, msg)
}

func TestSendMessageWithTools_WithErrorHandling(t *testing.T) {
	_, baseURL := newTestServer(t, successHandler("done"))
	em := agentErrors.NewManager(nil, slog.Default())

	c := NewClient("test-key", "claude-sonnet-4-20250514", 4096, baseURL).
		WithErrorHandling(em)

	tools := DevTools()
	msg, err := c.SendMessageWithTools(context.Background(), "system", nil, tools)
	require.NoError(t, err)
	require.NotNil(t, msg)
}

func TestEstimateTokens(t *testing.T) {
	c := NewClient("key", "model", 1024)

	t.Run("empty system no messages", func(t *testing.T) {
		tokens := c.estimateTokens("", nil)
		assert.Equal(t, 0, tokens)
	})

	t.Run("system prompt only", func(t *testing.T) {
		system := strings.Repeat("x", 100)
		tokens := c.estimateTokens(system, nil)
		assert.Equal(t, 25, tokens) // 100/4
	})

	t.Run("with messages", func(t *testing.T) {
		system := strings.Repeat("x", 40) // 10 tokens
		msgs := []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
			anthropic.NewUserMessage(anthropic.NewTextBlock("world")),
		}
		tokens := c.estimateTokens(system, msgs)
		// 40/4 + 2*50 = 10 + 100 = 110
		assert.Equal(t, 110, tokens)
	})
}

func TestExtractText_WithTextBlock(t *testing.T) {
	_, baseURL := newTestServer(t, successHandler("extracted text"))
	c := NewClient("key", "model", 1024, baseURL)

	msg, err := c.SendMessage(context.Background(), "", nil)
	require.NoError(t, err)

	text := ExtractText(msg)
	assert.Equal(t, "extracted text", text)
}

func TestExtractText_EmptyContent(t *testing.T) {
	_, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":          "msg_test",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-sonnet-4-20250514",
			"content":     []map[string]interface{}{},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 10, "output_tokens": 0},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	c := NewClient("key", "model", 1024, baseURL)

	msg, err := c.SendMessage(context.Background(), "", nil)
	require.NoError(t, err)

	text := ExtractText(msg)
	assert.Equal(t, "", text)
}
