package store

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	webconfig "github.com/gaskaj/OpenAgentFramework/web/config"
)

func TestPostgresStoreConfiguration(t *testing.T) {
	t.Run("default_configuration", func(t *testing.T) {
		cfg := webconfig.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Name:     "test_db",
			User:     "test_user",
			Password: "test_pass",
			SSLMode:  "disable",
		}

		// Test DSN generation
		dsn := cfg.DSN()
		expected := "postgres://test_user:test_pass@localhost:5432/test_db?sslmode=disable"
		assert.Equal(t, expected, dsn)
	})

	t.Run("pool_configuration", func(t *testing.T) {
		cfg := webconfig.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Name:     "test_db",
			User:     "test_user",
			Password: "test_pass",
			SSLMode:  "disable",
			Pool: webconfig.PoolConfig{
				MaxConnections:      25,
				MinConnections:      5,
				MaxIdleTime:         30 * time.Minute,
				HealthCheckPeriod:   1 * time.Minute,
				MaxConnLifetime:     1 * time.Hour,
				MaxConnIdleTime:     15 * time.Minute,
			},
		}

		assert.Equal(t, 25, cfg.Pool.MaxConnections)
		assert.Equal(t, 5, cfg.Pool.MinConnections)
		assert.Equal(t, 30*time.Minute, cfg.Pool.MaxIdleTime)
	})

	t.Run("performance_configuration", func(t *testing.T) {
		cfg := webconfig.DatabaseConfig{
			Performance: webconfig.PerfConfig{
				SlowQueryThreshold: 100 * time.Millisecond,
				QueryTimeout:       30 * time.Second,
				EnableQueryLog:     true,
				EnableMetrics:      true,
			},
		}

		assert.Equal(t, 100*time.Millisecond, cfg.Performance.SlowQueryThreshold)
		assert.Equal(t, 30*time.Second, cfg.Performance.QueryTimeout)
		assert.True(t, cfg.Performance.EnableQueryLog)
		assert.True(t, cfg.Performance.EnableMetrics)
	})
}

func TestQueryMonitorInitialization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	metrics := observability.NewMetrics(nil)

	perfConfig := &webconfig.PerfConfig{
		SlowQueryThreshold: 50 * time.Millisecond,
		QueryTimeout:       5 * time.Second,
		EnableQueryLog:     true,
		EnableMetrics:      true,
	}

	// This test creates a mock query monitor without a real connection
	// In integration tests, you'd use a real database connection
	monitor := &QueryMonitor{
		config:  perfConfig,
		logger:  logger,
		metrics: metrics,
	}

	assert.NotNil(t, monitor)
	assert.Equal(t, 50*time.Millisecond, monitor.config.SlowQueryThreshold)
	assert.True(t, monitor.config.EnableQueryLog)
	assert.True(t, monitor.config.EnableMetrics)
}

func TestDatabaseHealthCheck(t *testing.T) {
	t.Run("health_check_structure", func(t *testing.T) {
		health := &DatabaseHealth{
			Status:       "healthy",
			ResponseTime: 10 * time.Millisecond,
			QueryTime:    5 * time.Millisecond,
			Pool: PoolHealth{
				TotalConns:    10,
				IdleConns:     5,
				AcquiredConns: 3,
				MaxConns:      25,
			},
		}

		assert.Equal(t, "healthy", health.Status)
		assert.Equal(t, 10*time.Millisecond, health.ResponseTime)
		assert.Equal(t, 5*time.Millisecond, health.QueryTime)
		assert.Equal(t, 10, health.Pool.TotalConns)
		assert.Equal(t, 5, health.Pool.IdleConns)
		assert.Equal(t, 3, health.Pool.AcquiredConns)
		assert.Equal(t, 25, health.Pool.MaxConns)
	})

	t.Run("unhealthy_status", func(t *testing.T) {
		health := &DatabaseHealth{
			Status: "unhealthy",
			Error:  "connection refused",
		}

		assert.Equal(t, "unhealthy", health.Status)
		assert.Equal(t, "connection refused", health.Error)
	})
}

