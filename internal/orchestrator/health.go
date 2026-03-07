package orchestrator

import (
	"log/slog"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
)

// HealthChecker monitors agent health.
type HealthChecker struct {
	agents []agent.Agent
	logger *slog.Logger
}

// NewHealthChecker creates a new HealthChecker.
func NewHealthChecker(agents []agent.Agent, logger *slog.Logger) *HealthChecker {
	return &HealthChecker{
		agents: agents,
		logger: logger,
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

// IsHealthy returns true if all agents report a non-failed state.
func (h *HealthChecker) IsHealthy() bool {
	for _, a := range h.agents {
		if a.Status().State == "failed" {
			return false
		}
	}
	return true
}
