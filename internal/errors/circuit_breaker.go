package errors

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
)

// CircuitBreakerState represents the current state of a circuit breaker
type CircuitBreakerState int32

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	// MaxFailures is the number of failures needed to open the circuit
	MaxFailures int64
	// Timeout is how long to wait before transitioning from open to half-open
	Timeout time.Duration
	// MaxRequests is the maximum number of requests allowed when half-open
	MaxRequests int64
	// FailureRatio is the ratio of failures to total requests that triggers opening
	FailureRatio float64
	// MinRequests is the minimum number of requests before considering failure ratio
	MinRequests int64
}

// DefaultCircuitBreakerConfig returns a sensible default configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxFailures:  5,
		Timeout:      60 * time.Second,
		MaxRequests:  3,
		FailureRatio: 0.6,
		MinRequests:  10,
	}
}

// CircuitBreakerError represents an error when the circuit breaker is open
type CircuitBreakerError struct {
	message string
}

func (e *CircuitBreakerError) Error() string {
	return e.message
}

var ErrCircuitBreakerOpen = &CircuitBreakerError{"circuit breaker is open"}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config              *CircuitBreakerConfig
	state               int32 // Use atomic operations for thread safety
	failures            int64
	requests            int64
	successiveSuccesses int64
	lastFailureTime     int64 // Unix timestamp
	logger              *slog.Logger
	structuredLogger    *observability.StructuredLogger
	metrics             *observability.Metrics
	name                string
	mu                  sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config *CircuitBreakerConfig, name string, logger *slog.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreaker{
		config: config,
		state:  int32(StateClosed),
		name:   name,
		logger: logger,
	}
}

// WithObservability adds observability features to the circuit breaker
func (cb *CircuitBreaker) WithObservability(structuredLogger *observability.StructuredLogger, metrics *observability.Metrics) *CircuitBreaker {
	cb.structuredLogger = structuredLogger
	cb.metrics = metrics
	return cb
}

// Execute runs the given function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	// Check if we can execute
	canExecute, state := cb.canExecute()
	if !canExecute {
		if cb.metrics != nil {
			cb.metrics.Inc("circuit_breaker_rejected", map[string]string{
				"name":  cb.name,
				"state": state.String(),
			})
		}

		if cb.structuredLogger != nil {
			cb.structuredLogger.LogCircuitBreakerRejection(ctx, cb.name, state.String())
		}

		cb.logger.Debug("circuit breaker rejected request",
			"name", cb.name,
			"state", state.String(),
			"correlation_id", observability.GetCorrelationID(ctx))

		return ErrCircuitBreakerOpen
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	cb.recordResult(ctx, err)

	return err
}

// canExecute determines if a request can be executed based on the current state
func (cb *CircuitBreaker) canExecute() (bool, CircuitBreakerState) {
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	now := time.Now().Unix()

	switch state {
	case StateClosed:
		return true, state

	case StateOpen:
		// Check if timeout has elapsed
		lastFailure := atomic.LoadInt64(&cb.lastFailureTime)
		if now-lastFailure >= int64(cb.config.Timeout.Seconds()) {
			// Transition to half-open
			if atomic.CompareAndSwapInt32(&cb.state, int32(StateOpen), int32(StateHalfOpen)) {
				atomic.StoreInt64(&cb.successiveSuccesses, 0)
				cb.logStateTransition(StateOpen, StateHalfOpen)
			}
			return true, StateHalfOpen
		}
		return false, state

	case StateHalfOpen:
		// Allow limited requests
		requests := atomic.LoadInt64(&cb.requests)
		return requests < cb.config.MaxRequests, state

	default:
		return false, state
	}
}

// recordResult records the result of an operation and updates the circuit breaker state
func (cb *CircuitBreaker) recordResult(ctx context.Context, err error) {
	correlationID := observability.GetCorrelationID(ctx)
	atomic.AddInt64(&cb.requests, 1)

	if err != nil {
		cb.recordFailure(ctx, correlationID)
	} else {
		cb.recordSuccess(ctx, correlationID)
	}
}

