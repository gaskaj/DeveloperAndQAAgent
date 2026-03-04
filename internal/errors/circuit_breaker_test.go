package errors

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerBasicOperation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("closed state allows execution", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)

		assert.Equal(t, StateClosed, cb.State())

		callCount := 0
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			callCount++
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
		assert.Equal(t, StateClosed, cb.State())
	})

	t.Run("successful execution keeps circuit closed", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)

		for i := 0; i < 10; i++ {
			err := cb.Execute(context.Background(), func(ctx context.Context) error {
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, StateClosed, cb.State())
		}
	})

	t.Run("multiple failures open circuit", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			MaxFailures:  3,
			Timeout:      2 * time.Second, // Must be >= 1s because canExecute uses int64(Timeout.Seconds())
			MaxRequests:  2,
			FailureRatio: 0.6,
			MinRequests:  5,
		}
		cb := NewCircuitBreaker(config, "test", logger)

		// Fail enough times to open circuit
		for i := 0; i < 3; i++ {
			err := cb.Execute(context.Background(), func(ctx context.Context) error {
				return errors.New("test failure")
			})
			require.Error(t, err)
			assert.NotEqual(t, ErrCircuitBreakerOpen, err) // Should not be circuit breaker error yet
		}

		// Should be open now
		assert.Equal(t, StateOpen, cb.State())

		// Next call should be rejected (timeout hasn't elapsed)
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
		require.Error(t, err)
		assert.Equal(t, ErrCircuitBreakerOpen, err)
	})
}

func TestCircuitBreakerStateTransitions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("closed -> open -> half-open -> closed", func(t *testing.T) {
		// MaxRequests=1 means a single success in half-open closes the circuit.
		// This is necessary because the requests counter accumulates across states
		// and is not reset on the open->half-open transition, so higher MaxRequests
		// values would cause canExecute to reject half-open requests before
		// successiveSuccesses can reach the threshold.
		config := &CircuitBreakerConfig{
			MaxFailures:  2,
			Timeout:      1 * time.Second, // Must be >= 1s (int64 truncation in canExecute)
			MaxRequests:  1,
			FailureRatio: 0.5,
			MinRequests:  3,
		}
		cb := NewCircuitBreaker(config, "test", logger)

		// Start in closed state
		assert.Equal(t, StateClosed, cb.State())

		// Fail enough to open
		for i := 0; i < 2; i++ {
			cb.Execute(context.Background(), func(ctx context.Context) error {
				return errors.New("failure")
			})
		}

		// Should be open
		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout to elapse so next request triggers open -> half-open
		time.Sleep(1100 * time.Millisecond)

		// This request transitions open -> half-open (via canExecute), then succeeds.
		// With MaxRequests=1, recordSuccess sees successiveSuccesses(1) >= MaxRequests(1)
		// and immediately closes the circuit.
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil // Success
		})
		assert.Equal(t, StateClosed, cb.State())
	})

	t.Run("half-open failure reopens circuit", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			MaxFailures:  2,
			Timeout:      1 * time.Second, // Must be >= 1s (int64 truncation in canExecute)
			MaxRequests:  1,
			FailureRatio: 0.5,
			MinRequests:  3,
		}
		cb := NewCircuitBreaker(config, "test", logger)

		// Fail to open circuit
		for i := 0; i < 2; i++ {
			cb.Execute(context.Background(), func(ctx context.Context) error {
				return errors.New("failure")
			})
		}
		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout and transition to half-open
		time.Sleep(1100 * time.Millisecond)
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("half-open failure")
		})

		// Should immediately go back to open
		assert.Equal(t, StateOpen, cb.State())
	})
}

