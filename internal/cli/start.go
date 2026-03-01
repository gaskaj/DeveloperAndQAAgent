package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/claude"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/developer"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/observability"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/orchestrator"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent loop",
	Long:  "Start all enabled agents. They will poll GitHub for issues and process them autonomously.",
	RunE:  runStart,
}

func runStart(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("--config flag is required")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := setupLogger(cfg.Logging.Level)
	logger.Info("starting agentctl", "config", cfgFile)

	// Initialize observability components
	structuredLogger := observability.NewStructuredLogger(cfg.Logging)
	metrics := observability.NewMetrics(structuredLogger)

	// Initialize dependencies.
	ghClient := ghub.NewClient(cfg.GitHub.Token, cfg.GitHub.Owner, cfg.GitHub.Repo)
	claudeClient := claude.NewClient(cfg.Claude.APIKey, cfg.Claude.Model, cfg.Claude.MaxTokens).
		WithObservability(structuredLogger, metrics)

	store, err := state.NewFileStore(cfg.State.Dir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	deps := agent.Dependencies{
		Config:           cfg,
		GitHub:           ghClient,
		Claude:           claudeClient,
		Store:            store,
		Logger:           logger,
		StructuredLogger: structuredLogger,
		Metrics:          metrics,
	}

	// Create enabled agents.
	var agents []agent.Agent

	if cfg.Agents.Developer.Enabled {
		dev, err := developer.New(deps)
		if err != nil {
			return fmt.Errorf("creating developer agent: %w", err)
		}
		agents = append(agents, dev)
	}

	if len(agents) == 0 {
		return fmt.Errorf("no agents enabled in configuration")
	}

	// Setup signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("agents starting", "count", len(agents))

	// Run orchestrator.
	orch := orchestrator.New(agents, logger).WithObservability(structuredLogger, metrics)
	if err := orch.Run(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("orchestrator error: %w", err)
	}

	logger.Info("agentctl stopped")
	return nil
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
