package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	webconfig "github.com/gaskaj/OpenAgentFramework/web/config"
)

// QueryMonitor wraps database operations with performance monitoring
type QueryMonitor struct {
	pool     *pgxpool.Pool
	config   *webconfig.PerfConfig
	logger   *slog.Logger
	metrics  *observability.Metrics
}

// NewQueryMonitor creates a new query monitor
func NewQueryMonitor(pool *pgxpool.Pool, config *webconfig.PerfConfig, logger *slog.Logger, metrics *observability.Metrics) *QueryMonitor {
	return &QueryMonitor{
		pool:    pool,
		config:  config,
		logger:  logger,
		metrics: metrics,
	}
}

// QueryContext wraps pgx QueryContext with monitoring
func (qm *QueryMonitor) QueryContext(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	
	// Add query timeout if configured
	if qm.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, qm.config.QueryTimeout)
		defer cancel()
	}

	rows, err := qm.pool.Query(ctx, sql, args...)
	duration := time.Since(start)

	qm.recordQueryMetrics(ctx, sql, duration, err == nil, "query")
	qm.logSlowQuery(ctx, sql, args, duration, err)

	return rows, err
}

// QueryRowContext wraps pgx QueryRowContext with monitoring
func (qm *QueryMonitor) QueryRowContext(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	start := time.Now()
	
	// Add query timeout if configured
	if qm.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, qm.config.QueryTimeout)
		defer cancel()
	}

	row := qm.pool.QueryRow(ctx, sql, args...)
	duration := time.Since(start)

	qm.recordQueryMetrics(ctx, sql, duration, true, "query_row")
	qm.logSlowQuery(ctx, sql, args, duration, nil)

	return row
}

// ExecContext wraps pgx ExecContext with monitoring
func (qm *QueryMonitor) ExecContext(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	start := time.Now()
	
	// Add query timeout if configured
	if qm.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, qm.config.QueryTimeout)
		defer cancel()
	}

	tag, err := qm.pool.Exec(ctx, sql, args...)
	duration := time.Since(start)

	qm.recordQueryMetrics(ctx, sql, duration, err == nil, "exec")
	qm.logSlowQuery(ctx, sql, args, duration, err)

	return tag, err
}

// BeginTx wraps pgx BeginTx with monitoring
func (qm *QueryMonitor) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	start := time.Now()
	
	tx, err := qm.pool.BeginTx(ctx, txOptions)
	duration := time.Since(start)

	qm.recordQueryMetrics(ctx, "BEGIN TRANSACTION", duration, err == nil, "begin_tx")

	if err == nil {
		// Wrap the transaction with monitoring
		return &monitoredTx{
			Tx:      tx,
			monitor: qm,
		}, nil
	}

	return tx, err
}

// GetPoolStats returns connection pool statistics
func (qm *QueryMonitor) GetPoolStats() *pgxpool.Stat {
	return qm.pool.Stat()
}

// recordQueryMetrics records performance metrics for database operations
func (qm *QueryMonitor) recordQueryMetrics(ctx context.Context, sql string, duration time.Duration, success bool, operation string) {
	if !qm.config.EnableMetrics || qm.metrics == nil {
		return
	}

	labels := map[string]string{
		"operation": operation,
		"success":   fmt.Sprintf("%t", success),
	}

	// Record query execution metrics
	qm.metrics.Inc("db_queries_total", labels)
	qm.metrics.Observe("db_query_duration_ms", float64(duration.Milliseconds()), labels)

	// Record connection pool metrics
	stats := qm.pool.Stat()
	qm.metrics.Set("db_pool_total_conns", float64(stats.TotalConns()), nil)
	qm.metrics.Set("db_pool_idle_conns", float64(stats.IdleConns()), nil)
	qm.metrics.Set("db_pool_acquired_conns", float64(stats.AcquiredConns()), nil)

	// Record slow queries
	if duration >= qm.config.SlowQueryThreshold {
		qm.metrics.Inc("db_slow_queries_total", labels)
	}

	if !success {
		qm.metrics.Inc("db_query_errors_total", labels)
	}
}

// logSlowQuery logs queries that exceed the slow query threshold
func (qm *QueryMonitor) logSlowQuery(ctx context.Context, sql string, args []interface{}, duration time.Duration, err error) {
	if !qm.config.EnableQueryLog || duration < qm.config.SlowQueryThreshold {
		return
	}

	correlationID := observability.GetCorrelationID(ctx)
	
	logArgs := []interface{}{
		"sql", sql,
		"duration_ms", duration.Milliseconds(),
		"correlation_id", correlationID,
	}

	if err != nil {
		logArgs = append(logArgs, "error", err.Error())
		qm.logger.Error("slow database query with error", logArgs...)
	} else {
		qm.logger.Warn("slow database query", logArgs...)
	}

	// Log query arguments if present (be careful with sensitive data)
	if len(args) > 0 && qm.logger.Enabled(ctx, slog.LevelDebug) {
		qm.logger.Debug("slow query arguments", "args_count", len(args))
	}
}

// monitoredTx wraps a pgx transaction with monitoring
type monitoredTx struct {
	pgx.Tx
	monitor *QueryMonitor
}

func (tx *monitoredTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	rows, err := tx.Tx.Query(ctx, sql, args...)
	duration := time.Since(start)

	tx.monitor.recordQueryMetrics(ctx, sql, duration, err == nil, "tx_query")
	tx.monitor.logSlowQuery(ctx, sql, args, duration, err)

	return rows, err
}

func (tx *monitoredTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	start := time.Now()
	row := tx.Tx.QueryRow(ctx, sql, args...)
	duration := time.Since(start)

	tx.monitor.recordQueryMetrics(ctx, sql, duration, true, "tx_query_row")
	tx.monitor.logSlowQuery(ctx, sql, args, duration, nil)

	return row
}

func (tx *monitoredTx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := tx.Tx.Exec(ctx, sql, args...)
	duration := time.Since(start)

	tx.monitor.recordQueryMetrics(ctx, sql, duration, err == nil, "tx_exec")
	tx.monitor.logSlowQuery(ctx, sql, args, duration, err)

	return tag, err
}

func (tx *monitoredTx) Commit(ctx context.Context) error {
	start := time.Now()
	err := tx.Tx.Commit(ctx)
	duration := time.Since(start)

	tx.monitor.recordQueryMetrics(ctx, "COMMIT", duration, err == nil, "tx_commit")

	return err
}

func (tx *monitoredTx) Rollback(ctx context.Context) error {
	start := time.Now()
	err := tx.Tx.Rollback(ctx)
	duration := time.Since(start)

	tx.monitor.recordQueryMetrics(ctx, "ROLLBACK", duration, err == nil, "tx_rollback")

	return err
}

// QueryExecutor interface for database operations with monitoring
type QueryExecutor interface {
	QueryContext(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRowContext(ctx context.Context, sql string, args ...interface{}) pgx.Row
	ExecContext(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	GetPoolStats() *pgxpool.Stat
}

// Ensure QueryMonitor implements QueryExecutor
var _ QueryExecutor = (*QueryMonitor)(nil)