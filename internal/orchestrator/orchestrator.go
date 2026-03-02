package orchestrator

import (
	"context"
	"log/slog"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"
	"golang.org/x/sync/errgroup"
)

// Orchestrator manages the lifecycle of multiple agents.
type Orchestrator struct {
	agents           []agent.Agent
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	metrics          *observability.Metrics
	cleanupScheduler *workspace.Scheduler
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

// WithWorkspaceCleanup adds workspace cleanup scheduling to the orchestrator
func (o *Orchestrator) WithWorkspaceCleanup(scheduler *workspace.Scheduler) *Orchestrator {
	o.cleanupScheduler = scheduler
	return o
}

// Run starts all agents concurrently and blocks until they all stop or the context is cancelled.
// The context cancellation triggers graceful shutdown of all agents.
func (o *Orchestrator) Run(ctx context.Context) error {
	// Create enriched correlation context for orchestrator operations
	ctx = observability.EnsureCorrelationContext(ctx, "orchestrator", 0)
	
	// Log orchestrator start
	if o.structuredLogger != nil {
		o.structuredLogger.LogAgentStart(ctx, "orchestrator", "multi-agent system starting")
	}
	
	g, ctx := errgroup.WithContext(ctx)
	
	// Start workspace cleanup scheduler if configured
	if o.cleanupScheduler != nil {
		o.logger.Info("starting workspace cleanup scheduler")
		o.cleanupScheduler.Start(ctx)
		defer func() {
			o.logger.Info("stopping workspace cleanup scheduler")
			o.cleanupScheduler.Stop()
		}()
	}

	for _, a := range o.agents {
		a := a // capture for goroutine
		agentType := string(a.Type())
		
		o.logger.Info("starting agent", "type", agentType)
		
		// Log workflow transition to starting state and agent handoff
		if o.structuredLogger != nil {
			o.structuredLogger.LogWorkflowTransition(ctx, 0, "stopped", "starting", "orchestrator_start_agent")
			o.structuredLogger.LogAgentHandoff(ctx, "orchestrator", agentType, "system_startup", 0)
		}
		if o.metrics != nil {
			o.metrics.RecordWorkflowTransition(ctx, "stopped", "starting")
		}

		g.Go(func() error {
			// Create agent-specific correlation context
			agentCtx := observability.WithHandoff(ctx, "orchestrator", agentType, "startup", 0)
			agentCtx = observability.WithWorkflowStage(agentCtx, observability.WorkflowStageStart)
			
			if err := a.Run(agentCtx); err != nil {
				// Check if this is a graceful shutdown (context cancelled)
				if agentCtx.Err() != nil {
					o.logger.Info("agent stopped due to context cancellation", "type", agentType)
					
					// Log graceful shutdown
					if o.structuredLogger != nil {
						o.structuredLogger.LogWorkflowTransition(agentCtx, 0, "running", "stopped", "graceful_shutdown")
						o.structuredLogger.LogAgentHandoff(agentCtx, agentType, "orchestrator", "graceful_shutdown", 0)
					}
					if o.metrics != nil {
						o.metrics.RecordWorkflowTransition(agentCtx, "running", "stopped")
					}
					
					return nil // Context cancellation is not an error
				}
				
				o.logger.Error("agent stopped with error",
					"type", agentType,
					"error", err,
				)
				
				// Log workflow transition to error state
				if o.structuredLogger != nil {
					o.structuredLogger.LogWorkflowTransition(agentCtx, 0, "running", "error", err.Error())
					o.structuredLogger.LogDecisionPoint(agentCtx, agentType, "agent_failed", err.Error(), map[string]interface{}{
						"error_type": "agent_runtime_error",
					})
				}
				if o.metrics != nil {
					o.metrics.RecordWorkflowTransition(agentCtx, "running", "error")
				}
				
				return err
			}
			
			// Log successful shutdown with handoff back to orchestrator
			if o.structuredLogger != nil {
				o.structuredLogger.LogWorkflowTransition(agentCtx, 0, "running", "stopped", "graceful_shutdown")
				o.structuredLogger.LogAgentHandoff(agentCtx, agentType, "orchestrator", "shutdown", 0)
			}
			if o.metrics != nil {
				o.metrics.RecordWorkflowTransition(agentCtx, "running", "stopped")
			}
			
			return nil
		})
	}

	err := g.Wait()
	
	// Log orchestrator stop
	if o.structuredLogger != nil {
		o.structuredLogger.LogAgentStop(ctx, "orchestrator", 0, err)
	}
	
	return err
}

// Status returns status reports from all managed agents.
func (o *Orchestrator) Status() []agent.StatusReport {
	reports := make([]agent.StatusReport, len(o.agents))
	for i, a := range o.agents {
		reports[i] = a.Status()
	}
	return reports
}
