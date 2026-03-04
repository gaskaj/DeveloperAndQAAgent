package errors

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNetworkError(t *testing.T) {
	origErr := errors.New("connection refused")
	err := NewNetworkError("failed to connect", origErr)

	assert.Equal(t, ErrorTypeNetwork, err.Type)
	assert.Equal(t, "failed to connect", err.Message)
	assert.True(t, err.Retryable)
	assert.Equal(t, 5*time.Second, err.RetryAfter)
	assert.Equal(t, origErr, err.OriginalError)
}

func TestNewTimeoutError(t *testing.T) {
	origErr := errors.New("deadline exceeded")
	err := NewTimeoutError("request timed out", origErr)

	assert.Equal(t, ErrorTypeTimeout, err.Type)
	assert.Equal(t, "request timed out", err.Message)
	assert.True(t, err.Retryable)
	assert.Equal(t, 10*time.Second, err.RetryAfter)
	assert.Equal(t, origErr, err.OriginalError)
}

func TestNewAuthError(t *testing.T) {
	origErr := errors.New("invalid token")
	err := NewAuthError("authentication failed", origErr)

	assert.Equal(t, ErrorTypeAuthentication, err.Type)
	assert.Equal(t, "authentication failed", err.Message)
	assert.False(t, err.Retryable)
	assert.Equal(t, time.Duration(0), err.RetryAfter)
	assert.Equal(t, origErr, err.OriginalError)
}

func TestNewPermanentError(t *testing.T) {
	origErr := errors.New("resource not found")
	err := NewPermanentError("not found", origErr)

	assert.Equal(t, ErrorTypePermanent, err.Type)
	assert.Equal(t, "not found", err.Message)
	assert.False(t, err.Retryable)
	assert.Equal(t, time.Duration(0), err.RetryAfter)
	assert.Equal(t, origErr, err.OriginalError)
}

func TestNewAPIError(t *testing.T) {
	origErr := errors.New("server error")
	err := NewAPIError("API call failed", origErr)

	assert.Equal(t, ErrorTypeAPI, err.Type)
	assert.Equal(t, "API call failed", err.Message)
	assert.True(t, err.Retryable)
	assert.Equal(t, 5*time.Second, err.RetryAfter)
	assert.Equal(t, origErr, err.OriginalError)
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("too many requests", 30*time.Second)

	assert.Equal(t, ErrorTypeRateLimit, err.Type)
	assert.Equal(t, "too many requests", err.Message)
	assert.True(t, err.Retryable)
	assert.Equal(t, 30*time.Second, err.RetryAfter)
	assert.Nil(t, err.OriginalError)
}

