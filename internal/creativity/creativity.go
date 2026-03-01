package creativity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
)

// CreativityEngine manages autonomous suggestion generation during idle periods.
type CreativityEngine struct {
	gh             GitHubClient
	ai             AIClient
	cfg            config.CreativityConfig
	rejectionCache *RejectionCache
	agentID        string
	logger         *slog.Logger
}

// NewCreativityEngine creates a new CreativityEngine.
func NewCreativityEngine(gh GitHubClient, ai AIClient, cfg config.CreativityConfig, agentID string, logger *slog.Logger) *CreativityEngine {
	return &CreativityEngine{
		gh:             gh,
		ai:             ai,
		cfg:            cfg,
		rejectionCache: NewRejectionCache(cfg.MaxRejectionHistory),
		agentID:        agentID,
		logger:         logger,
	}
}

// Run executes the creativity loop. It checks for available work, generates
// suggestions, and creates issues. It returns when real work becomes available
// or the context is cancelled.
func (e *CreativityEngine) Run(ctx context.Context) error {
	e.logger.Info("creativity engine started")
	defer e.logger.Info("creativity engine stopped")

	// Load rejection history from closed rejected issues.
	if err := e.loadRejectionHistory(ctx); err != nil {
		e.logger.Warn("failed to load rejection history", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Step 1: Check for available work — exit if found.
		hasWork, err := e.checkForAvailableWork(ctx)
		if err != nil {
			e.logger.Error("failed to check for work", "error", err)
			return fmt.Errorf("checking for work: %w", err)
		}
		if hasWork {
			e.logger.Info("work available, exiting creativity mode")
			return nil
		}

		// Step 2: Check for pending suggestions — sleep cooldown if at max.
		pending, err := e.hasPendingSuggestion(ctx)
		if err != nil {
			e.logger.Error("failed to check pending suggestions", "error", err)
			return fmt.Errorf("checking pending suggestions: %w", err)
		}
		if pending {
			e.logger.Info("max pending suggestions reached, waiting", "cooldown_seconds", e.cfg.SuggestionCooldownSeconds)
			if err := e.sleep(ctx, time.Duration(e.cfg.SuggestionCooldownSeconds)*time.Second); err != nil {
				return err
			}
			continue
		}

		// Step 3: Gather context.
		projectCtx, err := e.gatherContext(ctx)
		if err != nil {
			e.logger.Error("failed to gather context", "error", err)
			return fmt.Errorf("gathering context: %w", err)
		}

		// Step 4: Generate suggestion via AI.
		suggestion, err := e.generateSuggestion(ctx, projectCtx)
		if err != nil {
			e.logger.Warn("failed to generate suggestion", "error", err)
			if err := e.sleep(ctx, time.Duration(e.cfg.SuggestionCooldownSeconds)*time.Second); err != nil {
				return err
			}
			continue
		}

		// Step 5: Check duplicates — skip if duplicate.
		if e.isDuplicate(suggestion, projectCtx) {
			e.logger.Info("skipping duplicate suggestion", "title", suggestion.Title)
			continue
		}

		// Step 6: Create suggestion issue.
		if err := e.createSuggestionIssue(ctx, suggestion); err != nil {
			e.logger.Error("failed to create suggestion issue", "error", err)
			return fmt.Errorf("creating suggestion issue: %w", err)
		}

		e.logger.Info("created suggestion issue", "title", suggestion.Title)

		// Step 7: Sleep cooldown, then repeat.
		if err := e.sleep(ctx, time.Duration(e.cfg.SuggestionCooldownSeconds)*time.Second); err != nil {
			return err
		}
	}
}

// loadRejectionHistory loads rejected suggestion titles from closed GitHub issues.
func (e *CreativityEngine) loadRejectionHistory(ctx context.Context) error {
	issues, err := e.gh.ListIssuesByLabel(ctx, labelSuggestionRejected)
	if err != nil {
		return fmt.Errorf("loading rejection history: %w", err)
	}

	for _, issue := range issues {
		e.rejectionCache.Add(issue.Title)
	}

	e.logger.Info("loaded rejection history", "count", len(issues))
	return nil
}

// sleep waits for the given duration or until the context is cancelled.
func (e *CreativityEngine) sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
