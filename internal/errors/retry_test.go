package errors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   *RetryPolicy
		expected *RetryPolicy
	}{
		{
			name:   "default policy",
			policy: DefaultRetryPolicy(),
			expected: &RetryPolicy{
				MaxAttempts:   3,
				BaseDelay:     1 * time.Second,
				MaxDelay:      30 * time.Second,
				BackoffFactor: 2.0,
				JitterFactor:  0.1,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeTimeout,
					ErrorTypeAPI,
					ErrorTypeTemporary,
				},
			},
		},
		{
			name:   "network policy",
			policy: NetworkRetryPolicy(),
			expected: &RetryPolicy{
				MaxAttempts:   5,
				BaseDelay:     2 * time.Second,
				MaxDelay:      60 * time.Second,
				BackoffFactor: 2.0,
				JitterFactor:  0.2,
				RetryableErrors: []ErrorType{
					ErrorTypeNetwork,
					ErrorTypeTimeout,
					ErrorTypeAPI,
					ErrorTypeTemporary,
				},
			},
		},
		{
			name:   "no retry policy",
			policy: NoRetryPolicy(),
			expected: &RetryPolicy{
				MaxAttempts:     1,
				RetryableErrors: []ErrorType{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected.MaxAttempts, tt.policy.MaxAttempts)
			assert.Equal(t, tt.expected.BaseDelay, tt.policy.BaseDelay)
			assert.Equal(t, tt.expected.MaxDelay, tt.policy.MaxDelay)
			assert.Equal(t, tt.expected.BackoffFactor, tt.policy.BackoffFactor)
			assert.Equal(t, tt.expected.JitterFactor, tt.policy.JitterFactor)
			assert.Equal(t, tt.expected.RetryableErrors, tt.policy.RetryableErrors)
		})
	}
}

func TestRetryExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("success on first attempt", func(t *testing.T) {
		retryer := NewRetryer(DefaultRetryPolicy(), logger)
		ctx := context.Background()

		callCount := 0
		result, err := Execute(ctx, retryer, func(ctx context.Context, attempt int) (string, error) {
			callCount++
			return "success", nil
		})

		require.NoError(t, err)
		assert.Equal(t, "success", result)
		assert.Equal(t, 1, callCount)
	})

	t.Run("success after retries", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			MaxAttempts:     3,
			BaseDelay:       10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			BackoffFactor:   2.0,
			JitterFactor:    0.0, // No jitter for predictable testing
			RetryableErrors: []ErrorType{ErrorTypeTemporary, ErrorTypeAPI},
		}, logger)
		ctx := context.Background()

		callCount := 0
		result, err := Execute(ctx, retryer, func(ctx context.Context, attempt int) (string, error) {
			callCount++
			if callCount < 3 {
				return "", NewAPIError("temporary failure", errors.New("api error"))
			}
			return "success", nil
		})

		require.NoError(t, err)
		assert.Equal(t, "success", result)
		assert.Equal(t, 3, callCount)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		retryer := NewRetryer(DefaultRetryPolicy(), logger)
		ctx := context.Background()

		callCount := 0
		result, err := Execute(ctx, retryer, func(ctx context.Context, attempt int) (string, error) {
			callCount++
			return "", NewAuthError("authentication failed", errors.New("auth error"))
		})

		require.Error(t, err)
		assert.Empty(t, result)
		assert.Equal(t, 1, callCount) // Should not retry auth errors
		assert.Contains(t, err.Error(), "non-retryable error")
	})

	t.Run("retry exhaustion", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			MaxAttempts:     2,
			BaseDelay:       10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			BackoffFactor:   2.0,
			JitterFactor:    0.0,
			RetryableErrors: []ErrorType{ErrorTypeTemporary, ErrorTypeAPI},
		}, logger)
		ctx := context.Background()

		callCount := 0
		result, err := Execute(ctx, retryer, func(ctx context.Context, attempt int) (string, error) {
			callCount++
			return "", NewAPIError("always fails", errors.New("persistent error"))
		})

		require.Error(t, err)
		assert.Empty(t, result)
		assert.Equal(t, 2, callCount) // Should retry once
		assert.Contains(t, err.Error(), "operation failed after 2 attempts")
	})

	t.Run("context cancellation", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			MaxAttempts:     5,
			BaseDelay:       100 * time.Millisecond,
			MaxDelay:        1 * time.Second,
			BackoffFactor:   2.0,
			JitterFactor:    0.0,
			RetryableErrors: []ErrorType{ErrorTypeTemporary, ErrorTypeAPI},
		}, logger)

		ctx, cancel := context.WithCancel(context.Background())

		callCount := 0
		go func() {
			// Cancel after first failure
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := Execute(ctx, retryer, func(ctx context.Context, attempt int) (string, error) {
			callCount++
			return "", NewAPIError("temporary failure", errors.New("api error"))
		})

		require.Error(t, err)
		assert.Empty(t, result)
		assert.Equal(t, 1, callCount) // Should be interrupted during delay
		assert.Contains(t, err.Error(), "operation cancelled during retry")
	})
}