func TestAgentCommunicationError_Error(t *testing.T) {
	t.Run("with original error", func(t *testing.T) {
		origErr := errors.New("underlying cause")
		err := NewNetworkError("connection lost", origErr)
		expected := "network: connection lost (original: underlying cause)"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("without original error", func(t *testing.T) {
		err := NewRateLimitError("rate limited", 10*time.Second)
		expected := "rate_limit: rate limited"
		assert.Equal(t, expected, err.Error())
	})
}

func TestAgentCommunicationError_Unwrap(t *testing.T) {
	t.Run("with original error", func(t *testing.T) {
		origErr := errors.New("root cause")
		err := NewNetworkError("wrapper", origErr)
		assert.Equal(t, origErr, err.Unwrap())
	})

	t.Run("without original error", func(t *testing.T) {
		err := NewRateLimitError("rate limited", 10*time.Second)
		assert.Nil(t, err.Unwrap())
	})
}

func TestAgentCommunicationError_Is(t *testing.T) {
	t.Run("same type matches", func(t *testing.T) {
		err1 := NewNetworkError("err1", nil)
		err2 := NewNetworkError("err2", nil)
		assert.True(t, err1.Is(err2))
	})

	t.Run("different type does not match", func(t *testing.T) {
		err1 := NewNetworkError("err1", nil)
		err2 := NewAuthError("err2", nil)
		assert.False(t, err1.Is(err2))
	})

	t.Run("non-AgentCommunicationError does not match", func(t *testing.T) {
		err1 := NewNetworkError("err1", nil)
		err2 := errors.New("plain error")
		assert.False(t, err1.Is(err2))
	})
}

func TestAgentCommunicationError_IsRetryable(t *testing.T) {
	assert.True(t, NewNetworkError("net", nil).IsRetryable())
	assert.True(t, NewTimeoutError("timeout", nil).IsRetryable())
	assert.True(t, NewAPIError("api", nil).IsRetryable())
	assert.True(t, NewRateLimitError("rate", 1*time.Second).IsRetryable())
	assert.False(t, NewAuthError("auth", nil).IsRetryable())
	assert.False(t, NewPermanentError("perm", nil).IsRetryable())
}

func TestAgentCommunicationError_GetRetryAfter(t *testing.T) {
	assert.Equal(t, 5*time.Second, NewNetworkError("net", nil).GetRetryAfter())
	assert.Equal(t, 10*time.Second, NewTimeoutError("timeout", nil).GetRetryAfter())
	assert.Equal(t, 5*time.Second, NewAPIError("api", nil).GetRetryAfter())
	assert.Equal(t, 30*time.Second, NewRateLimitError("rate", 30*time.Second).GetRetryAfter())
	assert.Equal(t, time.Duration(0), NewAuthError("auth", nil).GetRetryAfter())
	assert.Equal(t, time.Duration(0), NewPermanentError("perm", nil).GetRetryAfter())
}

func TestAgentCommunicationError_WithContext(t *testing.T) {
	t.Run("adds context to nil map", func(t *testing.T) {
		err := NewNetworkError("net", nil)
		assert.Nil(t, err.Context)

		result := err.WithContext("key1", "value1")
		assert.Equal(t, err, result) // returns self
		assert.Equal(t, "value1", err.Context["key1"])
	})

	t.Run("adds multiple context values", func(t *testing.T) {
		err := NewNetworkError("net", nil)
		err.WithContext("key1", "value1").WithContext("key2", 42)

		assert.Equal(t, "value1", err.Context["key1"])
		assert.Equal(t, 42, err.Context["key2"])
	})
}

func TestClassifyError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		result := ClassifyError(nil)
		assert.Nil(t, result)
	})

	t.Run("already classified error is returned as-is", func(t *testing.T) {
		original := NewNetworkError("net error", nil)
		result := ClassifyError(original)
		assert.Equal(t, original, result)
	})

	t.Run("wrapped AgentCommunicationError is extracted", func(t *testing.T) {
		original := NewNetworkError("net error", nil)
		wrapped := fmt.Errorf("wrapping: %w", original)
		result := ClassifyError(wrapped)
		assert.Equal(t, original, result)
	})

	t.Run("HTTP 429 classified as rate limit", func(t *testing.T) {
		err := &httpStatusError{statusCode: 429}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeRateLimit, result.Type)
		assert.True(t, result.Retryable)
	})

	t.Run("HTTP 401 classified as auth", func(t *testing.T) {
		err := &httpStatusError{statusCode: 401}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAuthentication, result.Type)
		assert.False(t, result.Retryable)
	})

	t.Run("HTTP 403 classified as auth", func(t *testing.T) {
		err := &httpStatusError{statusCode: 403}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAuthentication, result.Type)
	})

	t.Run("HTTP 500 classified as API error", func(t *testing.T) {
		err := &httpStatusError{statusCode: 500}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAPI, result.Type)
		assert.True(t, result.Retryable)
	})

	t.Run("HTTP 502 classified as API error", func(t *testing.T) {
		err := &httpStatusError{statusCode: 502}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAPI, result.Type)
	})

	t.Run("HTTP 503 classified as API error", func(t *testing.T) {
		err := &httpStatusError{statusCode: 503}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAPI, result.Type)
	})

	t.Run("HTTP 504 classified as API error", func(t *testing.T) {
		err := &httpStatusError{statusCode: 504}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeAPI, result.Type)
	})

	t.Run("HTTP 404 classified as permanent", func(t *testing.T) {
		err := &httpStatusError{statusCode: 404}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypePermanent, result.Type)
		assert.False(t, result.Retryable)
	})

	t.Run("HTTP 400 classified as permanent", func(t *testing.T) {
		err := &httpStatusError{statusCode: 400}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypePermanent, result.Type)
	})

	t.Run("context.DeadlineExceeded classified as timeout", func(t *testing.T) {
		result := ClassifyError(context.DeadlineExceeded)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeTimeout, result.Type)
		assert.True(t, result.Retryable)
	})

	t.Run("timeout error via Timeout() interface", func(t *testing.T) {
		err := &timeoutError{isTimeout: true}
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeTimeout, result.Type)
	})

	t.Run("error message containing timeout", func(t *testing.T) {
		err := errors.New("operation timeout occurred")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeTimeout, result.Type)
	})

	t.Run("error message containing deadline exceeded", func(t *testing.T) {
		err := errors.New("deadline exceeded for request")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeTimeout, result.Type)
	})

	t.Run("connection refused classified as network", func(t *testing.T) {
		err := errors.New("dial tcp: connection refused")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeNetwork, result.Type)
		assert.True(t, result.Retryable)
	})

	t.Run("no such host classified as network", func(t *testing.T) {
		err := errors.New("lookup host: no such host")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeNetwork, result.Type)
	})

	t.Run("network unreachable classified as network", func(t *testing.T) {
		err := errors.New("network unreachable")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeNetwork, result.Type)
	})

	t.Run("connection reset classified as network", func(t *testing.T) {
		err := errors.New("connection reset by peer")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeNetwork, result.Type)
	})

	t.Run("unknown error classified as temporary", func(t *testing.T) {
		err := errors.New("something completely unknown happened")
		result := ClassifyError(err)
		require.NotNil(t, result)
		assert.Equal(t, ErrorTypeTemporary, result.Type)
		assert.True(t, result.Retryable)
		assert.Equal(t, 5*time.Second, result.RetryAfter)
	})
}

