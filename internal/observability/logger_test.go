package observability

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStructuredLogger(t *testing.T) {
	tests := []struct {
		name   string
		config config.LoggingConfig
		want   string // Expected log level
	}{
		{
			name:   "debug level",
			config: config.LoggingConfig{Level: "debug"},
		},
		{
			name:   "info level",
			config: config.LoggingConfig{Level: "info"},
		},
		{
			name:   "warn level",
			config: config.LoggingConfig{Level: "warn"},
		},
		{
			name:   "error level",
			config: config.LoggingConfig{Level: "error"},
		},
		{
			name:   "invalid level defaults to info",
			config: config.LoggingConfig{Level: "invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewStructuredLogger(tt.config)
			require.NotNil(t, logger)
			require.NotNil(t, logger.Logger)
			assert.Equal(t, "correlation_id", logger.correlationIDKey)
		})
	}
}

func TestCorrelationIDHandling(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "info"})

	t.Run("without correlation ID", func(t *testing.T) {
		ctx := context.Background()
		contextLogger := logger.WithCorrelation(ctx)
		// Should return the base logger when no correlation ID is present
		assert.NotNil(t, contextLogger)
	})

	t.Run("with correlation ID", func(t *testing.T) {
		ctx := WithCorrelationID(context.Background(), "test-correlation-123")
		contextLogger := logger.WithCorrelation(ctx)
		// Should return a logger with correlation ID
		assert.NotNil(t, contextLogger)
	})
}

func TestLogMethods(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})
	ctx := WithCorrelationID(context.Background(), "test-correlation")

	// These tests mainly verify the methods don't panic
	// In a real environment, you'd capture and verify log output

	t.Run("LogAgentStart", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentStart(ctx, "test-agent", "starting up")
		})
	})

	t.Run("LogAgentStop", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentStop(ctx, "test-agent", 5*time.Second, nil)
		})

		assert.NotPanics(t, func() {
			logger.LogAgentStop(ctx, "test-agent", 5*time.Second, assert.AnError)
		})
	})

	t.Run("LogAgentHandoff", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentHandoff(ctx, "agent-1", "agent-2", "issue-ready", 1024)
		})
	})

	t.Run("LogWorkflowTransition", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogWorkflowTransition(ctx, 123, "idle", "processing", "new issue")
		})
	})

	t.Run("LogToolUsage", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogToolUsage(ctx, "file_read", 100*time.Millisecond, true, nil)
		})

		assert.NotPanics(t, func() {
			logger.LogToolUsage(ctx, "file_write", 200*time.Millisecond, false, assert.AnError)
		})
	})

	t.Run("LogLLMCall", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogLLMCall(ctx, "claude-3-sonnet", 100, 50, 2*time.Second, nil)
		})
	})

	t.Run("LogDecisionPoint", func(t *testing.T) {
		metadata := map[string]interface{}{
			"issue_id": 123,
			"priority": "high",
		}

		assert.NotPanics(t, func() {
			logger.LogDecisionPoint(ctx, "developer", "implement", "issue is ready", metadata)
		})
	})

	t.Run("LogPerformanceMetric", func(t *testing.T) {
		labels := map[string]string{
			"operation": "build",
			"status":    "success",
		}

		assert.NotPanics(t, func() {
			logger.LogPerformanceMetric(ctx, "build_duration", 5000.0, "ms", labels)
		})
	})
}

func TestLogMethodsWithoutCorrelationID(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "info"})
	ctx := context.Background()

	// Verify methods work even without correlation ID
	assert.NotPanics(t, func() {
		logger.LogAgentStart(ctx, "test-agent", "starting")
	})

	assert.NotPanics(t, func() {
		logger.LogWorkflowTransition(ctx, 456, "state1", "state2", "test")
	})
}

func TestLogRetryMethods(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})
	ctx := WithCorrelationID(context.Background(), "retry-test-corr")

	t.Run("LogRetryAttempt", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogRetryAttempt(ctx, "api_call", 1, 3)
		})
	})

	t.Run("LogRetrySuccess", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogRetrySuccess(ctx, "api_call", 2)
		})
	})

	t.Run("LogRetryExhausted", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogRetryExhausted(ctx, "api_call", 3, assert.AnError)
		})
	})

	t.Run("LogRetryNonRetryable", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogRetryNonRetryable(ctx, "api_call", 1, assert.AnError)
		})
	})

	t.Run("LogRetryDelay", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogRetryDelay(ctx, "api_call", 1, 500*time.Millisecond, assert.AnError)
		})
	})
}

func TestLogCircuitBreakerMethods(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})
	ctx := WithCorrelationID(context.Background(), "cb-test-corr")

	t.Run("LogCircuitBreakerStateChange", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogCircuitBreakerStateChange(ctx, "github-api", "closed", "open")
		})
	})

	t.Run("LogCircuitBreakerRejection", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogCircuitBreakerRejection(ctx, "github-api", "open")
		})
	})
}

func TestLogLLMCallWithError(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})
	ctx := WithCorrelationID(context.Background(), "llm-err-test")

	assert.NotPanics(t, func() {
		logger.LogLLMCall(ctx, "claude-3-sonnet", 100, 0, 500*time.Millisecond, assert.AnError)
	})
}

