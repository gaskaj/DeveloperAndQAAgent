package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CleanupStrategy defines how workspaces should be cleaned up.
type CleanupStrategy interface {
	// ShouldCleanup determines if a workspace should be cleaned up.
	ShouldCleanup(ctx context.Context, workspace *Workspace) (bool, string)
}

// AgeBased cleanup strategy cleans up workspaces based on age and state.
type AgeBasedStrategy struct {
	SuccessRetention time.Duration
	FailureRetention time.Duration
	StaleRetention   time.Duration
}

// ShouldCleanup implements CleanupStrategy for age-based cleanup.
func (s *AgeBasedStrategy) ShouldCleanup(ctx context.Context, workspace *Workspace) (bool, string) {
	now := time.Now()
	age := now.Sub(workspace.UpdatedAt)

	switch workspace.State {
	case WorkspaceStateActive:
		// Keep active workspaces, they should be managed by the workflow
		return false, "active workspace"
		
	case WorkspaceStateFailed:
		if age > s.FailureRetention {
			return true, fmt.Sprintf("failed workspace older than %v", s.FailureRetention)
		}
		
	case WorkspaceStateStale:
		if age > s.StaleRetention {
			return true, fmt.Sprintf("stale workspace older than %v", s.StaleRetention)
		}
		
	case WorkspaceStateCleaned:
		// Already cleaned, should not exist
		return true, "workspace already marked as cleaned"
	}

	return false, "within retention period"
}

// SizeBased cleanup strategy cleans up workspaces that exceed size limits.
type SizeBasedStrategy struct {
	MaxSizeMB int64
}

// ShouldCleanup implements CleanupStrategy for size-based cleanup.
func (s *SizeBasedStrategy) ShouldCleanup(ctx context.Context, workspace *Workspace) (bool, string) {
	if workspace.SizeMB > s.MaxSizeMB {
		return true, fmt.Sprintf("workspace size %d MB exceeds limit %d MB", workspace.SizeMB, s.MaxSizeMB)
	}
	return false, "within size limit"
}

// CompositeStrategy combines multiple cleanup strategies.
type CompositeStrategy struct {
	Strategies []CleanupStrategy
	Mode       string // "any" or "all"
}

// ShouldCleanup implements CleanupStrategy for composite cleanup.
func (s *CompositeStrategy) ShouldCleanup(ctx context.Context, workspace *Workspace) (bool, string) {
	var reasons []string
	var shouldCleanupCount int

	for _, strategy := range s.Strategies {
		should, reason := strategy.ShouldCleanup(ctx, workspace)
		if should {
			shouldCleanupCount++
			reasons = append(reasons, reason)
		}
	}

	switch s.Mode {
	case "any":
		if shouldCleanupCount > 0 {
			return true, fmt.Sprintf("matches %d strategies: %v", shouldCleanupCount, reasons)
		}
	case "all":
		if shouldCleanupCount == len(s.Strategies) {
			return true, fmt.Sprintf("matches all strategies: %v", reasons)
		}
	}

	return false, "cleanup criteria not met"
}

// Scheduler handles periodic cleanup operations.
type Scheduler struct {
	manager    Manager
	strategy   CleanupStrategy
	config     ManagerConfig
	logger     *slog.Logger
	stopCh     chan struct{}
	stoppedCh  chan struct{}
}

// NewScheduler creates a new cleanup scheduler.
func NewScheduler(manager Manager, config ManagerConfig, logger *slog.Logger) *Scheduler {
	// Create default cleanup strategy
	strategy := &CompositeStrategy{
		Mode: "any",
		Strategies: []CleanupStrategy{
			&AgeBasedStrategy{
				SuccessRetention: config.SuccessRetention,
				FailureRetention: config.FailureRetention,
				StaleRetention:   config.FailureRetention, // Use failure retention for stale
			},
			&SizeBasedStrategy{
				MaxSizeMB: config.MaxSizeMB,
			},
		},
	}

	return &Scheduler{
		manager:   manager,
		strategy:  strategy,
		config:    config,
		logger:    logger,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start begins the cleanup scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.config.CleanupEnabled {
		s.logger.Info("workspace cleanup disabled")
		close(s.stoppedCh)
		return
	}

	s.logger.Info("starting workspace cleanup scheduler",
		"interval", s.config.CleanupInterval,
		"success_retention", s.config.SuccessRetention,
		"failure_retention", s.config.FailureRetention,
	)

	go s.run(ctx)
}

// Stop stops the cleanup scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.stoppedCh
	s.logger.Info("workspace cleanup scheduler stopped")
}