// recordFailure handles a failed operation
func (cb *CircuitBreaker) recordFailure(ctx context.Context, correlationID string) {
	failures := atomic.AddInt64(&cb.failures, 1)
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().Unix())
	atomic.StoreInt64(&cb.successiveSuccesses, 0)

	currentState := CircuitBreakerState(atomic.LoadInt32(&cb.state))

	if cb.metrics != nil {
		cb.metrics.Inc("circuit_breaker_failure", map[string]string{
			"name":  cb.name,
			"state": currentState.String(),
		})
	}

	switch currentState {
	case StateClosed:
		if cb.shouldOpen() {
			if atomic.CompareAndSwapInt32(&cb.state, int32(StateClosed), int32(StateOpen)) {
				cb.logStateTransition(StateClosed, StateOpen)
			}
		}

	case StateHalfOpen:
		// Any failure in half-open state should open the circuit
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateOpen)) {
			cb.logStateTransition(StateHalfOpen, StateOpen)
		}
	}

	cb.logger.Debug("circuit breaker recorded failure",
		"name", cb.name,
		"state", currentState.String(),
		"failures", failures,
		"correlation_id", correlationID)
}

// recordSuccess handles a successful operation
func (cb *CircuitBreaker) recordSuccess(ctx context.Context, correlationID string) {
	successes := atomic.AddInt64(&cb.successiveSuccesses, 1)
	currentState := CircuitBreakerState(atomic.LoadInt32(&cb.state))

	if cb.metrics != nil {
		cb.metrics.Inc("circuit_breaker_success", map[string]string{
			"name":  cb.name,
			"state": currentState.String(),
		})
	}

	if currentState == StateHalfOpen && successes >= cb.config.MaxRequests {
		// Close the circuit after enough successful requests
		if atomic.CompareAndSwapInt32(&cb.state, int32(StateHalfOpen), int32(StateClosed)) {
			// Reset counters
			atomic.StoreInt64(&cb.failures, 0)
			atomic.StoreInt64(&cb.requests, 0)
			cb.logStateTransition(StateHalfOpen, StateClosed)
		}
	}

	cb.logger.Debug("circuit breaker recorded success",
		"name", cb.name,
		"state", currentState.String(),
		"successive_successes", successes,
		"correlation_id", correlationID)
}

// shouldOpen determines if the circuit should be opened based on current metrics
func (cb *CircuitBreaker) shouldOpen() bool {
	failures := atomic.LoadInt64(&cb.failures)
	requests := atomic.LoadInt64(&cb.requests)

	// Check absolute failure threshold
	if failures >= cb.config.MaxFailures {
		return true
	}

	// Check failure ratio threshold
	if requests >= cb.config.MinRequests {
		failureRatio := float64(failures) / float64(requests)
		if failureRatio >= cb.config.FailureRatio {
			return true
		}
	}

	return false
}

// logStateTransition logs a state transition
func (cb *CircuitBreaker) logStateTransition(from, to CircuitBreakerState) {
	if cb.structuredLogger != nil {
		cb.structuredLogger.LogCircuitBreakerStateChange(context.Background(), cb.name, from.String(), to.String())
	}

	if cb.metrics != nil {
		cb.metrics.Inc("circuit_breaker_state_change", map[string]string{
			"name": cb.name,
			"from": from.String(),
			"to":   to.String(),
		})
	}

	cb.logger.Info("circuit breaker state changed",
		"name", cb.name,
		"from", from.String(),
		"to", to.String(),
		"failures", atomic.LoadInt64(&cb.failures),
		"requests", atomic.LoadInt64(&cb.requests))
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitBreakerState {
	return CircuitBreakerState(atomic.LoadInt32(&cb.state))
}

// Stats returns current statistics for the circuit breaker
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	return CircuitBreakerStats{
		State:               cb.State(),
		Failures:            atomic.LoadInt64(&cb.failures),
		Requests:            atomic.LoadInt64(&cb.requests),
		SuccessiveSuccesses: atomic.LoadInt64(&cb.successiveSuccesses),
		LastFailureTime:     time.Unix(atomic.LoadInt64(&cb.lastFailureTime), 0),
	}
}

// CircuitBreakerStats represents the current statistics of a circuit breaker
type CircuitBreakerStats struct {
	State               CircuitBreakerState
	Failures            int64
	Requests            int64
	SuccessiveSuccesses int64
	LastFailureTime     time.Time
}

// CircuitBreakerDecorator wraps a function with circuit breaker protection
func CircuitBreakerDecorator(cb *CircuitBreaker, fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		return cb.Execute(ctx, fn)
	}
}

// CombinedDecorator combines circuit breaker and retry logic
func CombinedDecorator[T any](cb *CircuitBreaker, retryer *Retryer, fn func(context.Context) (T, error)) func(context.Context) (T, error) {
	return func(ctx context.Context) (T, error) {
		var result T
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var err error
			result, err = Execute(ctx, retryer, func(ctx context.Context, attempt int) (T, error) {
				return fn(ctx)
			})
			return err
		})
		return result, err
	}
}
