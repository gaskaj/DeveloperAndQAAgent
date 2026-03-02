package ghub

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/go-github/v68/github"
)

// EventHandler is called when new issues matching the watch labels are found.
type EventHandler func(ctx context.Context, issues []*github.Issue) error

// Poller polls GitHub for new issues at a regular interval.
type Poller struct {
	client      Client
	labels      []string
	interval    time.Duration
	handler     EventHandler
	IdleHandler func(ctx context.Context) error
	logger      *slog.Logger
}

// NewPoller creates a new Poller.
func NewPoller(client Client, labels []string, interval time.Duration, handler EventHandler, logger *slog.Logger) *Poller {
	return &Poller{
		client:   client,
		labels:   labels,
		interval: interval,
		handler:  handler,
		logger:   logger,
	}
}

// Run starts the polling loop. It blocks until the context is cancelled.
// Respects context cancellation for graceful shutdown.
func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.logger.Info("starting GitHub poller", "interval", p.interval, "labels", p.labels)

	// Do an initial poll immediately.
	if err := p.poll(ctx); err != nil {
		p.logger.Error("initial poll failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("poller stopped due to context cancellation")
			return nil // Context cancellation is not an error
		case <-ticker.C:
			// Check context before polling
			if ctx.Err() != nil {
				p.logger.Info("context cancelled during poll cycle")
				return nil
			}
			
			if err := p.poll(ctx); err != nil {
				p.logger.Error("poll failed", "error", err)
			}
		}
	}
}

func (p *Poller) poll(ctx context.Context) error {
	issues, err := p.client.ListIssues(ctx, p.labels)
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		if p.IdleHandler != nil {
			if err := p.IdleHandler(ctx); err != nil {
				p.logger.Error("idle handler failed", "error", err)
			}
		}
		return nil
	}

	p.logger.Info("found issues", "count", len(issues))
	return p.handler(ctx, issues)
}