func TestConfigurationDefaults(t *testing.T) {
	t.Run("apply_defaults", func(t *testing.T) {
		cfg := webconfig.DatabaseConfig{}

		// Apply defaults manually (normally done by Load function)
		if cfg.Pool.MaxConnections == 0 {
			cfg.Pool.MaxConnections = 25
		}
		if cfg.Pool.MinConnections == 0 {
			cfg.Pool.MinConnections = 5
		}
		if cfg.Pool.MaxIdleTime == 0 {
			cfg.Pool.MaxIdleTime = 30 * time.Minute
		}
		if cfg.Pool.HealthCheckPeriod == 0 {
			cfg.Pool.HealthCheckPeriod = 1 * time.Minute
		}
		if cfg.Performance.SlowQueryThreshold == 0 {
			cfg.Performance.SlowQueryThreshold = 100 * time.Millisecond
		}
		if cfg.Performance.QueryTimeout == 0 {
			cfg.Performance.QueryTimeout = 30 * time.Second
		}

		assert.Equal(t, 25, cfg.Pool.MaxConnections)
		assert.Equal(t, 5, cfg.Pool.MinConnections)
		assert.Equal(t, 30*time.Minute, cfg.Pool.MaxIdleTime)
		assert.Equal(t, 1*time.Minute, cfg.Pool.HealthCheckPeriod)
		assert.Equal(t, 100*time.Millisecond, cfg.Performance.SlowQueryThreshold)
		assert.Equal(t, 30*time.Second, cfg.Performance.QueryTimeout)
	})
}

func TestMetricsIntegration(t *testing.T) {
	t.Run("database_metrics", func(t *testing.T) {
		_ = slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
		metrics := observability.NewMetrics(nil)

		// Test database operation metrics
		ctx := context.Background()
		duration := 50 * time.Millisecond
		metrics.RecordDatabaseOperation(ctx, "select", duration, true)

		counters := metrics.GetCounters()
		assert.Contains(t, counters, "database_operations_total:operation=select")
		assert.Contains(t, counters, "database_operations_success_total:operation=select")
		assert.Equal(t, int64(1), counters["database_operations_total:operation=select"])

		// Test connection pool metrics
		metrics.RecordDatabaseConnectionPool(ctx, 10, 5, 3, 25)

		gauges := metrics.GetGauges()
		assert.Contains(t, gauges, "database_pool_total_connections:pool_type=postgresql")
		assert.Equal(t, float64(10), gauges["database_pool_total_connections:pool_type=postgresql"])
		assert.Equal(t, float64(5), gauges["database_pool_idle_connections:pool_type=postgresql"])
		assert.Equal(t, float64(3), gauges["database_pool_acquired_connections:pool_type=postgresql"])

		// Test slow query metrics
		slowDuration := 200 * time.Millisecond
		metrics.RecordSlowQuery(ctx, "select", slowDuration)

		updatedCounters := metrics.GetCounters()
		assert.Contains(t, updatedCounters, "database_slow_queries_total:operation=select")
	})
}

func TestCorrelationIDIntegration(t *testing.T) {
	t.Run("correlation_id_generation", func(t *testing.T) {
		id1 := observability.NewCorrelationID()
		id2 := observability.NewCorrelationID()

		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2)
	})

	t.Run("correlation_id_context", func(t *testing.T) {
		ctx := context.Background()
		id := "test-correlation-id"

		ctx = observability.WithCorrelationID(ctx, id)
		retrievedID := observability.GetCorrelationID(ctx)

		assert.Equal(t, id, retrievedID)
	})
}

func TestDatabaseStoreInstantiation(t *testing.T) {
	t.Run("store_creation_without_database", func(t *testing.T) {
		// Test that store structure can be created without actual database connection
		cfg := webconfig.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Name:     "test_db",
			User:     "test_user",
			Password: "test_pass",
			SSLMode:  "disable",
			Pool: webconfig.PoolConfig{
				MaxConnections: 10,
				MinConnections: 2,
			},
			Performance: webconfig.PerfConfig{
				SlowQueryThreshold: 100 * time.Millisecond,
				EnableQueryLog:     true,
				EnableMetrics:      true,
			},
		}

		logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
		metrics := observability.NewMetrics(nil)

		// This would fail with real database connection, but tests the configuration
		ctx := context.Background()
		_, err := NewPostgresStore(ctx, cfg, logger, metrics)

		// We expect this to fail since there's no real database
		assert.Error(t, err)
		// Error could be about connection, authentication, or database not found
		assert.True(t, err != nil, "should have an error without real database")
	})
}