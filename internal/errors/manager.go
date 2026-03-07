package errors

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
)

// Manager provides centralized error handling, retry, and circuit breaker management
type Manager struct {
	config           *config.ErrorHandlingConfig
	retryPolicies    map[string]*RetryPolicy
	circuitBreakers  map[string]*CircuitBreaker
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
	mu               sync.RWMutex
}

// NewManager creates a new error handling manager with the given configuration
func NewManager(cfg *config.ErrorHandlingConfig, logger *slog.Logger) *Manager {
	if cfg == nil {
		// Use default configuration
		cfg = &config.ErrorHandlingConfig{
			Retry: config.RetryConfig{
				Enabled: true,
				DefaultPolicy: config.RetryPolicyConfig{
					MaxAttempts:     3,
					BaseDelay:       1 * time.Second,
					MaxDelay:        30 * time.Second,
					BackoffFactor:   2.0,
					JitterFactor:    0.1,
					RetryableErrors: []string{"network", "timeout", "api", "temporary"},
				},
				Policies: make(map[string]config.RetryPolicyConfig),
			},
			CircuitBreaker: config.CircuitBreakerGroupConfig{
				Enabled: true,
				DefaultConfig: config.CircuitBreakerConfigSpec{
					MaxFailures:  5,
					Timeout:      60 * time.Second,
					MaxRequests:  3,
					FailureRatio: 0.6,
					MinRequests:  10,
				},
				Breakers: make(map[string]config.CircuitBreakerConfigSpec),
			},
		}
	}

	manager := &Manager{
		config:          cfg,
		retryPolicies:   make(map[string]*RetryPolicy),
		circuitBreakers: make(map[string]*CircuitBreaker),
		logger:          logger,
	}

	// Initialize retry policies
	manager.initializeRetryPolicies()

	// Initialize circuit breakers
	manager.initializeCircuitBreakers()

	return manager
}

// WithObservability adds observability features to the manager
func (m *Manager) WithObservability(structuredLogger *observability.StructuredLogger, metrics *observability.Metrics) *Manager {
	m.structuredLogger = structuredLogger
	m.metrics = metrics

	// Update all circuit breakers with observability
	m.mu.Lock()
	for _, cb := range m.circuitBreakers {
		cb.WithObservability(structuredLogger, metrics)
	}
	m.mu.Unlock()

	return m
}