func TestPredefinedErrors(t *testing.T) {
	t.Run("ErrNetworkTimeout", func(t *testing.T) {
		assert.Equal(t, ErrorTypeTimeout, ErrNetworkTimeout.Type)
		assert.True(t, ErrNetworkTimeout.Retryable)
		assert.Equal(t, 5*time.Second, ErrNetworkTimeout.RetryAfter)
	})

	t.Run("ErrRateLimited", func(t *testing.T) {
		assert.Equal(t, ErrorTypeRateLimit, ErrRateLimited.Type)
		assert.True(t, ErrRateLimited.Retryable)
		assert.Equal(t, 60*time.Second, ErrRateLimited.RetryAfter)
	})

	t.Run("ErrAuthentication", func(t *testing.T) {
		assert.Equal(t, ErrorTypeAuthentication, ErrAuthentication.Type)
		assert.False(t, ErrAuthentication.Retryable)
	})

	t.Run("ErrPermanentFailure", func(t *testing.T) {
		assert.Equal(t, ErrorTypePermanent, ErrPermanentFailure.Type)
		assert.False(t, ErrPermanentFailure.Retryable)
	})
}

func TestIsTimeoutError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isTimeoutError(nil))
	})

	t.Run("context.DeadlineExceeded", func(t *testing.T) {
		assert.True(t, isTimeoutError(context.DeadlineExceeded))
	})

	t.Run("net.Error with Timeout() true", func(t *testing.T) {
		assert.True(t, isTimeoutError(&timeoutError{isTimeout: true}))
	})

	t.Run("net.Error with Timeout() false but timeout in message", func(t *testing.T) {
		// Note: even though Timeout() returns false, the error message contains "timeout"
		// so isTimeoutError returns true due to the string check fallback
		assert.True(t, isTimeoutError(&timeoutError{isTimeout: false}))
	})

	t.Run("non-timeout error with Timeout() false", func(t *testing.T) {
		assert.False(t, isTimeoutError(&nonTimeoutError{}))
	})

	t.Run("error message with timeout keyword", func(t *testing.T) {
		assert.True(t, isTimeoutError(errors.New("request timeout")))
	})

	t.Run("error message with deadline exceeded", func(t *testing.T) {
		assert.True(t, isTimeoutError(errors.New("deadline exceeded")))
	})

	t.Run("regular error without timeout keywords", func(t *testing.T) {
		assert.False(t, isTimeoutError(errors.New("something else")))
	})
}

func TestIsNetworkError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isNetworkError(nil))
	})

	t.Run("connection refused", func(t *testing.T) {
		assert.True(t, isNetworkError(errors.New("connection refused")))
	})

	t.Run("no such host", func(t *testing.T) {
		assert.True(t, isNetworkError(errors.New("no such host")))
	})

	t.Run("network unreachable", func(t *testing.T) {
		assert.True(t, isNetworkError(errors.New("network unreachable")))
	})

	t.Run("connection reset", func(t *testing.T) {
		assert.True(t, isNetworkError(errors.New("connection reset")))
	})

	t.Run("regular error", func(t *testing.T) {
		assert.False(t, isNetworkError(errors.New("some other error")))
	})
}

func TestContains(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		assert.True(t, contains("timeout", "timeout"))
	})

	t.Run("case insensitive match", func(t *testing.T) {
		assert.True(t, contains("A Connection Refused error", "connection refused"))
	})

	t.Run("substring at start", func(t *testing.T) {
		assert.True(t, contains("timeout occurred", "timeout"))
	})

	t.Run("substring at end", func(t *testing.T) {
		assert.True(t, contains("request timeout", "timeout"))
	})

	t.Run("substring in middle", func(t *testing.T) {
		assert.True(t, contains("a timeout occurred", "timeout"))
	})

	t.Run("no match", func(t *testing.T) {
		assert.False(t, contains("something else", "timeout"))
	})

	t.Run("empty substring", func(t *testing.T) {
		assert.True(t, contains("anything", ""))
	})

	t.Run("substring longer than string", func(t *testing.T) {
		assert.False(t, contains("ab", "abcdef"))
	})
}

func TestCircuitBreakerStateString(t *testing.T) {
	assert.Equal(t, "closed", StateClosed.String())
	assert.Equal(t, "open", StateOpen.String())
	assert.Equal(t, "half-open", StateHalfOpen.String())
	assert.Equal(t, "unknown", CircuitBreakerState(99).String())
}

func TestCircuitBreakerErrorMessage(t *testing.T) {
	err := &CircuitBreakerError{message: "test breaker error"}
	assert.Equal(t, "test breaker error", err.Error())
	assert.Equal(t, "circuit breaker is open", ErrCircuitBreakerOpen.Error())
}

// Helper types for testing ClassifyError

type httpStatusError struct {
	statusCode int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d", e.statusCode)
}

func (e *httpStatusError) StatusCode() int {
	return e.statusCode
}

type timeoutError struct {
	isTimeout bool
}

func (e *timeoutError) Error() string {
	return "timeout error"
}

func (e *timeoutError) Timeout() bool {
	return e.isTimeout
}

type nonTimeoutError struct{}

func (e *nonTimeoutError) Error() string {
	return "some regular error"
}

func (e *nonTimeoutError) Timeout() bool {
	return false
}
