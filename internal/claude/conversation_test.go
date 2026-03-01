package claude

import (
	"errors"
	"fmt"
	"strings"
	"testing"
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