// GetRetryer returns a retryer for the specified operation type
func (m *Manager) GetRetryer(operationType string) *Retryer {
	if !m.config.Retry.Enabled {
		return NewRetryer(NoRetryPolicy(), m.logger)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Look for operation-specific policy first
	if policy, exists := m.retryPolicies[operationType]; exists {
		retryer := NewRetryer(policy, m.logger).
			WithOperationName(operationType)

		if m.structuredLogger != nil && m.metrics != nil {
			retryer = retryer.WithObservability(m.structuredLogger, m.metrics)
		}

		return retryer
	}

	// Fall back to default policy
	if policy, exists := m.retryPolicies["default"]; exists {
		retryer := NewRetryer(policy, m.logger).
			WithOperationName(operationType)

		if m.structuredLogger != nil && m.metrics != nil {
			retryer = retryer.WithObservability(m.structuredLogger, m.metrics)
		}

		return retryer
	}

	// Ultimate fallback
	retryer := NewRetryer(DefaultRetryPolicy(), m.logger).
		WithOperationName(operationType)

	if m.structuredLogger != nil && m.metrics != nil {
		retryer = retryer.WithObservability(m.structuredLogger, m.metrics)
	}

	return retryer
}

// GetCircuitBreaker returns a circuit breaker for the specified service
func (m *Manager) GetCircuitBreaker(serviceName string) *CircuitBreaker {
	if !m.config.CircuitBreaker.Enabled {
		// Return a disabled circuit breaker (always closed)
		return NewCircuitBreaker(
			&CircuitBreakerConfig{MaxFailures: 999999, Timeout: 1 * time.Hour},
			serviceName,
			m.logger,
		)
	}

	m.mu.RLock()
	if cb, exists := m.circuitBreakers[serviceName]; exists {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()

	// Create circuit breaker if it doesn't exist
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := m.circuitBreakers[serviceName]; exists {
		return cb
	}

	// Look for service-specific config
	var cbConfig *CircuitBreakerConfig
	if spec, exists := m.config.CircuitBreaker.Breakers[serviceName]; exists {
		cbConfig = &CircuitBreakerConfig{
			MaxFailures:  spec.MaxFailures,
			Timeout:      spec.Timeout,
			MaxRequests:  spec.MaxRequests,
			FailureRatio: spec.FailureRatio,
			MinRequests:  spec.MinRequests,
		}
	} else {
		// Use default config
		spec := m.config.CircuitBreaker.DefaultConfig
		cbConfig = &CircuitBreakerConfig{
			MaxFailures:  spec.MaxFailures,
			Timeout:      spec.Timeout,
			MaxRequests:  spec.MaxRequests,
			FailureRatio: spec.FailureRatio,
			MinRequests:  spec.MinRequests,
		}
	}

	cb := NewCircuitBreaker(cbConfig, serviceName, m.logger)
	if m.structuredLogger != nil && m.metrics != nil {
		cb = cb.WithObservability(m.structuredLogger, m.metrics)
	}

	m.circuitBreakers[serviceName] = cb
	return cb
}

// GetCombinedDecorator returns a function decorator that combines both retry and circuit breaker logic
func (m *Manager) GetCombinedDecorator(serviceName, operationType string) func(fn func(ctx context.Context) error) func(context.Context) error {
	retryer := m.GetRetryer(operationType)
	circuitBreaker := m.GetCircuitBreaker(serviceName)

	return func(fn func(ctx context.Context) error) func(context.Context) error {
		// Create a wrapped function that works with our retry decorator
		wrappedFn := func(ctx context.Context) (struct{}, error) {
			err := fn(ctx)
			return struct{}{}, err
		}

		decoratedFn := RetryDecorator(retryer, wrappedFn)

		return func(ctx context.Context) error {
			return circuitBreaker.Execute(ctx, func(ctx context.Context) error {
				_, err := decoratedFn(ctx)
				return err
			})
		}
	}
}

// initializeRetryPolicies sets up retry policies from configuration
func (m *Manager) initializeRetryPolicies() {
	// Initialize default policy
	defaultConfig := m.config.Retry.DefaultPolicy
	m.retryPolicies["default"] = &RetryPolicy{
		MaxAttempts:     defaultConfig.MaxAttempts,
		BaseDelay:       defaultConfig.BaseDelay,
		MaxDelay:        defaultConfig.MaxDelay,
		BackoffFactor:   defaultConfig.BackoffFactor,
		JitterFactor:    defaultConfig.JitterFactor,
		RetryableErrors: m.convertRetryableErrors(defaultConfig.RetryableErrors),
	}

	// Initialize operation-specific policies
	for name, policyConfig := range m.config.Retry.Policies {
		m.retryPolicies[name] = &RetryPolicy{
			MaxAttempts:     policyConfig.MaxAttempts,
			BaseDelay:       policyConfig.BaseDelay,
			MaxDelay:        policyConfig.MaxDelay,
			BackoffFactor:   policyConfig.BackoffFactor,
			JitterFactor:    policyConfig.JitterFactor,
			RetryableErrors: m.convertRetryableErrors(policyConfig.RetryableErrors),
		}
	}
}

// initializeCircuitBreakers sets up circuit breakers from configuration
func (m *Manager) initializeCircuitBreakers() {
	// Circuit breakers are created lazily in GetCircuitBreaker()
	// This allows for dynamic service discovery
}

// convertRetryableErrors converts string error types to ErrorType enums
func (m *Manager) convertRetryableErrors(errorStrings []string) []ErrorType {
	var errorTypes []ErrorType
	for _, errorStr := range errorStrings {
		switch errorStr {
		case "network":
			errorTypes = append(errorTypes, ErrorTypeNetwork)
		case "rate_limit":
			errorTypes = append(errorTypes, ErrorTypeRateLimit)
		case "timeout":
			errorTypes = append(errorTypes, ErrorTypeTimeout)
		case "api":
			errorTypes = append(errorTypes, ErrorTypeAPI)
		case "temporary":
			errorTypes = append(errorTypes, ErrorTypeTemporary)
		case "authentication":
			errorTypes = append(errorTypes, ErrorTypeAuthentication)
		case "permanent":
			errorTypes = append(errorTypes, ErrorTypePermanent)
		}
	}
	return errorTypes
}

// GetStats returns statistics for all managed circuit breakers
func (m *Manager) GetStats() map[string]CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for name, cb := range m.circuitBreakers {
		stats[name] = cb.Stats()
	}
	return stats
}

// IsEnabled returns whether error handling features are enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Retry.Enabled || m.config.CircuitBreaker.Enabled
}