func TestRetryDecorator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(&RetryPolicy{
		MaxAttempts:     3,
		BaseDelay:       10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterFactor:    0.0,
		RetryableErrors: []ErrorType{ErrorTypeTemporary, ErrorTypeAPI},
	}, logger)

	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		if callCount < 2 {
			return "", NewAPIError("temporary failure", errors.New("api error"))
		}
		return "decorated success", nil
	}

	decoratedFn := RetryDecorator(retryer, fn)

	result, err := decoratedFn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "decorated success", result)
	assert.Equal(t, 2, callCount)
}

func TestBackoffCalculation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(&RetryPolicy{
		MaxAttempts:     5,
		BaseDelay:       1 * time.Second,
		MaxDelay:        10 * time.Second,
		BackoffFactor:   2.0,
		JitterFactor:    0.0, // No jitter for predictable testing
		RetryableErrors: []ErrorType{ErrorTypeTemporary},
	}, logger)

	testCases := []struct {
		attempt       int
		expectedDelay time.Duration
	}{
		{1, 1 * time.Second},  // Base delay
		{2, 2 * time.Second},  // 1s * 2^1
		{3, 4 * time.Second},  // 1s * 2^2
		{4, 8 * time.Second},  // 1s * 2^3
		{5, 10 * time.Second}, // Capped at max delay
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
			// Use an error without RetryAfter set so exponential backoff is tested
			err := &AgentCommunicationError{
				Type:      ErrorTypeAPI,
				Message:   "test error",
				Retryable: true,
			}
			delay := retryer.calculateDelay(tc.attempt, err)
			assert.Equal(t, tc.expectedDelay, delay)
		})
	}
}

func TestErrorSpecificRetryAfter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(DefaultRetryPolicy(), logger)

	// Test that error-specific retry after takes precedence
	customRetryAfter := 5 * time.Second
	err := NewRateLimitError("rate limited", customRetryAfter)
	delay := retryer.calculateDelay(1, err)

	// Should use error-specific retry after (with potential jitter)
	assert.True(t, delay >= time.Duration(float64(customRetryAfter)*0.9)) // Account for negative jitter
	assert.True(t, delay <= time.Duration(float64(customRetryAfter)*1.1)) // Account for positive jitter
}

func TestShouldRetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(DefaultRetryPolicy(), logger)

	tests := []struct {
		name        string
		err         *AgentCommunicationError
		shouldRetry bool
	}{
		{
			name:        "network error",
			err:         NewNetworkError("connection failed", errors.New("network")),
			shouldRetry: true,
		},
		{
			name:        "timeout error",
			err:         NewTimeoutError("timed out", errors.New("timeout")),
			shouldRetry: true,
		},
		{
			name:        "auth error",
			err:         NewAuthError("unauthorized", errors.New("auth")),
			shouldRetry: false,
		},
		{
			name:        "permanent error",
			err:         NewPermanentError("not found", errors.New("404")),
			shouldRetry: false,
		},
		{
			name:        "nil error",
			err:         nil,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retryer.shouldRetry(tt.err)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

func TestRateLimitRetryPolicy(t *testing.T) {
	policy := RateLimitRetryPolicy()
	assert.Equal(t, 3, policy.MaxAttempts)
	assert.Equal(t, 60*time.Second, policy.BaseDelay)
	assert.Equal(t, 300*time.Second, policy.MaxDelay)
	assert.Equal(t, 1.5, policy.BackoffFactor)
	assert.Equal(t, 0.1, policy.JitterFactor)
	assert.Contains(t, policy.RetryableErrors, ErrorTypeRateLimit)
	assert.Contains(t, policy.RetryableErrors, ErrorTypeAPI)
	assert.Contains(t, policy.RetryableErrors, ErrorTypeTemporary)
}

func TestNewRetryer_NilPolicy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(nil, logger)
	require.NotNil(t, retryer)
	assert.Equal(t, 3, retryer.policy.MaxAttempts) // Uses DefaultRetryPolicy
}

func TestRetryer_WithOperationName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(DefaultRetryPolicy(), logger).WithOperationName("test_op")
	assert.Equal(t, "test_op", retryer.operationName)
}

func TestRetryer_WithObservability(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(DefaultRetryPolicy(), logger).WithObservability(nil, nil)
	assert.Nil(t, retryer.structuredLogger)
	assert.Nil(t, retryer.metrics)
}

func TestExecute_NilRetryer(t *testing.T) {
	ctx := context.Background()
	result, err := Execute(ctx, nil, func(ctx context.Context, attempt int) (string, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestRetryDecoratorWithAttempt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(&RetryPolicy{
		MaxAttempts:     3,
		BaseDelay:       10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterFactor:    0.0,
		RetryableErrors: []ErrorType{ErrorTypeTemporary, ErrorTypeAPI},
	}, logger)

	callCount := 0
	fn := func(ctx context.Context, attempt int) (string, error) {
		callCount++
		if attempt < 2 {
			return "", NewAPIError("temp", errors.New("api error"))
		}
		return fmt.Sprintf("success on attempt %d", attempt), nil
	}

	decoratedFn := RetryDecoratorWithAttempt(retryer, fn)
	result, err := decoratedFn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "success on attempt 2", result)
	assert.Equal(t, 2, callCount)
}

func TestAddJitter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("zero jitter factor returns original delay", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			JitterFactor: 0,
		}, logger)
		delay := retryer.addJitter(1 * time.Second)
		assert.Equal(t, 1*time.Second, delay)
	})

	t.Run("negative jitter factor returns original delay", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			JitterFactor: -0.1,
		}, logger)
		delay := retryer.addJitter(1 * time.Second)
		assert.Equal(t, 1*time.Second, delay)
	})

	t.Run("positive jitter adds variance", func(t *testing.T) {
		retryer := NewRetryer(&RetryPolicy{
			JitterFactor: 0.5,
		}, logger)
		baseDelay := 1 * time.Second
		// Run multiple times to verify jitter is within range
		for i := 0; i < 100; i++ {
			delay := retryer.addJitter(baseDelay)
			// Should be within +/- 50% of base delay
			assert.True(t, delay >= time.Duration(float64(baseDelay)*0.5), "delay too small: %v", delay)
			assert.True(t, delay <= time.Duration(float64(baseDelay)*1.5), "delay too large: %v", delay)
		}
	})
}

func TestCalculateDelay_WithRetryAfter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(&RetryPolicy{
		BaseDelay:     1 * time.Second,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.0,
	}, logger)

	// Error with RetryAfter should use that value, not exponential backoff
	err := NewRateLimitError("rate limited", 30*time.Second)
	delay := retryer.calculateDelay(1, err)
	assert.Equal(t, 30*time.Second, delay)
}

func TestCalculateDelay_MaxDelayCap(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	retryer := NewRetryer(&RetryPolicy{
		BaseDelay:     1 * time.Second,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 10.0, // Very aggressive backoff
		JitterFactor:  0.0,
	}, logger)

	err := &AgentCommunicationError{
		Type:      ErrorTypeAPI,
		Retryable: true,
	}
	delay := retryer.calculateDelay(5, err) // 1s * 10^4 = 10000s, should be capped at 5s
	assert.Equal(t, 5*time.Second, delay)
}
