package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
)

// ShutdownManager coordinates graceful shutdown across all agents
type ShutdownManager struct {
	agents           []agent.Agent
	store            state.Store
	config           *config.Config
	logger           *slog.Logger
	structuredLogger *observability.StructuredLogger
	shutdownTimeout  time.Duration
	cleanupHandlers  []CleanupHandler
	mu               sync.RWMutex
}

// CleanupHandler defines a function that performs cleanup during shutdown
type CleanupHandler func(ctx context.Context) error

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(agents []agent.Agent, store state.Store, cfg *config.Config, logger *slog.Logger) *ShutdownManager {
	timeout := 30 * time.Second
	if cfg.Shutdown.Timeout > 0 {
		timeout = cfg.Shutdown.Timeout
	}

	return &ShutdownManager{
		agents:          agents,
		store:           store,
		config:          cfg,
		logger:          logger,
		shutdownTimeout: timeout,
		cleanupHandlers: make([]CleanupHandler, 0),
	}
}

// WithObservability adds observability features to the shutdown manager
func (sm *ShutdownManager) WithObservability(structuredLogger *observability.StructuredLogger) *ShutdownManager {
	sm.structuredLogger = structuredLogger
	return sm
}

// AddCleanupHandler registers a cleanup function to be called during shutdown
func (sm *ShutdownManager) AddCleanupHandler(handler CleanupHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cleanupHandlers = append(sm.cleanupHandlers, handler)
}

// Shutdown performs graceful shutdown with timeout protection
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	// Create enriched correlation context for shutdown operations
	ctx = observability.EnsureCorrelationContext(ctx, "shutdown_manager", 0)

	sm.logger.Info("initiating graceful shutdown", "timeout", sm.shutdownTimeout)

	// Log shutdown initiation
	if sm.structuredLogger != nil {
		sm.structuredLogger.LogAgentStart(ctx, "shutdown_manager", "graceful shutdown initiated")
		sm.structuredLogger.LogWorkflowTransition(ctx, 0, "running", "shutting_down", "signal_received")
	}

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, sm.shutdownTimeout)
	defer cancel()

	var shutdownErr error

	// Step 1: Save current state and create checkpoints
	if err := sm.saveCheckpoints(shutdownCtx); err != nil {
		sm.logger.Error("failed to save checkpoints", "error", err)
		shutdownErr = fmt.Errorf("checkpoint save failed: %w", err)
	}

	// Step 2: Signal agents to stop (context cancellation handles this)
	sm.logger.Info("signaling agents to stop")
	if sm.structuredLogger != nil {
		sm.structuredLogger.LogAgentHandoff(shutdownCtx, "shutdown_manager", "agents", "graceful_stop_signal", len(sm.agents))
	}

	// Step 3: Wait for agents to finish current work (handled by orchestrator's errgroup)

	// Step 4: Run cleanup handlers
	if err := sm.runCleanupHandlers(shutdownCtx); err != nil {
		sm.logger.Error("cleanup handlers failed", "error", err)
		if shutdownErr == nil {
			shutdownErr = fmt.Errorf("cleanup failed: %w", err)
		}
	}

	// Step 5: Clean workspace directories if configured
	if sm.config.Shutdown.CleanupWorkspaces {
		if err := sm.cleanupWorkspaces(shutdownCtx); err != nil {
			sm.logger.Error("workspace cleanup failed", "error", err)
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("workspace cleanup failed: %w", err)
			}
		}
	}

	// Log shutdown completion
	if sm.structuredLogger != nil {
		sm.structuredLogger.LogWorkflowTransition(shutdownCtx, 0, "shutting_down", "stopped", "graceful_shutdown_complete")
		sm.structuredLogger.LogAgentStop(shutdownCtx, "shutdown_manager", 0, shutdownErr)
	}

	if shutdownErr != nil {
		sm.logger.Error("shutdown completed with errors", "error", shutdownErr)
	} else {
		sm.logger.Info("graceful shutdown completed successfully")
	}

	return shutdownErr
}

