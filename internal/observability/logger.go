package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
)

// LogLevel represents structured log levels
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// StructuredLogger provides enhanced structured logging capabilities
type StructuredLogger struct {
	*slog.Logger
	correlationIDKey string
}

// NewStructuredLogger creates a new structured logger with the given configuration
func NewStructuredLogger(cfg config.LoggingConfig) *StructuredLogger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.StructuredLogging.IncludeCaller,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Add timestamp formatting and other structured enhancements
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   "timestamp",
					Value: slog.StringValue(a.Value.Time().Format(time.RFC3339Nano)),
				}
			}
			// Map log level to structured format if configured
			if a.Key == slog.LevelKey && cfg.StructuredLogging.Export.Enabled {
				if fieldMappings, ok := cfg.StructuredLogging.Export.FieldMappings["generic"]; ok {
					if levelField, ok := fieldMappings["level"]; ok {
						return slog.Attr{
							Key:   levelField,
							Value: a.Value,
						}
					}
				}
			}
			return a
		},
	}

	var handler slog.Handler

	// Choose handler based on format configuration
	switch cfg.StructuredLogging.Format {
	case "structured_text":
		handler = slog.NewTextHandler(os.Stderr, opts)
	case "development":
		// Use text handler with pretty printing for development
		handler = slog.NewTextHandler(os.Stderr, opts)
	default:
		// Default to JSON for structured logging
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	logger := slog.New(handler)

	return &StructuredLogger{
		Logger:           logger,
		correlationIDKey: "correlation_id",
	}
}

// WithCorrelation adds a correlation ID to the logger context
func (sl *StructuredLogger) WithCorrelation(ctx context.Context) *slog.Logger {
	if corrID := GetCorrelationID(ctx); corrID != "" {
		return sl.Logger.With(sl.correlationIDKey, corrID)
	}
	return sl.Logger
}

// LogAgentStart logs the start of an agent with timing and context
func (sl *StructuredLogger) LogAgentStart(ctx context.Context, agentType, message string) {
	attrs := []slog.Attr{
		slog.String("event_type", "agent_start"),
		slog.String("agent_type", agentType),
		slog.String("message", message),
		slog.Time("timestamp", time.Now()),
	}

	// Add enriched correlation context if available
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		attrs = append(attrs,
			slog.Int("issue_id", corrCtx.IssueID),
			slog.String("workflow_stage", string(corrCtx.GetCurrentWorkflowStage())),
			slog.String("task_id", corrCtx.TaskID),
			slog.Int("handoff_count", corrCtx.GetHandoffCount()),
		)

		// Add metadata as structured attributes
		for k, v := range corrCtx.GetMetadataCopy() {
			attrs = append(attrs, slog.String("meta_"+k, v))
		}
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), slog.LevelInfo, "agent_lifecycle", attrs...)
}

// LogAgentStop logs the stop of an agent with timing and context
func (sl *StructuredLogger) LogAgentStop(ctx context.Context, agentType string, duration time.Duration, err error) {
	attrs := []slog.Attr{
		slog.String("event_type", "agent_stop"),
		slog.String("agent_type", agentType),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.Time("timestamp", time.Now()),
	}

	// Add enriched correlation context if available
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		attrs = append(attrs,
			slog.Int("issue_id", corrCtx.IssueID),
			slog.String("workflow_stage", string(corrCtx.GetCurrentWorkflowStage())),
			slog.String("task_id", corrCtx.TaskID),
			slog.Int("handoff_count", corrCtx.GetHandoffCount()),
			slog.Int64("total_workflow_duration_ms", corrCtx.GetWorkflowDuration().Milliseconds()),
		)

		// Add stage timing breakdown
		for _, entry := range corrCtx.GetStageEntriesCopy() {
			if entry.Duration > 0 {
				attrs = append(attrs, slog.Int64("stage_"+string(entry.Stage)+"_duration_ms", entry.Duration.Milliseconds()))
			}
		}
	}

	level := slog.LevelInfo
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		level = slog.LevelError
	} else {
		attrs = append(attrs, slog.String("status", "success"))
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), level, "agent_lifecycle", attrs...)
}

