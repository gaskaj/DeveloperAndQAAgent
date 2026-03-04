package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMaxIterationsError_Direct(t *testing.T) {
	if !IsMaxIterationsError(ErrMaxIterations) {
		t.Error("expected IsMaxIterationsError to return true for direct ErrMaxIterations")
	}
}

func TestIsMaxIterationsError_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", ErrMaxIterations)
	if !IsMaxIterationsError(wrapped) {
		t.Error("expected IsMaxIterationsError to return true for wrapped ErrMaxIterations")
	}
}

func TestIsMaxIterationsError_DoubleWrapped(t *testing.T) {
	inner := fmt.Errorf("inner: %w", ErrMaxIterations)
	outer := fmt.Errorf("outer: %w", inner)
	if !IsMaxIterationsError(outer) {
		t.Error("expected IsMaxIterationsError to return true for double-wrapped ErrMaxIterations")
	}
}

func TestIsMaxIterationsError_NonMatching(t *testing.T) {
	other := errors.New("some other error")
	if IsMaxIterationsError(other) {
		t.Error("expected IsMaxIterationsError to return false for unrelated error")
	}
}

func TestIsMaxIterationsError_Nil(t *testing.T) {
	if IsMaxIterationsError(nil) {
		t.Error("expected IsMaxIterationsError to return false for nil")
	}
}

func TestIsMaxIterationsError_FormattedLikeConversation(t *testing.T) {
	// This mimics the actual error returned by Conversation.Send.
	err := fmt.Errorf("%w (%d)", ErrMaxIterations, 20)
	if !IsMaxIterationsError(err) {
		t.Error("expected IsMaxIterationsError to return true for formatted error from Send")
	}
}

func TestTruncateToolResult_Short(t *testing.T) {
	result := "short output"
	got := truncateToolResult(result)
	if got != result {
		t.Errorf("expected unchanged result, got %q", got)
	}
}

func TestTruncateToolResult_ExactLimit(t *testing.T) {
	result := strings.Repeat("x", maxToolResultLen)
	got := truncateToolResult(result)
	if got != result {
		t.Error("expected unchanged result at exact limit")
	}
}

func TestTruncateToolResult_OverLimit(t *testing.T) {
	result := strings.Repeat("x", maxToolResultLen+500)
	got := truncateToolResult(result)
	if len(got) >= len(result) {
		t.Error("expected truncated result to be shorter than original")
	}
	if !strings.Contains(got, "truncated") {
		t.Error("expected truncated result to contain 'truncated' marker")
	}
	if !strings.Contains(got, fmt.Sprintf("%d", len(result))) {
		t.Error("expected truncated result to show original byte count")
	}
}

func TestNewConversation(t *testing.T) {
	c := NewClient("key", "model", 1024)
	logger := slog.Default()
	tools := DevTools()
	executor := func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		return "ok", nil
	}

	conv := NewConversation(c, "system prompt", tools, executor, logger, 10)
	require.NotNil(t, conv)
	assert.Equal(t, "system prompt", conv.system)
	assert.Equal(t, 10, conv.maxIter)
	assert.Len(t, conv.tools, 6)
}

func TestNewConversation_DefaultMaxIter(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "", nil, nil, slog.Default(), 0)
	assert.Equal(t, 20, conv.maxIter, "maxIter should default to 20 when 0")
}

func TestNewConversation_NegativeMaxIter(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "", nil, nil, slog.Default(), -5)
	assert.Equal(t, 20, conv.maxIter, "maxIter should default to 20 when negative")
}

// conversationTestServer creates a test server that returns text on the first call
// and optionally handles tool responses.
func conversationTestServer(t *testing.T, responses []string) (*Client, int) {
	t.Helper()
	callCount := 0
	ts, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		idx := callCount
		callCount++
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		successHandler(responses[idx])(w, r)
	})
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)
	return client, callCount
}

func TestConversation_Send_SimpleTextResponse(t *testing.T) {
	ts, baseURL := newTestServer(t, successHandler("Hello!"))
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)

	conv := NewConversation(client, "system", nil, nil, slog.Default(), 5)
	result, err := conv.Send(context.Background(), "Hi there")
	require.NoError(t, err)
	assert.Equal(t, "Hello!", result)
}

