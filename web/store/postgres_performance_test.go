package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	webconfig "github.com/gaskaj/OpenAgentFramework/web/config"
)

func TestConnectionPoolPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	cfg := webconfig.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "test_db",
		User:     "test_user",
		Password: "test_pass",
		SSLMode:  "disable",
		Pool: webconfig.PoolConfig{
			MaxConnections:      10,
			MinConnections:      2,
			MaxIdleTime:         30 * time.Second,
			HealthCheckPeriod:   10 * time.Second,
			MaxConnLifetime:     1 * time.Hour,
			MaxConnIdleTime:     15 * time.Minute,
		},
		Performance: webconfig.PerfConfig{
			SlowQueryThreshold: 50 * time.Millisecond,
			QueryTimeout:       5 * time.Second,
			EnableQueryLog:     true,
			EnableMetrics:      true,
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelWarn}))
	metrics := observability.NewMetrics(nil)

	// This test would require a real database connection in integration tests
	t.Skip("Requires database connection for integration testing")

	ctx := context.Background()
	store, err := NewPostgresStore(ctx, cfg, logger, metrics)
	require.NoError(t, err)
	defer store.Close()

	t.Run("concurrent_connections", func(t *testing.T) {
		concurrency := 20
		iterations := 100

		var wg sync.WaitGroup
		start := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					health, err := store.HealthCheck(ctx)
					assert.NoError(t, err)
					assert.Equal(t, "healthy", health.Status)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		totalOps := concurrency * iterations
		opsPerSecond := float64(totalOps) / duration.Seconds()

		t.Logf("Concurrent operations: %d workers, %d iterations each", concurrency, iterations)
		t.Logf("Total operations: %d", totalOps)
		t.Logf("Duration: %v", duration)
		t.Logf("Operations per second: %.2f", opsPerSecond)

		// Assert reasonable performance (adjust thresholds based on your requirements)
		assert.Greater(t, opsPerSecond, 100.0, "Operations per second should be reasonable")
	})

	t.Run("connection_pool_stress", func(t *testing.T) {
		// Test connection pool under stress
		concurrency := 50 // More than max connections
		var wg sync.WaitGroup
		errors := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				health, err := store.HealthCheck(ctx)
				if err != nil {
					errors <- err
					return
				}
				if health.Status != "healthy" {
					errors <- fmt.Errorf("unhealthy status: %s", health.Status)
				}
			}()
		}

		wg.Wait()
		close(errors)

		var errorCount int
		for err := range errors {
			errorCount++
			t.Logf("Error during stress test: %v", err)
		}

		// Allow some errors under extreme stress, but not too many
		errorRate := float64(errorCount) / float64(concurrency)
		assert.Less(t, errorRate, 0.1, "Error rate should be less than 10%")
	})
}

func TestQueryMonitoringPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Mock configuration for testing
	_ = &webconfig.PerfConfig{
		SlowQueryThreshold: 10 * time.Millisecond,
		QueryTimeout:       1 * time.Second,
		EnableQueryLog:     true,
		EnableMetrics:      true,
	}

	_ = slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelWarn}))
	metrics := observability.NewMetrics(nil)

	t.Run("metrics_collection_overhead", func(t *testing.T) {
		// Test that metrics collection doesn't add significant overhead
		// This is a mock test - in real scenarios you'd benchmark with actual queries

		iterations := 1000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			// Simulate recording metrics
			labels := map[string]string{
				"operation": "test_query",
				"success":   "true",
			}
			metrics.Inc("db_queries_total", labels)
			metrics.Observe("db_query_duration_ms", float64(5), labels)
		}

		duration := time.Since(start)
		avgTime := duration / time.Duration(iterations)

		t.Logf("Metrics collection: %d operations in %v", iterations, duration)
		t.Logf("Average time per operation: %v", avgTime)

		// Assert that metrics collection is fast
		assert.Less(t, avgTime.Microseconds(), int64(100), "Metrics collection should be fast")
	})
}