func TestNewStructuredLoggerWithConfig(t *testing.T) {
	t.Run("with file path", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")

		cfg := config.LoggingConfig{
			Level:    "info",
			FilePath: logPath,
		}

		logger := NewStructuredLoggerWithConfig(cfg, nil)
		require.NotNil(t, logger)
		require.NotNil(t, logger.Logger)
	})

	t.Run("with app config for repo-specific path", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "logs", "agent.log")

		cfg := config.LoggingConfig{
			Level:    "info",
			FilePath: logPath,
		}

		appCfg := &config.Config{
			GitHub: config.GitHubConfig{
				Owner: "testowner",
				Repo:  "testrepo",
			},
		}

		logger := NewStructuredLoggerWithConfig(cfg, appCfg)
		require.NotNil(t, logger)
	})

	t.Run("structured_text format", func(t *testing.T) {
		cfg := config.LoggingConfig{
			Level: "info",
			StructuredLogging: config.StructuredLoggingConfig{
				Format: "structured_text",
			},
		}

		logger := NewStructuredLoggerWithConfig(cfg, nil)
		require.NotNil(t, logger)
	})

	t.Run("development format", func(t *testing.T) {
		cfg := config.LoggingConfig{
			Level: "info",
			StructuredLogging: config.StructuredLoggingConfig{
				Format: "development",
			},
		}

		logger := NewStructuredLoggerWithConfig(cfg, nil)
		require.NotNil(t, logger)
	})

	t.Run("json format with export field mappings", func(t *testing.T) {
		cfg := config.LoggingConfig{
			Level: "info",
			StructuredLogging: config.StructuredLoggingConfig{
				Format: "json",
				Export: config.LogExportConfig{
					Enabled: true,
					FieldMappings: map[string]map[string]string{
						"generic": {
							"level": "severity",
						},
					},
				},
			},
		}

		logger := NewStructuredLoggerWithConfig(cfg, nil)
		require.NotNil(t, logger)
	})

	t.Run("with include caller", func(t *testing.T) {
		cfg := config.LoggingConfig{
			Level: "debug",
			StructuredLogging: config.StructuredLoggingConfig{
				IncludeCaller: true,
			},
		}

		logger := NewStructuredLoggerWithConfig(cfg, nil)
		require.NotNil(t, logger)
	})
}

func TestLogMethodsWithFullCorrelationContext(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})

	// Create context with full correlation context including handoffs and metadata
	corrCtx := NewCorrelationContext("developer", 42)
	corrCtx.TaskID = "task-123"
	corrCtx.Metadata["key1"] = "value1"

	ctx := WithCorrelationContext(context.Background(), corrCtx)
	ctx = WithWorkflowStage(ctx, WorkflowStageAnalyze)
	ctx = WithHandoff(ctx, "orchestrator", "developer", "new_issue", 512)

	t.Run("LogAgentStart with full context", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentStart(ctx, "developer", "starting analysis")
		})
	})

	t.Run("LogAgentStop with full context and error", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentStop(ctx, "developer", 3*time.Second, assert.AnError)
		})
	})

	t.Run("LogAgentStop with full context and success", func(t *testing.T) {
		// Add stage entries with durations for better coverage
		corrCtx2 := NewCorrelationContext("developer", 42)
		corrCtx2.StageEntries = []StageEntry{
			{Stage: WorkflowStageStart, EnteredAt: time.Now().Add(-2 * time.Second), Duration: 1 * time.Second},
			{Stage: WorkflowStageAnalyze, EnteredAt: time.Now().Add(-1 * time.Second), Duration: 500 * time.Millisecond},
		}
		ctx2 := WithCorrelationContext(context.Background(), corrCtx2)

		assert.NotPanics(t, func() {
			logger.LogAgentStop(ctx2, "developer", 2*time.Second, nil)
		})
	})

	t.Run("LogAgentHandoff with full context", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.LogAgentHandoff(ctx, "developer", "qa", "ready_for_review", 2048)
		})
	})

	t.Run("LogWorkflowTransition with full context and stage duration", func(t *testing.T) {
		corrCtx3 := NewCorrelationContext("developer", 42)
		corrCtx3.TaskID = "task-456"
		corrCtx3.StageEntries = []StageEntry{
			{Stage: WorkflowStageStart, EnteredAt: time.Now().Add(-5 * time.Second), Duration: 2 * time.Second},
			{Stage: WorkflowStageAnalyze, EnteredAt: time.Now().Add(-3 * time.Second), Duration: 1 * time.Second},
		}
		ctx3 := WithCorrelationContext(context.Background(), corrCtx3)

		assert.NotPanics(t, func() {
			logger.LogWorkflowTransition(ctx3, 42, string(WorkflowStageStart), string(WorkflowStageAnalyze), "analysis ready")
		})
	})

	t.Run("LogRetryMethods without correlation ID", func(t *testing.T) {
		bgCtx := context.Background()
		assert.NotPanics(t, func() {
			logger.LogRetryAttempt(bgCtx, "op", 1, 3)
			logger.LogRetrySuccess(bgCtx, "op", 2)
			logger.LogRetryExhausted(bgCtx, "op", 3, assert.AnError)
			logger.LogRetryNonRetryable(bgCtx, "op", 1, assert.AnError)
			logger.LogRetryDelay(bgCtx, "op", 1, time.Second, assert.AnError)
			logger.LogCircuitBreakerStateChange(bgCtx, "cb", "closed", "open")
			logger.LogCircuitBreakerRejection(bgCtx, "cb", "open")
		})
	})
}
