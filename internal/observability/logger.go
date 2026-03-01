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
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Add timestamp formatting and other structured enhancements
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   "timestamp",
					Value: slog.StringValue(a.Value.Time().Format(time.RFC3339Nano)),
				}
			}
			return a
		},
	}

	// Use JSON handler for structured output
	handler := slog.NewJSONHandler(os.Stderr, opts)
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
	sl.WithCorrelation(ctx).Info("agent_start",
		"agent_type", agentType,
		"message", message,
		"timestamp", time.Now(),
	)
}

// LogAgentStop logs the stop of an agent with timing and context
func (sl *StructuredLogger) LogAgentStop(ctx context.Context, agentType string, duration time.Duration, err error) {
	logger := sl.WithCorrelation(ctx).With(
		"agent_type", agentType,
		"duration_ms", duration.Milliseconds(),
		"timestamp", time.Now(),
	)

	if err != nil {
		logger.Error("agent_stop", "error", err.Error())
	} else {
		logger.Info("agent_stop", "status", "success")
	}
}

// LogAgentHandoff logs agent handoffs with context and payload information
func (sl *StructuredLogger) LogAgentHandoff(ctx context.Context, fromAgent, toAgent, trigger string, payloadSize int) {
	sl.WithCorrelation(ctx).Info("agent_handoff",
		"from_agent", fromAgent,
		"to_agent", toAgent,
		"trigger", trigger,
		"payload_size_bytes", payloadSize,
		"timestamp", time.Now(),
	)
}

// LogWorkflowTransition logs workflow state changes
func (sl *StructuredLogger) LogWorkflowTransition(ctx context.Context, issueID int, fromState, toState, reason string) {
	sl.WithCorrelation(ctx).Info("workflow_transition",
		"issue_id", issueID,
		"from_state", fromState,
		"to_state", toState,
		"reason", reason,
		"timestamp", time.Now(),
	)
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