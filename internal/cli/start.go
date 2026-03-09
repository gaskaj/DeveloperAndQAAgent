package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/claude"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/developer"
	"github.com/gaskaj/OpenAgentFramework/internal/errors"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/orchestrator"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/gaskaj/OpenAgentFramework/internal/version"
	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
	"github.com/gaskaj/OpenAgentFramework/pkg/reporter"
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

	// If remote config mode is enabled, fetch config from control plane
	if cfg.ControlPlane.Enabled && cfg.ControlPlane.ConfigMode == "remote" {
		fetcher := reporter.NewConfigFetcher(reporter.Config{
			ControlPlaneURL: cfg.ControlPlane.URL,
			APIKey:          cfg.ControlPlane.APIKey,
		})
		remoteResp, fetchErr := fetcher.FetchConfig(context.Background())
		if fetchErr != nil {
			return fmt.Errorf("fetching remote config: %w", fetchErr)
		}
		if remoteResp != nil {
			config.MergeRemoteConfig(cfg, &remoteResp.Config)
		}
		// When running in remote mode, default the developer agent to enabled
		// since the user is explicitly starting the agent process.
		if !cfg.Agents.Developer.Enabled {
			cfg.Agents.Developer.Enabled = true
		}
		// Validate the merged config now that remote values are applied
		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("validating merged remote config: %w", err)
		}
	}

	logger := setupLogger(cfg.Logging.Level)
	logger.Info("starting agentctl", "config", cfgFile)

	// Initialize observability components
	structuredLogger := observability.NewStructuredLogger(cfg.Logging)
	metrics := observability.NewMetrics(structuredLogger)

	// Initialize log rotation and cleanup managers
	var rotationManager *observability.LogRotationManager
	var cleanupManager *observability.LogCleanupManager

	if cfg.Logging.FilePath != "" {
		if cfg.Logging.Rotation.Enabled {
			rotationManager = observability.NewLogRotationManager(cfg.Logging.Rotation)
		}
		if cfg.Logging.Cleanup.Enabled {
			cleanupManager = observability.NewLogCleanupManager(cfg.Logging.Cleanup)
		}
	}

	// Initialize error handling manager
	errorManager := errors.NewManager(&cfg.ErrorHandling, logger).
		WithObservability(structuredLogger, metrics)

	// Initialize dependencies.
	ghClient := ghub.NewClient(cfg.GitHub.Token, cfg.GitHub.Owner, cfg.GitHub.Repo).
		WithErrorHandling(errorManager)
	claudeClient := claude.NewClient(cfg.Claude.APIKey, cfg.Claude.Model, cfg.Claude.MaxTokens).
		WithObservability(structuredLogger, metrics).
		WithErrorHandling(errorManager)

	store, err := state.NewFileStore(cfg.State.Dir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	// Initialize control plane reporter if configured
	var rptr *reporter.Reporter
	var logHandler *reporter.LogHandler
	if cfg.ControlPlane.Enabled {
		hostname, _ := os.Hostname()
		rptrCfg := reporter.Config{
			ControlPlaneURL: cfg.ControlPlane.URL,
			APIKey:          cfg.ControlPlane.APIKey,
			AgentType:       "developer",
			Version:         version.Version,
			Hostname:        hostname,
			GitHubOwner:     cfg.GitHub.Owner,
			GitHubRepo:      cfg.GitHub.Repo,
			FlushInterval:   5 * time.Second,
		}
		rptr, err = reporter.New(rptrCfg)
		if err != nil {
			logger.Warn("failed to initialize control plane reporter, continuing without it", "error", err)
		} else {
			logger.Info("control plane reporter initialized", "url", cfg.ControlPlane.URL)

			// Wrap logger with forwarding handler so all agent logs stream to control plane
			logHandler = reporter.NewLogHandler(logger.Handler(), rptrCfg, slog.LevelDebug)
			logger = slog.New(logHandler)
		}
	}

	deps := agent.Dependencies{
		Config:           cfg,
		GitHub:           ghClient,
		Claude:           claudeClient,
		Store:            store,
		Logger:           logger,
		StructuredLogger: structuredLogger,
		Metrics:          metrics,
		ErrorManager:     errorManager,
		Reporter:         rptr,
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

	// Setup signal handling with graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start control plane heartbeat now that we have a context
	if rptr != nil && cfg.ControlPlane.HeartbeatInterval > 0 {
		rptr.Heartbeat(ctx, cfg.ControlPlane.HeartbeatInterval)
	}

	// Start config polling if in remote mode
	if cfg.ControlPlane.Enabled && cfg.ControlPlane.ConfigMode == "remote" {
		pollInterval := cfg.ControlPlane.ConfigPollInterval
		if pollInterval == 0 {
			pollInterval = 30 * time.Second
		}
		configFetcher := reporter.NewConfigFetcher(reporter.Config{
			ControlPlaneURL: cfg.ControlPlane.URL,
			APIKey:          cfg.ControlPlane.APIKey,
		})
		go configFetcher.PollLoop(ctx, pollInterval, func(resp *apitypes.ConfigResponse) {
			config.MergeRemoteConfig(cfg, &resp.Config)
			logger.Info("configuration updated from control plane", "version", resp.Version)
		})
	}

	// Initialize recovery manager to handle interrupted workflows
	recoveryManager := state.NewRecoveryManager(store, ghClient, cfg, logger).
		WithObservability(structuredLogger)

	// Perform recovery before starting agents
	if err := recoveryManager.RecoverInterruptedWorkflows(ctx); err != nil {
		logger.Error("recovery failed, continuing anyway", "error", err)
	}

	logger.Info("agents starting", "count", len(agents))

	// Run orchestrator with graceful shutdown support.
	orch := orchestrator.New(agents, logger).WithObservability(structuredLogger, metrics)

	// Add log management to orchestrator if configured
	if rotationManager != nil {
		orch = orch.WithLogRotation(rotationManager)
	}
	if cleanupManager != nil {
		orch = orch.WithLogCleanup(cleanupManager)
	}
	if cfg.Logging.FilePath != "" {
		orch = orch.WithLogFilePath(cfg.Logging.FilePath)
	}

	shutdownManager := orchestrator.NewShutdownManager(agents, store, cfg, logger).
		WithObservability(structuredLogger)

	// Start orchestrator in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Run(ctx)
	}()

	// Wait for either completion or signal
	select {
	case err := <-errCh:
		// Orchestrator completed normally
		if err != nil && ctx.Err() == nil {
			return fmt.Errorf("orchestrator error: %w", err)
		}
	case <-ctx.Done():
		// Signal received, perform graceful shutdown
		logger.Info("shutdown signal received, initiating graceful shutdown")

		// Start force shutdown timer
		forceCtx, forceCancel := context.WithTimeout(context.Background(), cfg.Shutdown.Timeout+10*time.Second)
		defer forceCancel()

		go func() {
			<-forceCtx.Done()
			if forceCtx.Err() == context.DeadlineExceeded {
				shutdownManager.ForceShutdown("shutdown timeout exceeded")
			}
		}()

		// Perform graceful shutdown
		if err := shutdownManager.Shutdown(context.Background()); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}

		// Wait for orchestrator to finish
		select {
		case <-errCh:
			// Orchestrator finished
		case <-time.After(5 * time.Second):
			logger.Warn("orchestrator did not finish within timeout")
		}
	}

	// Shutdown log forwarding and control plane reporter
	if logHandler != nil {
		logHandler.Close()
	}
	if rptr != nil {
		if err := rptr.Close(); err != nil {
			logger.Error("failed to close control plane reporter", "error", err)
		}
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