func TestConversation_Send_WithToolUse(t *testing.T) {
	callCount := 0
	ts, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First response: tool_use
			toolUseHandler()(w, r)
		} else {
			// Second response: final text
			successHandler("Done reading file.")(w, r)
		}
	})
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)

	executor := func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		assert.Equal(t, "read_file", name)
		return "file content here", nil
	}

	tools := DevTools()
	conv := NewConversation(client, "system", tools, executor, slog.Default(), 10)
	result, err := conv.Send(context.Background(), "Read main.go")
	require.NoError(t, err)
	assert.Equal(t, "Done reading file.", result)
	assert.Equal(t, 2, callCount)
}

func TestConversation_Send_ToolExecutorError(t *testing.T) {
	callCount := 0
	ts, baseURL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			toolUseHandler()(w, r)
		} else {
			successHandler("I see the tool failed.")(w, r)
		}
	})
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)

	executor := func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		return "", fmt.Errorf("permission denied")
	}

	tools := DevTools()
	conv := NewConversation(client, "system", tools, executor, slog.Default(), 10)
	result, err := conv.Send(context.Background(), "Read secret file")
	require.NoError(t, err)
	assert.Contains(t, result, "tool failed")
}

func TestConversation_Send_MaxIterations(t *testing.T) {
	// Always return tool_use to exhaust iterations
	ts, baseURL := newTestServer(t, toolUseHandler())
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)

	executor := func(ctx context.Context, name string, input json.RawMessage) (string, error) {
		return "result", nil
	}

	tools := DevTools()
	conv := NewConversation(client, "system", tools, executor, slog.Default(), 3)
	_, err := conv.Send(context.Background(), "Do something")
	require.Error(t, err)
	assert.True(t, IsMaxIterationsError(err))
	assert.Contains(t, err.Error(), "3")
}

func TestConversation_Send_APIError(t *testing.T) {
	ts, baseURL := newTestServer(t, errorHandler(500))
	_ = ts
	client := NewClient("key", "model", 4096, baseURL)

	conv := NewConversation(client, "system", nil, nil, slog.Default(), 5)
	_, err := conv.Send(context.Background(), "Hi")
	require.Error(t, err)
}

func TestSummarizeToolCall_ReadFile(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"path": "main.go"})
	summary := summarizeToolCall("read_file", input)
	assert.Equal(t, "read_file(main.go)", summary)
}

func TestSummarizeToolCall_EditFile(t *testing.T) {
	input, _ := json.Marshal(map[string]string{
		"path":       "main.go",
		"old_string": "old code",
		"new_string": "new code",
	})
	summary := summarizeToolCall("edit_file", input)
	assert.Contains(t, summary, "edit_file(main.go")
	assert.Contains(t, summary, "old code")
}

func TestSummarizeToolCall_EditFile_LongOldString(t *testing.T) {
	longOld := strings.Repeat("a", 100)
	input, _ := json.Marshal(map[string]string{
		"path":       "main.go",
		"old_string": longOld,
		"new_string": "new",
	})
	summary := summarizeToolCall("edit_file", input)
	// old_string should be truncated to 60 chars + "..."
	assert.Contains(t, summary, "...")
	assert.True(t, len(summary) < 200)
}

func TestSummarizeToolCall_WriteFile(t *testing.T) {
	input, _ := json.Marshal(map[string]string{
		"path":    "out.go",
		"content": "package main",
	})
	summary := summarizeToolCall("write_file", input)
	assert.Contains(t, summary, "write_file(out.go")
	assert.Contains(t, summary, "bytes")
}

func TestSummarizeToolCall_SearchFiles_WithPath(t *testing.T) {
	input, _ := json.Marshal(map[string]string{
		"pattern": "TODO",
		"path":    "internal/",
	})
	summary := summarizeToolCall("search_files", input)
	assert.Contains(t, summary, "search_files")
	assert.Contains(t, summary, "TODO")
	assert.Contains(t, summary, "internal/")
}

func TestSummarizeToolCall_SearchFiles_NoPath(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"pattern": "func main"})
	summary := summarizeToolCall("search_files", input)
	assert.Contains(t, summary, "search_files")
	assert.Contains(t, summary, "func main")
	assert.NotContains(t, summary, "path=")
}

func TestSummarizeToolCall_ListFiles(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"path": "."})
	summary := summarizeToolCall("list_files", input)
	assert.Equal(t, "list_files(.)", summary)
}

func TestSummarizeToolCall_RunCommand(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": "go test ./..."})
	summary := summarizeToolCall("run_command", input)
	assert.Contains(t, summary, "run_command")
	assert.Contains(t, summary, "go test ./...")
}

func TestSummarizeToolCall_RunCommand_Long(t *testing.T) {
	longCmd := strings.Repeat("x", 100)
	input, _ := json.Marshal(map[string]string{"command": longCmd})
	summary := summarizeToolCall("run_command", input)
	assert.Contains(t, summary, "...")
}