// LogAgentHandoff logs agent handoffs with context and payload information
func (sl *StructuredLogger) LogAgentHandoff(ctx context.Context, fromAgent, toAgent, trigger string, payloadSize int) {
	attrs := []slog.Attr{
		slog.String("event_type", "agent_handoff"),
		slog.String("from_agent", fromAgent),
		slog.String("to_agent", toAgent),
		slog.String("trigger", trigger),
		slog.Int("payload_size_bytes", payloadSize),
		slog.Time("timestamp", time.Now()),
	}

	// Add enriched correlation context if available
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		attrs = append(attrs,
			slog.Int("issue_id", corrCtx.IssueID),
			slog.String("current_workflow_stage", string(corrCtx.GetCurrentWorkflowStage())),
			slog.String("task_id", corrCtx.TaskID),
			slog.Int("handoff_sequence", corrCtx.GetHandoffCount()),
			slog.Int64("workflow_duration_ms", corrCtx.GetWorkflowDuration().Milliseconds()),
		)

		// Add handoff chain context for traceability
		if chain := corrCtx.GetHandoffChainCopy(); len(chain) > 0 {
			handoffChain := make([]string, len(chain))
			for i, h := range chain {
				handoffChain[i] = h.FromAgent + "->" + h.ToAgent
			}
			attrs = append(attrs, slog.Any("previous_handoffs", handoffChain))
		}
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), slog.LevelInfo, "agent_handoff", attrs...)
}

// LogWorkflowTransition logs workflow state changes
func (sl *StructuredLogger) LogWorkflowTransition(ctx context.Context, issueID int, fromState, toState, reason string) {
	attrs := []slog.Attr{
		slog.String("event_type", "workflow_transition"),
		slog.Int("issue_id", issueID),
		slog.String("from_state", fromState),
		slog.String("to_state", toState),
		slog.String("reason", reason),
		slog.Time("timestamp", time.Now()),
	}

	// Add enriched correlation context if available
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		attrs = append(attrs,
			slog.String("agent_type", corrCtx.GetAgentType()),
			slog.String("task_id", corrCtx.TaskID),
			slog.Int("handoff_count", corrCtx.GetHandoffCount()),
			slog.Int64("workflow_duration_ms", corrCtx.GetWorkflowDuration().Milliseconds()),
		)

		// Add stage timing if transitioning stages
		if fromStage := WorkflowStage(fromState); fromStage != "" {
			stageDuration := corrCtx.GetStageDuration(fromStage)
			if stageDuration > 0 {
				attrs = append(attrs, slog.Int64("stage_duration_ms", stageDuration.Milliseconds()))
			}
		}

		// Add workflow stage performance summary
		stageEntries := corrCtx.GetStageEntriesCopy()
		totalStages := len(stageEntries)
		completedStages := 0
		for _, entry := range stageEntries {
			if entry.Duration > 0 {
				completedStages++
			}
		}

		attrs = append(attrs,
			slog.Int("total_stages", totalStages),
			slog.Int("completed_stages", completedStages),
		)
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), slog.LevelInfo, "workflow_transition", attrs...)
}

// LogToolUsage logs tool usage with success/failure tracking
func (sl *StructuredLogger) LogToolUsage(ctx context.Context, toolName string, duration time.Duration, success bool, err error) {
	logger := sl.WithCorrelation(ctx).With(
		"tool_name", toolName,
		"duration_ms", duration.Milliseconds(),
		"success", success,
		"timestamp", time.Now(),
	)

	if err != nil {
		logger.Error("tool_usage", "error", err.Error())
	} else {
		logger.Info("tool_usage", "status", "completed")
	}
}

// LogLLMCall logs LLM API calls with timing and token usage
func (sl *StructuredLogger) LogLLMCall(ctx context.Context, model string, inputTokens, outputTokens int, duration time.Duration, err error) {
	logger := sl.WithCorrelation(ctx).With(
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"total_tokens", inputTokens+outputTokens,
		"duration_ms", duration.Milliseconds(),
		"timestamp", time.Now(),
	)

	if err != nil {
		logger.Error("llm_call", "error", err.Error())
	} else {
		logger.Info("llm_call", "status", "success")
	}
}

