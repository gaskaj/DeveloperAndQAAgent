package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	webconfig "github.com/gaskaj/OpenAgentFramework/web/config"
)

// PostgresStore wraps a pgxpool.Pool and provides access to all sub-stores.
type PostgresStore struct {
	Pool         *pgxpool.Pool
	QueryMonitor *QueryMonitor
	Logger       *slog.Logger
	Metrics      *observability.Metrics
	Orgs         *PgOrgStore
	Users        *PgUserStore
	Agents       *PgAgentStore
	Events       *PgEventStore
	APIKeys      *PgAPIKeyStore
	Invitations  *PgInvitationStore
	AuditLogs    *PgAuditStore
}

// NewPostgresStore creates a new PostgresStore with a connection pool.
func NewPostgresStore(ctx context.Context, cfg webconfig.DatabaseConfig, logger *slog.Logger, metrics *observability.Metrics) (*PostgresStore, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	// Configure connection pool settings
	if cfg.Pool.MaxConnections > 0 {
		poolCfg.MaxConns = int32(cfg.Pool.MaxConnections)
	} else {
		poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	}
	
	if cfg.Pool.MinConnections > 0 {
		poolCfg.MinConns = int32(cfg.Pool.MinConnections)
	} else {
		poolCfg.MinConns = int32(cfg.MaxIdleConns)
	}
	
	if cfg.Pool.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.Pool.MaxConnLifetime
	} else {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}
	
	if cfg.Pool.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.Pool.MaxConnIdleTime
	}
	
	if cfg.Pool.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.Pool.HealthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Initialize query monitor
	queryMonitor := NewQueryMonitor(pool, &cfg.Performance, logger, metrics)

	s := &PostgresStore{
		Pool:         pool,
		QueryMonitor: queryMonitor,
		Logger:       logger,
		Metrics:      metrics,
	}
	s.Orgs = &PgOrgStore{pool: pool, monitor: queryMonitor}
	s.Users = &PgUserStore{pool: pool, monitor: queryMonitor}
	s.Agents = &PgAgentStore{pool: pool, monitor: queryMonitor}
	s.Events = &PgEventStore{pool: pool, monitor: queryMonitor}
	s.APIKeys = &PgAPIKeyStore{pool: pool, monitor: queryMonitor}
	s.Invitations = &PgInvitationStore{pool: pool, monitor: queryMonitor}
	s.AuditLogs = &PgAuditStore{pool: pool, monitor: queryMonitor}

	return s, nil
}

// Close closes the connection pool.
func (s *PostgresStore) Close() {
	s.Pool.Close()
}

// Ping checks the database connection.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.Pool.Ping(ctx)
}

// HealthCheck returns detailed database health information.
func (s *PostgresStore) HealthCheck(ctx context.Context) (*DatabaseHealth, error) {
	start := time.Now()
	
	// Test basic connectivity
	if err := s.Pool.Ping(ctx); err != nil {
		return &DatabaseHealth{
			Status:       "unhealthy",
			Error:        err.Error(),
			ResponseTime: time.Since(start),
		}, err
	}

	// Get pool statistics
	stats := s.Pool.Stat()
	
	// Test a simple query
	var result int
	queryStart := time.Now()
	err := s.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	queryTime := time.Since(queryStart)
	
	health := &DatabaseHealth{
		Status:       "healthy",
		ResponseTime: time.Since(start),
		QueryTime:    queryTime,
		Pool: PoolHealth{
			TotalConns:    int(stats.TotalConns()),
			IdleConns:     int(stats.IdleConns()),
			AcquiredConns: int(stats.AcquiredConns()),
			MaxConns:      int(stats.MaxConns()),
		},
	}

	if err != nil {
		health.Status = "degraded"
		health.Error = err.Error()
	}

	return health, nil
}

// DatabaseHealth represents the health status of the database.
type DatabaseHealth struct {
	Status       string        `json:"status"`
	Error        string        `json:"error,omitempty"`
	ResponseTime time.Duration `json:"response_time"`
	QueryTime    time.Duration `json:"query_time"`
	Pool         PoolHealth    `json:"pool"`
}

// PoolHealth represents connection pool health metrics.
type PoolHealth struct {
	TotalConns    int `json:"total_conns"`
	IdleConns     int `json:"idle_conns"`
	AcquiredConns int `json:"acquired_conns"`
	MaxConns      int `json:"max_conns"`
}