// saveCheckpoints saves the current state of all active workflows
func (sm *ShutdownManager) saveCheckpoints(ctx context.Context) error {
	sm.logger.Info("saving workflow checkpoints")

	// For each agent, try to save their current state
	// This is a best-effort operation
	for _, a := range sm.agents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		status := a.Status()
		if status.State == string(state.StateIdle) {
			continue // No active work to checkpoint
		}

		// Create a checkpoint record
		checkpoint := &state.AgentWorkState{
			AgentType:      string(a.Type()),
			IssueNumber:    status.IssueID,
			State:          state.WorkflowState(status.State),
			UpdatedAt:      time.Now(),
			CheckpointedAt: time.Now(),
			InterruptedBy:  "graceful_shutdown",
		}

		if err := sm.store.Save(ctx, checkpoint); err != nil {
			sm.logger.Error("failed to save checkpoint",
				"agent", a.Type(),
				"issue", status.IssueID,
				"error", err)
			continue
		}

		sm.logger.Debug("saved checkpoint",
			"agent", a.Type(),
			"issue", status.IssueID,
			"state", status.State)
	}

	return nil
}

// runCleanupHandlers executes all registered cleanup handlers
func (sm *ShutdownManager) runCleanupHandlers(ctx context.Context) error {
	sm.mu.RLock()
	handlers := make([]CleanupHandler, len(sm.cleanupHandlers))
	copy(handlers, sm.cleanupHandlers)
	sm.mu.RUnlock()

	sm.logger.Info("running cleanup handlers", "count", len(handlers))

	var cleanupErr error
	for i, handler := range handlers {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cleanup timeout: %w", ctx.Err())
		default:
		}

		if err := handler(ctx); err != nil {
			sm.logger.Error("cleanup handler failed", "handler_index", i, "error", err)
			if cleanupErr == nil {
				cleanupErr = err
			}
		}
	}

	return cleanupErr
}

// cleanupWorkspaces removes temporary workspace directories
func (sm *ShutdownManager) cleanupWorkspaces(ctx context.Context) error {
	if sm.config.Agents.Developer.WorkspaceDir == "" {
		return nil
	}

	workspaceDir := sm.config.Agents.Developer.WorkspaceDir
	sm.logger.Info("cleaning workspace directories", "dir", workspaceDir)

	// Check if workspace directory exists
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		return nil // Nothing to clean
	}

	// List all issue directories
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return fmt.Errorf("reading workspace directory: %w", err)
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		// Remove issue-specific workspace directories
		if filepath.HasPrefix(entry.Name(), "issue-") {
			dirPath := filepath.Join(workspaceDir, entry.Name())
			if err := os.RemoveAll(dirPath); err != nil {
				sm.logger.Error("failed to remove workspace directory",
					"dir", dirPath,
					"error", err)
				continue
			}
			sm.logger.Debug("removed workspace directory", "dir", dirPath)
		}
	}

	return nil
}

// ForceShutdown performs immediate termination after timeout
func (sm *ShutdownManager) ForceShutdown(reason string) {
	sm.logger.Warn("force shutdown initiated", "reason", reason)

	// Log force shutdown
	if sm.structuredLogger != nil {
		ctx := observability.EnsureCorrelationContext(context.Background(), "shutdown_manager", 0)
		sm.structuredLogger.LogWorkflowTransition(ctx, 0, "shutting_down", "force_stopped", reason)
		sm.structuredLogger.LogDecisionPoint(ctx, "shutdown_manager", "force_shutdown", reason, map[string]interface{}{
			"shutdown_type": "forced",
			"reason":        reason,
		})
	}

	// Perform minimal critical cleanup
	sm.runEmergencyCleanup()

	sm.logger.Error("force shutdown completed")
	os.Exit(1)
}

// runEmergencyCleanup performs minimal critical cleanup during force shutdown
func (sm *ShutdownManager) runEmergencyCleanup() {
	// Quick cleanup of critical resources with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Save emergency checkpoint
	sm.logger.Info("saving emergency checkpoints")
	_ = sm.saveCheckpoints(ctx) // Best effort, ignore errors

	// Close any open file handles or connections
	sm.logger.Info("closing open resources")
}