// LogDecisionPoint logs agent decision points with reasoning context
func (sl *StructuredLogger) LogDecisionPoint(ctx context.Context, agentType, decision, reasoning string, metadata map[string]interface{}) {
	attrs := []slog.Attr{
		slog.String("agent_type", agentType),
		slog.String("decision", decision),
		slog.String("reasoning", reasoning),
		slog.Time("timestamp", time.Now()),
	}

	// Add metadata as attributes
	for k, v := range metadata {
		attrs = append(attrs, slog.Any(k, v))
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), slog.LevelInfo, "decision_point", attrs...)
}

// LogPerformanceMetric logs performance-related metrics
func (sl *StructuredLogger) LogPerformanceMetric(ctx context.Context, metricName string, value float64, unit string, labels map[string]string) {
	attrs := []slog.Attr{
		slog.String("metric_name", metricName),
		slog.Float64("value", value),
		slog.String("unit", unit),
		slog.Time("timestamp", time.Now()),
	}

	// Add labels as attributes
	for k, v := range labels {
		attrs = append(attrs, slog.String("label_"+k, v))
	}

	sl.WithCorrelation(ctx).LogAttrs(context.Background(), slog.LevelInfo, "performance_metric", attrs...)
}

// LogRetryAttempt logs a retry attempt with context
func (sl *StructuredLogger) LogRetryAttempt(ctx context.Context, operation string, attempt, maxAttempts int) {
	sl.WithCorrelation(ctx).Debug("retry_attempt",
		"operation", operation,
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"timestamp", time.Now(),
	)
}

// LogRetrySuccess logs a successful operation after retries
func (sl *StructuredLogger) LogRetrySuccess(ctx context.Context, operation string, attempts int) {
	sl.WithCorrelation(ctx).Info("retry_success",
		"operation", operation,
		"attempts", attempts,
		"timestamp", time.Now(),
	)
}

// LogRetryExhausted logs when all retry attempts have been exhausted
func (sl *StructuredLogger) LogRetryExhausted(ctx context.Context, operation string, attempts int, err error) {
	sl.WithCorrelation(ctx).Error("retry_exhausted",
		"operation", operation,
		"attempts", attempts,
		"error", err.Error(),
		"timestamp", time.Now(),
	)
}

// LogRetryNonRetryable logs when an operation fails with a non-retryable error
func (sl *StructuredLogger) LogRetryNonRetryable(ctx context.Context, operation string, attempt int, err error) {
	sl.WithCorrelation(ctx).Warn("retry_non_retryable",
		"operation", operation,
		"attempt", attempt,
		"error", err.Error(),
		"timestamp", time.Now(),
	)
}

// LogRetryDelay logs the delay before a retry attempt
func (sl *StructuredLogger) LogRetryDelay(ctx context.Context, operation string, attempt int, delay time.Duration, err error) {
	sl.WithCorrelation(ctx).Debug("retry_delay",
		"operation", operation,
		"attempt", attempt,
		"delay_ms", delay.Milliseconds(),
		"error", err.Error(),
		"timestamp", time.Now(),
	)
}

// LogCircuitBreakerStateChange logs circuit breaker state transitions
func (sl *StructuredLogger) LogCircuitBreakerStateChange(ctx context.Context, name, fromState, toState string) {
	sl.WithCorrelation(ctx).Info("circuit_breaker_state_change",
		"name", name,
		"from_state", fromState,
		"to_state", toState,
		"timestamp", time.Now(),
	)
}

// LogCircuitBreakerRejection logs when a circuit breaker rejects a request
func (sl *StructuredLogger) LogCircuitBreakerRejection(ctx context.Context, name, state string) {
	sl.WithCorrelation(ctx).Warn("circuit_breaker_rejection",
		"name", name,
		"state", state,
		"timestamp", time.Now(),
	)
}