// run is the main cleanup loop.
func (s *Scheduler) run(ctx context.Context) {
	defer close(s.stoppedCh)

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	// Run initial cleanup
	s.runCleanupCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("cleanup scheduler context cancelled")
			return
		case <-s.stopCh:
			s.logger.Debug("cleanup scheduler stop requested")
			return
		case <-ticker.C:
			s.runCleanupCycle(ctx)
		}
	}
}

// runCleanupCycle performs a single cleanup cycle.
func (s *Scheduler) runCleanupCycle(ctx context.Context) {
	startTime := time.Now()
	s.logger.Debug("starting cleanup cycle")

	// Get all workspaces
	allWorkspaces, err := s.manager.ListWorkspaces(ctx, "")
	if err != nil {
		s.logger.Error("failed to list workspaces for cleanup", "error", err)
		return
	}

	var cleanedCount int
	var skippedCount int
	var errorCount int

	for _, workspace := range allWorkspaces {
		// Check if workspace should be cleaned up
		shouldCleanup, reason := s.strategy.ShouldCleanup(ctx, workspace)
		if !shouldCleanup {
			skippedCount++
			s.logger.Debug("skipping workspace cleanup",
				"issue_id", workspace.ID,
				"reason", reason,
			)
			continue
		}

		// Perform cleanup
		s.logger.Info("cleaning up workspace",
			"issue_id", workspace.ID,
			"state", workspace.State,
			"age", time.Since(workspace.UpdatedAt),
			"size_mb", workspace.SizeMB,
			"reason", reason,
		)

		if err := s.manager.CleanupWorkspace(ctx, workspace.ID); err != nil {
			s.logger.Error("failed to cleanup workspace",
				"issue_id", workspace.ID,
				"error", err,
			)
			errorCount++
		} else {
			cleanedCount++
		}

		// Check for context cancellation
		if ctx.Err() != nil {
			s.logger.Info("cleanup cycle cancelled",
				"cleaned_count", cleanedCount,
				"error_count", errorCount,
			)
			return
		}
	}

	duration := time.Since(startTime)
	s.logger.Info("cleanup cycle completed",
		"duration", duration,
		"total_workspaces", len(allWorkspaces),
		"cleaned_count", cleanedCount,
		"skipped_count", skippedCount,
		"error_count", errorCount,
	)
}

// ForceCleanup performs an immediate cleanup of all eligible workspaces.
func (s *Scheduler) ForceCleanup(ctx context.Context) error {
	s.logger.Info("performing forced cleanup")
	s.runCleanupCycle(ctx)
	return nil
}

// CleanupWorkspacesByAge is a convenience function for age-based cleanup.
func CleanupWorkspacesByAge(ctx context.Context, manager Manager, olderThan time.Duration, logger *slog.Logger) error {
	workspaces, err := manager.ListWorkspaces(ctx, "")
	if err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}

	strategy := &AgeBasedStrategy{
		SuccessRetention: olderThan,
		FailureRetention: olderThan,
		StaleRetention:   olderThan,
	}

	var cleanedCount int
	cutoffTime := time.Now().Add(-olderThan)

	for _, workspace := range workspaces {
		if workspace.UpdatedAt.After(cutoffTime) {
			continue
		}

		shouldCleanup, reason := strategy.ShouldCleanup(ctx, workspace)
		if !shouldCleanup {
			continue
		}

		logger.Debug("cleaning up workspace by age",
			"issue_id", workspace.ID,
			"age", time.Since(workspace.UpdatedAt),
			"reason", reason,
		)

		if err := manager.CleanupWorkspace(ctx, workspace.ID); err != nil {
			logger.Error("failed to cleanup workspace by age",
				"issue_id", workspace.ID,
				"error", err,
			)
			continue
		}

		cleanedCount++

		// Check for context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	logger.Info("age-based cleanup completed",
		"older_than", olderThan,
		"cleaned_count", cleanedCount,
	)

	return nil
}