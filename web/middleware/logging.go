package middleware

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/observability"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

// Hijack implements http.Hijacker, required for WebSocket upgrades.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// RequestLogger returns middleware that logs HTTP requests using slog.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"size", rw.size,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
			)
		})
	}
}

// DatabaseQueryLogger returns middleware that adds database query logging context.
func DatabaseQueryLogger(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add correlation ID to context for database query tracing
			correlationID := observability.NewCorrelationID()
			ctx := observability.WithCorrelationID(r.Context(), correlationID)
			r = r.WithContext(ctx)

			// Add query logging context
			ctx = context.WithValue(ctx, "request_method", r.Method)
			ctx = context.WithValue(ctx, "request_path", r.URL.Path)
			r = r.WithContext(ctx)

			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			// Log database query metrics if available in context
			if queryCount, ok := ctx.Value("db_query_count").(int); ok {
				if metrics != nil {
					labels := map[string]string{
						"method": r.Method,
						"path":   r.URL.Path,
						"status": string(rune(rw.status)),
					}
					metrics.Set("http_request_db_queries", float64(queryCount), labels)
				}

				logger.Debug("database queries for request",
					"method", r.Method,
					"path", r.URL.Path,
					"query_count", queryCount,
					"correlation_id", correlationID,
				)
			}
		})
	}
}

// DatabaseOperationContext holds database operation tracking information.
type DatabaseOperationContext struct {
	QueryCount   int
	TotalTimeMs  int64
	SlowQueries  int
	ErrorQueries int
}
