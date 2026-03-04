package errors

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func newTestConfig() *config.ErrorHandlingConfig {
	return &config.ErrorHandlingConfig{
		Retry: config.RetryConfig{
			Enabled: true,
			DefaultPolicy: config.RetryPolicyConfig{
				MaxAttempts:     3,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				BackoffFactor:   2.0,
				JitterFactor:    0.0,
				RetryableErrors: []string{"network", "timeout", "api", "temporary"},
			},
			Policies: map[string]config.RetryPolicyConfig{
				"github_api": {
					MaxAttempts:     5,
					BaseDelay:       20 * time.Millisecond,
					MaxDelay:        200 * time.Millisecond,
					BackoffFactor:   2.0,
					JitterFactor:    0.0,
					RetryableErrors: []string{"network", "timeout", "rate_limit"},
				},
			},
		},
		CircuitBreaker: config.CircuitBreakerGroupConfig{
			Enabled: true,
			DefaultConfig: config.CircuitBreakerConfigSpec{
				MaxFailures:  5,
				Timeout:      1 * time.Second,
				MaxRequests:  3,
				FailureRatio: 0.6,
				MinRequests:  10,
			},
			Breakers: map[string]config.CircuitBreakerConfigSpec{
				"github": {
					MaxFailures:  3,
					Timeout:      2 * time.Second,
					MaxRequests:  2,
					FailureRatio: 0.5,
					MinRequests:  5,
				},
			},
		},
	}
}

func TestNewManager_NilConfig(t *testing.T) {
	logger := newTestLogger()
	m := NewManager(nil, logger)

	require.NotNil(t, m)
	assert.NotNil(t, m.config)
	assert.True(t, m.config.Retry.Enabled)
	assert.True(t, m.config.CircuitBreaker.Enabled)
}

func TestNewManager_WithConfig(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	require.NotNil(t, m)
	assert.Equal(t, cfg, m.config)
}

func TestManager_WithObservability(t *testing.T) {
	logger := newTestLogger()
	m := NewManager(nil, logger)

	// Just verify it doesn't panic and returns self
	result := m.WithObservability(nil, nil)
	assert.Equal(t, m, result)
}

func TestManager_GetRetryer(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	t.Run("returns operation-specific policy", func(t *testing.T) {
		retryer := m.GetRetryer("github_api")
		require.NotNil(t, retryer)
		assert.Equal(t, 5, retryer.policy.MaxAttempts)
		assert.Equal(t, "github_api", retryer.operationName)
	})

	t.Run("falls back to default policy", func(t *testing.T) {
		retryer := m.GetRetryer("unknown_operation")
		require.NotNil(t, retryer)
		assert.Equal(t, 3, retryer.policy.MaxAttempts)
		assert.Equal(t, "unknown_operation", retryer.operationName)
	})

	t.Run("retry disabled returns no-retry policy", func(t *testing.T) {
		disabledCfg := newTestConfig()
		disabledCfg.Retry.Enabled = false
		dm := NewManager(disabledCfg, logger)

		retryer := dm.GetRetryer("any")
		require.NotNil(t, retryer)
		assert.Equal(t, 1, retryer.policy.MaxAttempts)
	})
}

func TestManager_GetRetryer_UltimateFallback(t *testing.T) {
	logger := newTestLogger()
	// Create config with empty policies and no default
	cfg := &config.ErrorHandlingConfig{
		Retry: config.RetryConfig{
			Enabled:       true,
			DefaultPolicy: config.RetryPolicyConfig{},
			Policies:      map[string]config.RetryPolicyConfig{},
		},
		CircuitBreaker: config.CircuitBreakerGroupConfig{
			Enabled:       true,
			DefaultConfig: config.CircuitBreakerConfigSpec{},
			Breakers:      map[string]config.CircuitBreakerConfigSpec{},
		},
	}
	m := NewManager(cfg, logger)
	// Clear the default policy that was initialized
	delete(m.retryPolicies, "default")

	retryer := m.GetRetryer("something")
	require.NotNil(t, retryer)
	// Should use DefaultRetryPolicy() as ultimate fallback
	assert.Equal(t, 3, retryer.policy.MaxAttempts)
}

