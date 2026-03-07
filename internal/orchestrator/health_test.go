package orchestrator

import (
	"log/slog"
	"testing"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	agents := []agent.Agent{newMockAgent(agent.TypeDeveloper)}
	logger := slog.Default()

	hc := NewHealthChecker(agents, logger)
	require.NotNil(t, hc)
	assert.Len(t, hc.agents, 1)
}

func TestNewHealthChecker_NoAgents(t *testing.T) {
	hc := NewHealthChecker(nil, slog.Default())
	require.NotNil(t, hc)
	assert.Empty(t, hc.agents)
}

func TestCheck_ReturnsAllReports(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{Type: agent.TypeDeveloper, State: "idle"})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{Type: agent.TypeQA, State: "implement"})

	hc := NewHealthChecker([]agent.Agent{ma1, ma2}, slog.Default())
	reports := hc.Check()

	require.Len(t, reports, 2)
	assert.Equal(t, agent.TypeDeveloper, reports[0].Type)
	assert.Equal(t, "idle", reports[0].State)
	assert.Equal(t, agent.TypeQA, reports[1].Type)
	assert.Equal(t, "implement", reports[1].State)
}

func TestCheck_NoAgents(t *testing.T) {
	hc := NewHealthChecker(nil, slog.Default())
	reports := hc.Check()
	assert.Empty(t, reports)
}

func TestIsHealthy_AllHealthy(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{State: "idle"})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{State: "implement"})

	hc := NewHealthChecker([]agent.Agent{ma1, ma2}, slog.Default())
	assert.True(t, hc.IsHealthy())
}

func TestIsHealthy_OneFailed(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{State: "idle"})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{State: "failed"})

	hc := NewHealthChecker([]agent.Agent{ma1, ma2}, slog.Default())
	assert.False(t, hc.IsHealthy())
}

func TestIsHealthy_AllFailed(t *testing.T) {
	ma1 := newMockAgent(agent.TypeDeveloper)
	ma1.setStatus(agent.StatusReport{State: "failed"})
	ma2 := newMockAgent(agent.TypeQA)
	ma2.setStatus(agent.StatusReport{State: "failed"})

	hc := NewHealthChecker([]agent.Agent{ma1, ma2}, slog.Default())
	assert.False(t, hc.IsHealthy())
}

func TestIsHealthy_NoAgents(t *testing.T) {
	hc := NewHealthChecker(nil, slog.Default())
	assert.True(t, hc.IsHealthy(), "no agents means nothing is failed")
}

func TestIsHealthy_SingleAgentHealthy(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{State: "idle"})

	hc := NewHealthChecker([]agent.Agent{ma}, slog.Default())
	assert.True(t, hc.IsHealthy())
}

func TestIsHealthy_SingleAgentFailed(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{State: "failed"})

	hc := NewHealthChecker([]agent.Agent{ma}, slog.Default())
	assert.False(t, hc.IsHealthy())
}

func TestIsHealthy_VariousStates(t *testing.T) {
	states := []string{"idle", "claim", "workspace", "analyze", "implement", "commit", "pr", "validation", "review", "complete"}
	for _, s := range states {
		ma := newMockAgent(agent.TypeDeveloper)
		ma.setStatus(agent.StatusReport{State: s})
		hc := NewHealthChecker([]agent.Agent{ma}, slog.Default())
		assert.True(t, hc.IsHealthy(), "state %q should be considered healthy", s)
	}
}

func TestCheck_StatusUpdates(t *testing.T) {
	ma := newMockAgent(agent.TypeDeveloper)
	ma.setStatus(agent.StatusReport{State: "idle"})

	hc := NewHealthChecker([]agent.Agent{ma}, slog.Default())

	reports := hc.Check()
	assert.Equal(t, "idle", reports[0].State)

	// Update status
	ma.setStatus(agent.StatusReport{State: "implement", IssueID: 42})

	reports = hc.Check()
	assert.Equal(t, "implement", reports[0].State)
	assert.Equal(t, 42, reports[0].IssueID)
}
