package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
)

// DatabaseHealthChecker interface for database health checks.
type DatabaseHealthChecker interface {
	HealthCheck(ctx context.Context) (*DatabaseHealth, error)
}

// DatabaseHealth represents database health status.
type DatabaseHealth struct {
	Status       string        `json:"status"`
	Error        string        `json:"error,omitempty"`
	ResponseTime time.Duration `json:"response_time"`
	QueryTime    time.Duration `json:"query_time"`
	Pool         PoolHealth    `json:"pool"`
}

// PoolHealth represents connection pool health.
type PoolHealth struct {
	TotalConns    int `json:"total_conns"`
	IdleConns     int `json:"idle_conns"`
	AcquiredConns int `json:"acquired_conns"`
	MaxConns      int `json:"max_conns"`
}

// HealthStatus represents the overall health status.
type HealthStatus struct {
	Status   string            `json:"status"`
	Agents   []agent.StatusReport `json:"agents"`
	Database *DatabaseHealth   `json:"database,omitempty"`
	Uptime   time.Duration     `json:"uptime"`
}

// HealthChecker monitors agent and database health.
type HealthChecker struct {
	agents   []agent.Agent
	database DatabaseHealthChecker
	logger   *slog.Logger
	startTime time.Time
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(agents []agent.Agent, database DatabaseHealthChecker, logger *slog.Logger) *HealthChecker {
	return &HealthChecker{
		agents:    agents,
		database:  database,
		logger:    logger,
		startTime: time.Now(),
	}
}

// Check returns health status for all agents.
func (h *HealthChecker) Check() []agent.StatusReport {
	reports := make([]agent.StatusReport, len(h.agents))
	for i, a := range h.agents {
		reports[i] = a.Status()
	}
	return reports
}

// CheckAll returns comprehensive health status including database.
func (h *HealthChecker) CheckAll(ctx context.Context) *HealthStatus {
	// Get agent status
	agentReports := h.Check()

	// Get database health if available
	var dbHealth *DatabaseHealth
	if h.database != nil {
		if health, err := h.database.HealthCheck(ctx); err != nil {
			h.logger.Error("database health check failed", "error", err)
			dbHealth = &DatabaseHealth{
				Status: "unhealthy",
				Error:  err.Error(),
			}
		} else {
			dbHealth = health
		}
	}

	// Determine overall status
	status := h.determineOverallStatus(agentReports, dbHealth)

	return &HealthStatus{
		Status:   status,
		Agents:   agentReports,
		Database: dbHealth,
		Uptime:   time.Since(h.startTime),
	}
}

// IsHealthy returns true if all agents report a non-failed state.
func (h *HealthChecker) IsHealthy() bool {
	for _, a := range h.agents {
		if a.Status().State == "failed" {
			return false
		}
	}
	return true
}

// IsSystemHealthy returns true if both agents and database are healthy.
func (h *HealthChecker) IsSystemHealthy(ctx context.Context) bool {
	// Check agents
	for _, a := range h.agents {
		if a.Status().State == "failed" {
			return false
		}
	}

	// Check database if available
	if h.database != nil {
		if health, err := h.database.HealthCheck(ctx); err != nil || health.Status == "unhealthy" {
			return false
		}
	}

	return true
}

// determineOverallStatus determines the overall system health status.
func (h *HealthChecker) determineOverallStatus(agentReports []agent.StatusReport, dbHealth *DatabaseHealth) string {
	// Check database health
	if dbHealth != nil && dbHealth.Status == "unhealthy" {
		return "unhealthy"
	}

	// Check agent health
	hasFailedAgent := false
	hasDegradedAgent := false
	
	for _, report := range agentReports {
		switch report.State {
		case "failed":
			hasFailedAgent = true
		case "degraded":
			hasDegradedAgent = true
		}
	}

	if hasFailedAgent {
		return "unhealthy"
	}

	if hasDegradedAgent || (dbHealth != nil && dbHealth.Status == "degraded") {
		return "degraded"
	}

	return "healthy"
}
