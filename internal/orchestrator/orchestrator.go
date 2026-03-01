package orchestrator

import (
	"context"
	"log/slog"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"golang.org/x/sync/errgroup"
)

// Orchestrator manages the lifecycle of multiple agents.
type Orchestrator struct {
	agents           []agent.Agent
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
}

// New creates a new Orchestrator with the given agents.
func New(agents []agent.Agent, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		agents: agents,
		logger: logger,
	}
}

// WithObservability adds observability features to the orchestrator
func (o *Orchestrator) WithObservability(structuredLogger *observability.StructuredLogger, metrics *observability.Metrics) *Orchestrator {
	o.structuredLogger = structuredLogger
	o.metrics = metrics
	return o
}

// Run starts all agents concurrently and blocks until they all stop or the context is cancelled.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Ensure correlation ID for orchestrator operations
	ctx = observability.EnsureCorrelationID(ctx)
	
	g, ctx := errgroup.WithContext(ctx)

	for _, a := range o.agents {
		a := a // capture for goroutine
		agentType := string(a.Type())
		
		o.logger.Info("starting agent", "type", agentType)
		
		// Log workflow transition to starting state
		if o.structuredLogger != nil {
			o.structuredLogger.LogWorkflowTransition(ctx, 0, "stopped", "starting", "orchestrator_start")
		}
		if o.metrics != nil {
			o.metrics.RecordWorkflowTransition(ctx, "stopped", "starting")
		}

		g.Go(func() error {
			if err := a.Run(ctx); err != nil {
				o.logger.Error("agent stopped with error",
					"type", agentType,
					"error", err,
				)
				
				// Log workflow transition to error state
				if o.structuredLogger != nil {
					o.structuredLogger.LogWorkflowTransition(ctx, 0, "running", "error", err.Error())
				}
				if o.metrics != nil {
					o.metrics.RecordWorkflowTransition(ctx, "running", "error")
				}
				
				return err
			}
			
			// Log successful shutdown
			if o.structuredLogger != nil {
				o.structuredLogger.LogWorkflowTransition(ctx, 0, "running", "stopped", "graceful_shutdown")
			}
			if o.metrics != nil {
				o.metrics.RecordWorkflowTransition(ctx, "running", "stopped")
			}
			
			return nil
		})
	}

	return g.Wait()
}

// Status returns status reports from all managed agents.
func (o *Orchestrator) Status() []agent.StatusReport {
	reports := make([]agent.StatusReport, len(o.agents))
	for i, a := range o.agents {
		reports[i] = a.Status()
	}
	return reports
}