func TestDatabaseHealthMonitoring(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	cfg := webconfig.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "test_db",
		User:     "test_user", 
		Password: "test_pass",
		SSLMode:  "disable",
		Pool: webconfig.PoolConfig{
			MaxConnections:    5,
			MinConnections:    1,
			MaxIdleTime:       30 * time.Second,
			HealthCheckPeriod: 5 * time.Second,
		},
		Performance: webconfig.PerfConfig{
			SlowQueryThreshold: 100 * time.Millisecond,
			QueryTimeout:       5 * time.Second,
			EnableQueryLog:     true,
			EnableMetrics:      true,
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelWarn}))
	metrics := observability.NewMetrics(nil)

	// This test would require a real database connection
	t.Skip("Requires database connection for integration testing")

	ctx := context.Background()
	store, err := NewPostgresStore(ctx, cfg, logger, metrics)
	require.NoError(t, err)
	defer store.Close()

	t.Run("health_check_response_time", func(t *testing.T) {
		iterations := 50
		var totalDuration time.Duration

		for i := 0; i < iterations; i++ {
			health, err := store.HealthCheck(ctx)
			require.NoError(t, err)
			require.Equal(t, "healthy", health.Status)
			
			totalDuration += health.ResponseTime
		}

		avgResponseTime := totalDuration / time.Duration(iterations)
		t.Logf("Average health check response time: %v", avgResponseTime)

		// Health checks should be fast
		assert.Less(t, avgResponseTime.Milliseconds(), int64(100), "Health checks should complete quickly")
	})

	t.Run("pool_metrics_accuracy", func(t *testing.T) {
		health, err := store.HealthCheck(ctx)
		require.NoError(t, err)

		// Validate pool metrics make sense
		assert.GreaterOrEqual(t, health.Pool.TotalConns, health.Pool.AcquiredConns)
		assert.GreaterOrEqual(t, health.Pool.MaxConns, health.Pool.TotalConns)
		assert.GreaterOrEqual(t, health.Pool.TotalConns, health.Pool.IdleConns+health.Pool.AcquiredConns)

		t.Logf("Pool metrics: Total=%d, Idle=%d, Acquired=%d, Max=%d",
			health.Pool.TotalConns, health.Pool.IdleConns, 
			health.Pool.AcquiredConns, health.Pool.MaxConns)
	})
}

func BenchmarkQueryMonitoring(b *testing.B) {
	_ = slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := observability.NewMetrics(nil)

	b.Run("metrics_recording", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			labels := map[string]string{
				"operation": "benchmark_query",
				"success":   "true",
			}
			metrics.Inc("db_queries_total", labels)
			metrics.Observe("db_query_duration_ms", float64(i%100), labels)
		}
	})

	b.Run("correlation_id_generation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = observability.NewCorrelationID()
		}
	})
}

func TestConnectionPoolRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test connection pool recovery scenarios
	cfg := webconfig.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "test_db",
		User:     "test_user",
		Password: "test_pass", 
		SSLMode:  "disable",
		Pool: webconfig.PoolConfig{
			MaxConnections:    3,
			MinConnections:    1,
			MaxIdleTime:       10 * time.Second,
			HealthCheckPeriod: 2 * time.Second,
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelWarn}))
	metrics := observability.NewMetrics(nil)

	t.Skip("Requires database connection and failure simulation for integration testing")

	ctx := context.Background()
	store, err := NewPostgresStore(ctx, cfg, logger, metrics)
	require.NoError(t, err)
	defer store.Close()

	t.Run("connection_pool_exhaustion_recovery", func(t *testing.T) {
		// This would test recovery after connection pool exhaustion
		// In a real test, you'd simulate connection exhaustion and verify recovery
		health, err := store.HealthCheck(ctx)
		require.NoError(t, err)
		assert.Equal(t, "healthy", health.Status)
	})
}