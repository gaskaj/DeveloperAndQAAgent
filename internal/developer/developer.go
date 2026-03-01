package developer

import (
	"context"
	"log/slog"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/creativity"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
)

// DeveloperAgent monitors GitHub for issues and implements solutions.
type DeveloperAgent struct {
	agent.BaseAgent
	poller *ghub.Poller
	status agent.StatusReport
}

// New creates a new DeveloperAgent.
func New(deps agent.Dependencies) (agent.Agent, error) {
	da := &DeveloperAgent{
		BaseAgent: agent.NewBaseAgent(deps),
		status: agent.StatusReport{
			Type:    agent.TypeDeveloper,
			State:   string(state.StateIdle),
			Message: "waiting for issues",
		},
	}

	da.poller = ghub.NewPoller(
		deps.GitHub,
		deps.Config.GitHub.WatchLabels,
		deps.Config.GitHub.PollInterval,
		da.handleIssues,
		deps.Logger,
	)

	// Wire up creativity engine as idle handler when enabled.
	if deps.Config.Creativity.Enabled {
		ghAdapter := creativity.NewGitHubAdapter(deps.GitHub)
		aiAdapter := creativity.NewClaudeAdapter(deps.Claude)
		engine := creativity.NewCreativityEngine(
			ghAdapter,
			aiAdapter,
			deps.Config.Creativity,
			string(agent.TypeDeveloper),
			deps.Logger.With("component", "creativity"),
		)

		da.poller.IdleHandler = func(ctx context.Context) error {
			da.updateStatus(state.StateCreativeThink, 0, "generating improvement suggestions")
			defer da.updateStatus(state.StateIdle, 0, "waiting for issues")
			return engine.Run(ctx)
		}
	}

	return da, nil
}

// Type returns the developer agent type.
func (d *DeveloperAgent) Type() agent.AgentType {
	return agent.TypeDeveloper
}

// Run starts the developer agent's polling loop.
func (d *DeveloperAgent) Run(ctx context.Context) error {
	// Ensure correlation ID for this agent run
	ctx = observability.EnsureCorrelationID(ctx)
	
	// Log agent start with structured logging
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogAgentStart(ctx, string(d.Type()), "developer agent started")
	}
	d.Deps.Logger.Info("developer agent started")

	// Start timing for agent lifecycle
	var timer *observability.Timer
	if d.Deps.Metrics != nil {
		timer = d.Deps.Metrics.Timer("agent_lifecycle", map[string]string{
			"agent_type": string(d.Type()),
		})
	}

	// Start heartbeat in background.
	go agent.Heartbeat(ctx, d.Type(), 60*time.Second, d.Deps.Logger)

	err := d.poller.Run(ctx)
	
	// Log agent stop with metrics
	var duration time.Duration
	if timer != nil {
		duration = timer.StopWithContext(ctx, "agent_lifecycle")
	}
	
	if d.Deps.StructuredLogger != nil {
		d.Deps.StructuredLogger.LogAgentStop(ctx, string(d.Type()), duration, err)
	}
	
	return err
}

// Status returns the current agent status.
func (d *DeveloperAgent) Status() agent.StatusReport {
	return d.status
}

func (d *DeveloperAgent) updateStatus(s state.WorkflowState, issueID int, msg string) {
	d.status = agent.StatusReport{
		Type:    agent.TypeDeveloper,
		State:   string(s),
		IssueID: issueID,
		Message: msg,
	}
}

func (d *DeveloperAgent) logger() *slog.Logger {
	return d.Deps.Logger.With("agent", "developer")
}