func TestSummarizeToolCall_Unknown(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"foo": "bar"})
	summary := summarizeToolCall("unknown_tool", input)
	assert.Equal(t, "unknown_tool", summary)
}

func TestSummarizeToolCall_InvalidJSON(t *testing.T) {
	summary := summarizeToolCall("read_file", json.RawMessage("not json"))
	assert.Equal(t, "read_file", summary)
}

func TestParamStr(t *testing.T) {
	params := map[string]interface{}{
		"path":  "main.go",
		"count": 42,
	}

	assert.Equal(t, "main.go", paramStr(params, "path"))
	assert.Equal(t, "", paramStr(params, "missing"))
	assert.Equal(t, "", paramStr(params, "count")) // not a string
}

func TestConversationState_Struct(t *testing.T) {
	cs := ConversationState{
		MessageCount:    5,
		LastInteraction: time.Now(),
		ContextSummary:  "test summary",
		SystemPrompt:    "system",
		MaxIterations:   10,
	}
	assert.Equal(t, 5, cs.MessageCount)
	assert.Equal(t, "system", cs.SystemPrompt)
	assert.Equal(t, 10, cs.MaxIterations)
}

func TestSerializeConversation_Empty(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "system", nil, nil, slog.Default(), 10)

	state, err := conv.SerializeConversation()
	require.NoError(t, err)
	assert.Equal(t, 0, state.MessageCount)
	assert.Equal(t, "system", state.SystemPrompt)
	assert.Equal(t, 10, state.MaxIterations)
	assert.Equal(t, "No conversation history", state.ContextSummary)
}

func TestSerializeConversation_WithMessages(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "system", nil, nil, slog.Default(), 10)
	// Add some messages manually
	conv.messages = append(conv.messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
	)

	state, err := conv.SerializeConversation()
	require.NoError(t, err)
	assert.Equal(t, 2, state.MessageCount)
	assert.NotEmpty(t, state.CompressedHistory)
	assert.Contains(t, state.ContextSummary, "2 message turns")
}

func TestRestoreConversation_Basic(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "original", nil, nil, slog.Default(), 10)

	state := &ConversationState{
		MessageCount:    5,
		LastInteraction: time.Now(),
		SystemPrompt:    "restored system",
		MaxIterations:   15,
	}

	err := conv.RestoreConversation(state)
	require.NoError(t, err)
	assert.Equal(t, "restored system", conv.system)
	assert.Equal(t, 15, conv.maxIter)
}

func TestRestoreConversation_EmptySystemPrompt(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "original", nil, nil, slog.Default(), 10)

	state := &ConversationState{
		SystemPrompt: "",
	}

	err := conv.RestoreConversation(state)
	require.NoError(t, err)
	assert.Equal(t, "original", conv.system, "should keep original when state has empty system")
}

func TestRestoreConversation_ZeroMaxIterations(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)

	state := &ConversationState{
		MaxIterations: 0,
	}

	err := conv.RestoreConversation(state)
	require.NoError(t, err)
	assert.Equal(t, 10, conv.maxIter, "should keep original when state has zero maxIter")
}

func TestRestoreConversation_WithCompressedHistory(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)

	state := &ConversationState{
		CompressedHistory: "some compressed data",
	}

	err := conv.RestoreConversation(state)
	require.NoError(t, err)
	// Messages should be nil after restore (simplified implementation)
	assert.Nil(t, conv.messages)
}

func TestGenerateContextSummary_Empty(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)

	summary := conv.generateContextSummary()
	assert.Equal(t, "No conversation history", summary)
}

func TestGenerateContextSummary_WithMessages(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)
	conv.messages = append(conv.messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
	)

	summary := conv.generateContextSummary()
	assert.Contains(t, summary, "1 message turns")
}

func TestCompressMessageHistory_Empty(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)

	result, err := conv.compressMessageHistory()
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestCompressMessageHistory_WithMessages(t *testing.T) {
	c := NewClient("key", "model", 1024)
	conv := NewConversation(c, "sys", nil, nil, slog.Default(), 10)
	conv.messages = append(conv.messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock("msg1")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("msg2")),
	)

	result, err := conv.compressMessageHistory()
	require.NoError(t, err)
	assert.Contains(t, result, "2 messages")
	assert.Contains(t, result, "Message[0]")
	assert.Contains(t, result, "Message[1]")
}

// Ensure http import is used (used in test server handlers above).
var _ http.Handler