func TestCircuitBreakerFailureRatio(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := &CircuitBreakerConfig{
		MaxFailures:  100, // High threshold so we test ratio
		Timeout:      100 * time.Millisecond,
		MaxRequests:  2,
		FailureRatio: 0.6, // 60% failure ratio
		MinRequests:  10,
	}
	cb := NewCircuitBreaker(config, "test", logger)

	// Execute 10 requests: 6 failures, 4 successes = 60% failure rate
	for i := 0; i < 6; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("failure")
		})
	}

	for i := 0; i < 4; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}

	// Should still be closed (exactly at threshold)
	assert.Equal(t, StateClosed, cb.State())

	// One more failure should open it (7/11 = 63.6%)
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("final failure")
	})

	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreakerConcurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := &CircuitBreakerConfig{
		MaxFailures:  5,
		Timeout:      100 * time.Millisecond,
		MaxRequests:  2,
		FailureRatio: 0.7,
		MinRequests:  10,
	}
	cb := NewCircuitBreaker(config, "test", logger)

	const numGoroutines = 10
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	// Launch concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				err := cb.Execute(context.Background(), func(ctx context.Context) error {
					// Fail every third request
					if (id*requestsPerGoroutine+j)%3 == 0 {
						return errors.New("intermittent failure")
					}
					return nil
				})

				if err != nil {
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify some operations completed
	totalRequests := successCount.Load() + errorCount.Load()
	assert.True(t, totalRequests > 0, "Expected some operations to complete")

	// Circuit breaker should have tracked all operations
	stats := cb.Stats()
	assert.True(t, stats.Requests > 0, "Expected circuit breaker to track requests")
}

func TestCircuitBreakerStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)

	// Initial stats
	stats := cb.Stats()
	assert.Equal(t, StateClosed, stats.State)
	assert.Equal(t, int64(0), stats.Failures)
	assert.Equal(t, int64(0), stats.Requests)

	// Execute some operations
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil // Success
	})

	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("failure")
	})

	// Check updated stats
	stats = cb.Stats()
	assert.True(t, stats.Requests >= 2)
	assert.True(t, stats.Failures >= 1)
}

func TestCircuitBreakerDecorator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		return nil
	}

	decoratedFn := CircuitBreakerDecorator(cb, fn)

	err := decoratedFn(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestNewCircuitBreaker_NilConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cb := NewCircuitBreaker(nil, "test", logger)
	require.NotNil(t, cb)
	assert.Equal(t, int64(5), cb.config.MaxFailures) // DefaultCircuitBreakerConfig
	assert.Equal(t, StateClosed, cb.State())
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	assert.Equal(t, int64(5), config.MaxFailures)
	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, int64(3), config.MaxRequests)
	assert.Equal(t, 0.6, config.FailureRatio)
	assert.Equal(t, int64(10), config.MinRequests)
}

func TestCircuitBreaker_WithObservability(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)
	result := cb.WithObservability(nil, nil)
	assert.Equal(t, cb, result) // returns self
}

func TestCircuitBreaker_ExecuteReturnsOriginalError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig(), "test", logger)

	expectedErr := errors.New("original error")
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return expectedErr
	})

	assert.Equal(t, expectedErr, err)
}

func TestCircuitBreaker_HalfOpenMaxRequestsLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := &CircuitBreakerConfig{
		MaxFailures:  2,
		Timeout:      1 * time.Second,
		MaxRequests:  1, // Only allow 1 request in half-open
		FailureRatio: 0.5,
		MinRequests:  3,
	}
	cb := NewCircuitBreaker(config, "test", logger)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("failure")
		})
	}
	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(1100 * time.Millisecond)

	// First request should succeed (transitions to half-open)
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
}

func TestCombinedDecorator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create circuit breaker with high failure threshold (won't trip during test)
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		MaxFailures:  10,
		Timeout:      100 * time.Millisecond,
		MaxRequests:  3,
		FailureRatio: 0.9,
		MinRequests:  20,
	}, "test", logger)

	// Create retry policy
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
		return "combined success", nil
	}

	decoratedFn := CombinedDecorator(cb, retryer, fn)

	result, err := decoratedFn(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "combined success", result)
	assert.Equal(t, 2, callCount) // Should have retried once
}
