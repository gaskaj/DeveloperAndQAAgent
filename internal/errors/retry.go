package errors

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
)

// RetryPolicy defines how retries should be performed
type RetryPolicy struct {
	MaxAttempts     int
	BaseDelay       time.Duration
	MaxDelay        time.Duration
	BackoffFactor   float64
	JitterFactor    float64
	RetryableErrors []ErrorType
}

// DefaultRetryPolicy returns a sensible default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
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
	}
}

// NetworkRetryPolicy returns a policy optimized for network operations
func NetworkRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
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
	}
}

// RateLimitRetryPolicy returns a policy optimized for rate-limited APIs
func RateLimitRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:   3,
		BaseDelay:     60 * time.Second,
		MaxDelay:      300 * time.Second,
		BackoffFactor: 1.5,
		JitterFactor:  0.1,
		RetryableErrors: []ErrorType{
			ErrorTypeRateLimit,
			ErrorTypeAPI,
			ErrorTypeTemporary,
		},
	}
}

// NoRetryPolicy returns a policy that disables retries
func NoRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:     1,
		RetryableErrors: []ErrorType{},
	}
}

// Retryer handles retry logic with observability
type Retryer struct {
	policy           *RetryPolicy
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
	operationName    string
}

// NewRetryer creates a new retryer with the given policy
func NewRetryer(policy *RetryPolicy, logger *slog.Logger) *Retryer {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}
	return &Retryer{
		policy: policy,
		logger: logger,
	}
}

// WithObservability adds observability features to the retryer
func (r *Retryer) WithObservability(structuredLogger *observability.StructuredLogger, metrics *observability.Metrics) *Retryer {
	r.structuredLogger = structuredLogger
	r.metrics = metrics
	return r
}

// WithOperationName sets the operation name for logging and metrics
func (r *Retryer) WithOperationName(name string) *Retryer {
	r.operationName = name
	return r
}

// ExecuteFunc is the function type that can be retried
type ExecuteFunc[T any] func(ctx context.Context, attempt int) (T, error)