func TestManager_GetCircuitBreaker(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	t.Run("returns service-specific circuit breaker", func(t *testing.T) {
		cb := m.GetCircuitBreaker("github")
		require.NotNil(t, cb)
		assert.Equal(t, "github", cb.name)
		assert.Equal(t, int64(3), cb.config.MaxFailures)
	})

	t.Run("returns default circuit breaker for unknown service", func(t *testing.T) {
		cb := m.GetCircuitBreaker("unknown_service")
		require.NotNil(t, cb)
		assert.Equal(t, "unknown_service", cb.name)
		assert.Equal(t, int64(5), cb.config.MaxFailures)
	})

	t.Run("returns same instance for same service", func(t *testing.T) {
		cb1 := m.GetCircuitBreaker("github")
		cb2 := m.GetCircuitBreaker("github")
		assert.Equal(t, cb1, cb2) // Same pointer
	})

	t.Run("circuit breaker disabled returns permissive breaker", func(t *testing.T) {
		disabledCfg := newTestConfig()
		disabledCfg.CircuitBreaker.Enabled = false
		dm := NewManager(disabledCfg, logger)

		cb := dm.GetCircuitBreaker("any")
		require.NotNil(t, cb)
		assert.Equal(t, int64(999999), cb.config.MaxFailures)
	})
}

func TestManager_GetCombinedDecorator(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	callCount := 0
	fn := func(ctx context.Context) error {
		callCount++
		if callCount < 2 {
			return NewNetworkError("temp failure", errors.New("network error"))
		}
		return nil
	}

	decorator := m.GetCombinedDecorator("github", "github_api")
	decoratedFn := decorator(fn)

	err := decoratedFn(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestManager_GetStats(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	t.Run("empty stats initially", func(t *testing.T) {
		stats := m.GetStats()
		assert.Empty(t, stats)
	})

	t.Run("stats after creating circuit breakers", func(t *testing.T) {
		m.GetCircuitBreaker("service_a")
		m.GetCircuitBreaker("service_b")

		stats := m.GetStats()
		assert.Len(t, stats, 2)
		assert.Contains(t, stats, "service_a")
		assert.Contains(t, stats, "service_b")
		assert.Equal(t, StateClosed, stats["service_a"].State)
	})
}

func TestManager_IsEnabled(t *testing.T) {
	logger := newTestLogger()

	t.Run("both enabled", func(t *testing.T) {
		cfg := newTestConfig()
		m := NewManager(cfg, logger)
		assert.True(t, m.IsEnabled())
	})

	t.Run("only retry enabled", func(t *testing.T) {
		cfg := newTestConfig()
		cfg.CircuitBreaker.Enabled = false
		m := NewManager(cfg, logger)
		assert.True(t, m.IsEnabled())
	})

	t.Run("only circuit breaker enabled", func(t *testing.T) {
		cfg := newTestConfig()
		cfg.Retry.Enabled = false
		m := NewManager(cfg, logger)
		assert.True(t, m.IsEnabled())
	})

	t.Run("both disabled", func(t *testing.T) {
		cfg := newTestConfig()
		cfg.Retry.Enabled = false
		cfg.CircuitBreaker.Enabled = false
		m := NewManager(cfg, logger)
		assert.False(t, m.IsEnabled())
	})
}

func TestManager_ConvertRetryableErrors(t *testing.T) {
	logger := newTestLogger()
	m := NewManager(nil, logger)

	t.Run("all known error types", func(t *testing.T) {
		input := []string{"network", "rate_limit", "timeout", "api", "temporary", "authentication", "permanent"}
		result := m.convertRetryableErrors(input)
		expected := []ErrorType{
			ErrorTypeNetwork,
			ErrorTypeRateLimit,
			ErrorTypeTimeout,
			ErrorTypeAPI,
			ErrorTypeTemporary,
			ErrorTypeAuthentication,
			ErrorTypePermanent,
		}
		assert.Equal(t, expected, result)
	})

	t.Run("unknown error types are skipped", func(t *testing.T) {
		input := []string{"network", "unknown_type", "api"}
		result := m.convertRetryableErrors(input)
		assert.Len(t, result, 2)
		assert.Equal(t, ErrorTypeNetwork, result[0])
		assert.Equal(t, ErrorTypeAPI, result[1])
	})

	t.Run("empty input", func(t *testing.T) {
		result := m.convertRetryableErrors([]string{})
		assert.Nil(t, result)
	})
}

func TestManager_InitializeRetryPolicies(t *testing.T) {
	logger := newTestLogger()
	cfg := newTestConfig()
	m := NewManager(cfg, logger)

	// Should have default + github_api policies
	assert.Contains(t, m.retryPolicies, "default")
	assert.Contains(t, m.retryPolicies, "github_api")

	defaultPolicy := m.retryPolicies["default"]
	assert.Equal(t, 3, defaultPolicy.MaxAttempts)

	githubPolicy := m.retryPolicies["github_api"]
	assert.Equal(t, 5, githubPolicy.MaxAttempts)
}
