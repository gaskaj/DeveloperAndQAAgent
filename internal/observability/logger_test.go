package observability

import (
	"context"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
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