// Execute runs the given function with retry logic
func Execute[T any](ctx context.Context, retryer *Retryer, fn ExecuteFunc[T]) (T, error) {
	var zero T

	if retryer == nil {
		retryer = NewRetryer(DefaultRetryPolicy(), slog.Default())
	}

	correlationID := observability.GetCorrelationID(ctx)
	operationName := retryer.operationName
	if operationName == "" {
		operationName = "unknown_operation"
	}

	// Track overall operation metrics
	var timer *observability.Timer
	if retryer.metrics != nil {
		timer = retryer.metrics.Timer("retry_operation", map[string]string{
			"operation": operationName,
		})
	}

	var lastErr error
	for attempt := 1; attempt <= retryer.policy.MaxAttempts; attempt++ {
		// Log attempt start
		if retryer.structuredLogger != nil {
			retryer.structuredLogger.LogRetryAttempt(ctx, operationName, attempt, retryer.policy.MaxAttempts)
		}

		retryer.logger.Debug("executing operation",
			"operation", operationName,
			"attempt", attempt,
			"max_attempts", retryer.policy.MaxAttempts,
			"correlation_id", correlationID)

		// Execute the function
		result, err := fn(ctx, attempt)

		// If no error, we're done
		if err == nil {
			if timer != nil {
				timer.StopWithContext(ctx, "retry_operation")
			}

			if retryer.structuredLogger != nil {
				retryer.structuredLogger.LogRetrySuccess(ctx, operationName, attempt)
			}

			if retryer.metrics != nil {
				retryer.metrics.Inc("retry_success", map[string]string{
					"operation": operationName,
					"attempts":  fmt.Sprintf("%d", attempt),
				})
			}

			retryer.logger.Debug("operation succeeded",
				"operation", operationName,
				"attempt", attempt,
				"correlation_id", correlationID)

			return result, nil
		}

		lastErr = err

		// Classify the error
		agentErr := ClassifyError(err)

		// Check if this is the last attempt
		if attempt >= retryer.policy.MaxAttempts {
			if timer != nil {
				timer.StopWithContext(ctx, "retry_operation")
			}

			if retryer.metrics != nil {
				retryer.metrics.Inc("retry_exhausted", map[string]string{
					"operation":  operationName,
					"error_type": string(agentErr.Type),
					"attempts":   fmt.Sprintf("%d", attempt),
				})
			}

			if retryer.structuredLogger != nil {
				retryer.structuredLogger.LogRetryExhausted(ctx, operationName, attempt, err)
			}

			retryer.logger.Error("operation failed after all retry attempts",
				"operation", operationName,
				"attempts", attempt,
				"error", err,
				"correlation_id", correlationID)

			return zero, fmt.Errorf("operation failed after %d attempts: %w", attempt, err)
		}

		// Check if error is retryable
		if !retryer.shouldRetry(agentErr) {
			if timer != nil {
				timer.StopWithContext(ctx, "retry_operation")
			}

			if retryer.metrics != nil {
				retryer.metrics.Inc("retry_non_retryable", map[string]string{
					"operation":  operationName,
					"error_type": string(agentErr.Type),
					"attempt":    fmt.Sprintf("%d", attempt),
				})
			}

			if retryer.structuredLogger != nil {
				retryer.structuredLogger.LogRetryNonRetryable(ctx, operationName, attempt, err)
			}

			retryer.logger.Warn("operation failed with non-retryable error",
				"operation", operationName,
				"attempt", attempt,
				"error_type", agentErr.Type,
				"error", err,
				"correlation_id", correlationID)

			return zero, fmt.Errorf("non-retryable error: %w", err)
		}

		// Calculate delay
		delay := retryer.calculateDelay(attempt, agentErr)

		if retryer.metrics != nil {
			retryer.metrics.Inc("retry_attempt", map[string]string{
				"operation":  operationName,
				"error_type": string(agentErr.Type),
				"attempt":    fmt.Sprintf("%d", attempt),
			})
		}

		if retryer.structuredLogger != nil {
			retryer.structuredLogger.LogRetryDelay(ctx, operationName, attempt, delay, err)
		}

		retryer.logger.Warn("operation failed, retrying",
			"operation", operationName,
			"attempt", attempt,
			"error_type", agentErr.Type,
			"delay", delay,
			"error", err,
			"correlation_id", correlationID)

		// Wait for the delay or context cancellation
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.StopWithContext(ctx, "retry_operation")
			}
			return zero, fmt.Errorf("operation cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// This shouldn't be reached, but just in case
	if timer != nil {
		timer.StopWithContext(ctx, "retry_operation")
	}
	return zero, fmt.Errorf("retry logic error: %w", lastErr)
}

// shouldRetry determines if an error should be retried based on the policy
func (r *Retryer) shouldRetry(err *AgentCommunicationError) bool {
	if err == nil || !err.IsRetryable() {
		return false
	}

	// Check if error type is in the retryable list
	for _, retryableType := range r.policy.RetryableErrors {
		if err.Type == retryableType {
			return true
		}
	}

	return false
}

// calculateDelay computes the delay before the next retry attempt
func (r *Retryer) calculateDelay(attempt int, err *AgentCommunicationError) time.Duration {
	// Use error-specific retry after if provided
	if err.RetryAfter > 0 {
		return r.addJitter(err.RetryAfter)
	}

	// Calculate exponential backoff
	delay := float64(r.policy.BaseDelay) * math.Pow(r.policy.BackoffFactor, float64(attempt-1))
	delayDuration := time.Duration(delay)

	// Cap at max delay
	if delayDuration > r.policy.MaxDelay {
		delayDuration = r.policy.MaxDelay
	}

	return r.addJitter(delayDuration)
}

// addJitter adds random jitter to a delay to avoid thundering herd problems
func (r *Retryer) addJitter(delay time.Duration) time.Duration {
	if r.policy.JitterFactor <= 0 {
		return delay
	}

	jitter := float64(delay) * r.policy.JitterFactor * (2*rand.Float64() - 1) // -jitterFactor to +jitterFactor
	jitteredDelay := delay + time.Duration(jitter)

	// Ensure we don't go negative
	if jitteredDelay < 0 {
		return delay / 2
	}

	return jitteredDelay
}

// RetryDecorator is a function decorator that adds retry logic
func RetryDecorator[T any](retryer *Retryer, fn func(context.Context) (T, error)) func(context.Context) (T, error) {
	return func(ctx context.Context) (T, error) {
		return Execute(ctx, retryer, func(ctx context.Context, attempt int) (T, error) {
			return fn(ctx)
		})
	}
}

// RetryDecoratorWithAttempt is like RetryDecorator but passes the attempt number to the function
func RetryDecoratorWithAttempt[T any](retryer *Retryer, fn ExecuteFunc[T]) func(context.Context) (T, error) {
	return func(ctx context.Context) (T, error) {
		return Execute(ctx, retryer, fn)
	}
}
