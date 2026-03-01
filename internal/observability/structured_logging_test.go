package observability

import (
	"context"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelationContext(t *testing.T) {
	t.Run("NewCorrelationContext creates valid context", func(t *testing.T) {
		corrCtx := NewCorrelationContext("developer", 123)
		
		assert.NotEmpty(t, corrCtx.CorrelationID)
		assert.Equal(t, "developer", corrCtx.AgentType)
		assert.Equal(t, 123, corrCtx.IssueID)
		assert.Equal(t, WorkflowStageStart, corrCtx.WorkflowStage)
		assert.NotZero(t, corrCtx.CreatedAt)
		assert.NotZero(t, corrCtx.StartTime)
		assert.Len(t, corrCtx.StageEntries, 1)
		assert.Equal(t, WorkflowStageStart, corrCtx.StageEntries[0].Stage)
		assert.NotZero(t, corrCtx.StageEntries[0].EnteredAt)
	})
	
	t.Run("WithCorrelationContext and GetCorrelationContext", func(t *testing.T) {
		ctx := context.Background()
		corrCtx := NewCorrelationContext("qa", 456)
		
		// Initially, no correlation context
		assert.Nil(t, GetCorrelationContext(ctx))
		
		// Add correlation context
		ctx = WithCorrelationContext(ctx, corrCtx)
		retrieved := GetCorrelationContext(ctx)
		
		require.NotNil(t, retrieved)
		assert.Equal(t, corrCtx.CorrelationID, retrieved.CorrelationID)
		assert.Equal(t, "qa", retrieved.AgentType)
		assert.Equal(t, 456, retrieved.IssueID)
		
		// Should also set simple correlation ID for backward compatibility
		assert.Equal(t, corrCtx.CorrelationID, GetCorrelationID(ctx))
	})
	
	t.Run("EnsureCorrelationContext creates when missing", func(t *testing.T) {
		ctx := context.Background()
		
		// Initially no correlation context
		assert.Nil(t, GetCorrelationContext(ctx))
		
		// EnsureCorrelationContext should create one
		ctx = EnsureCorrelationContext(ctx, "test-agent", 789)
		corrCtx := GetCorrelationContext(ctx)
		
		require.NotNil(t, corrCtx)
		assert.Equal(t, "test-agent", corrCtx.AgentType)
		assert.Equal(t, 789, corrCtx.IssueID)
		assert.NotEmpty(t, corrCtx.CorrelationID)
	})
	
	t.Run("EnsureCorrelationContext preserves existing", func(t *testing.T) {
		ctx := context.Background()
		original := NewCorrelationContext("original", 100)
		
		// Set existing correlation context
		ctx = WithCorrelationContext(ctx, original)
		
		// EnsureCorrelationContext should not change it
		ctx = EnsureCorrelationContext(ctx, "different", 200)
		corrCtx := GetCorrelationContext(ctx)
		
		require.NotNil(t, corrCtx)
		assert.Equal(t, original.CorrelationID, corrCtx.CorrelationID)
		assert.Equal(t, "original", corrCtx.AgentType)
		assert.Equal(t, 100, corrCtx.IssueID)
	})
	
	t.Run("EnsureCorrelationContext reuses existing correlation ID", func(t *testing.T) {
		ctx := context.Background()
		existingID := "existing-correlation-id"
		
		// Set only a simple correlation ID
		ctx = WithCorrelationID(ctx, existingID)
		
		// EnsureCorrelationContext should reuse the existing ID
		ctx = EnsureCorrelationContext(ctx, "new-agent", 300)
		corrCtx := GetCorrelationContext(ctx)
		
		require.NotNil(t, corrCtx)
		assert.Equal(t, existingID, corrCtx.CorrelationID)
		assert.Equal(t, "new-agent", corrCtx.AgentType)
		assert.Equal(t, 300, corrCtx.IssueID)
	})
}

func TestWorkflowStageTracking(t *testing.T) {
	t.Run("WithWorkflowStage updates stage and timing", func(t *testing.T) {
		ctx := context.Background()
		corrCtx := NewCorrelationContext("developer", 123)
		ctx = WithCorrelationContext(ctx, corrCtx)
		
		// Start with initial stage
		assert.Equal(t, WorkflowStageStart, corrCtx.WorkflowStage)
		assert.Len(t, corrCtx.StageEntries, 1)
		
		// Allow some time to pass
		time.Sleep(10 * time.Millisecond)
		
		// Transition to new stage
		ctx = WithWorkflowStage(ctx, WorkflowStageAnalyze)
		updatedCtx := GetCorrelationContext(ctx)
		
		require.NotNil(t, updatedCtx)
		assert.Equal(t, WorkflowStageAnalyze, updatedCtx.WorkflowStage)
		assert.Len(t, updatedCtx.StageEntries, 2)
		
		// First stage should have duration set
		firstStage := updatedCtx.StageEntries[0]
		assert.Equal(t, WorkflowStageStart, firstStage.Stage)
		assert.True(t, firstStage.Duration > 0)
		
		// Second stage should be current
		secondStage := updatedCtx.StageEntries[1]
		assert.Equal(t, WorkflowStageAnalyze, secondStage.Stage)
		assert.Zero(t, secondStage.Duration) // Current stage has no duration yet
	})
	
	t.Run("GetStageDuration returns correct duration", func(t *testing.T) {
		corrCtx := NewCorrelationContext("developer", 123)
		
		// Simulate stage transitions with timing
		start := time.Now()
		corrCtx.StageEntries = []StageEntry{
			{Stage: WorkflowStageStart, EnteredAt: start, Duration: 100 * time.Millisecond},
			{Stage: WorkflowStageAnalyze, EnteredAt: start.Add(100 * time.Millisecond), Duration: 200 * time.Millisecond},
		}
		
		assert.Equal(t, 100*time.Millisecond, corrCtx.GetStageDuration(WorkflowStageStart))
		assert.Equal(t, 200*time.Millisecond, corrCtx.GetStageDuration(WorkflowStageAnalyze))
		assert.Zero(t, corrCtx.GetStageDuration(WorkflowStageImplement)) // Not present
	})
	
	t.Run("IsCurrentStage checks current stage", func(t *testing.T) {
		corrCtx := NewCorrelationContext("developer", 123)
		corrCtx.WorkflowStage = WorkflowStageAnalyze
		
		assert.True(t, corrCtx.IsCurrentStage(WorkflowStageAnalyze))
		assert.False(t, corrCtx.IsCurrentStage(WorkflowStageStart))
		assert.False(t, corrCtx.IsCurrentStage(WorkflowStageImplement))
	})
}

func TestHandoffTracking(t *testing.T) {
	t.Run("WithHandoff records handoff information", func(t *testing.T) {
		ctx := context.Background()
		corrCtx := NewCorrelationContext("developer", 123)
		ctx = WithCorrelationContext(ctx, corrCtx)
		
		// Record handoff
		ctx = WithHandoff(ctx, "developer", "qa", "ready_for_review", 1024)
		updatedCtx := GetCorrelationContext(ctx)
		
		require.NotNil(t, updatedCtx)
		assert.Equal(t, "qa", updatedCtx.AgentType) // Agent type should be updated
		assert.Len(t, updatedCtx.HandoffChain, 1)
		
		handoff := updatedCtx.HandoffChain[0]
		assert.Equal(t, "developer", handoff.FromAgent)
		assert.Equal(t, "qa", handoff.ToAgent)
		assert.Equal(t, "ready_for_review", handoff.Reason)
		assert.Equal(t, 1024, handoff.PayloadSize)
		assert.NotZero(t, handoff.Timestamp)
	})
	
	t.Run("Multiple handoffs create chain", func(t *testing.T) {
		ctx := context.Background()
		corrCtx := NewCorrelationContext("orchestrator", 123)
		ctx = WithCorrelationContext(ctx, corrCtx)
		
		// Chain of handoffs
		ctx = WithHandoff(ctx, "orchestrator", "developer", "new_issue", 512)
		ctx = WithHandoff(ctx, "developer", "qa", "implementation_complete", 1024)
		ctx = WithHandoff(ctx, "qa", "human", "review_required", 256)
		
		updatedCtx := GetCorrelationContext(ctx)
		require.NotNil(t, updatedCtx)
		assert.Equal(t, "human", updatedCtx.AgentType)
		assert.Len(t, updatedCtx.HandoffChain, 3)
		assert.Equal(t, 3, updatedCtx.GetHandoffCount())
		
		// Verify chain order
		chain := updatedCtx.HandoffChain
		assert.Equal(t, "orchestrator->developer", chain[0].FromAgent+"->"+chain[0].ToAgent)
		assert.Equal(t, "developer->qa", chain[1].FromAgent+"->"+chain[1].ToAgent)
		assert.Equal(t, "qa->human", chain[2].FromAgent+"->"+chain[2].ToAgent)
	})
}

func TestMetadataHandling(t *testing.T) {
	t.Run("WithMetadata adds metadata to context", func(t *testing.T) {
		ctx := context.Background()
		corrCtx := NewCorrelationContext("developer", 123)
		ctx = WithCorrelationContext(ctx, corrCtx)
		
		// Add metadata
		ctx = WithMetadata(ctx, "issue_title", "Fix logging bug")
		ctx = WithMetadata(ctx, "priority", "high")
		
		updatedCtx := GetCorrelationContext(ctx)
		require.NotNil(t, updatedCtx)
		require.NotNil(t, updatedCtx.Metadata)
		
		assert.Equal(t, "Fix logging bug", updatedCtx.Metadata["issue_title"])
		assert.Equal(t, "high", updatedCtx.Metadata["priority"])
	})
	
	t.Run("WithMetadata handles nil context", func(t *testing.T) {
		ctx := context.Background()
		
		// Should not panic with nil correlation context
		assert.NotPanics(t, func() {
			WithMetadata(ctx, "key", "value")
		})
	})
}

func TestWorkflowDuration(t *testing.T) {
	t.Run("GetWorkflowDuration calculates correctly", func(t *testing.T) {
		start := time.Now().Add(-5 * time.Minute)
		corrCtx := &CorrelationContext{
			CorrelationID: "test-123",
			StartTime:     start,
		}
		
		duration := corrCtx.GetWorkflowDuration()
		
		// Should be approximately 5 minutes (allowing for test execution time)
		assert.True(t, duration >= 4*time.Minute)
		assert.True(t, duration <= 6*time.Minute)
	})
	
	t.Run("GetWorkflowDuration returns zero for unset start time", func(t *testing.T) {
		corrCtx := &CorrelationContext{
			CorrelationID: "test-123",
			// StartTime is zero
		}
		
		duration := corrCtx.GetWorkflowDuration()
		assert.Zero(t, duration)
	})
}

func TestStructuredLoggingIntegration(t *testing.T) {
	t.Run("Enhanced structured logging with correlation context", func(t *testing.T) {
		cfg := config.LoggingConfig{
			Level: "info",
			StructuredLogging: config.StructuredLoggingConfig{
				Enabled: true,
				Format:  "json",
				Correlation: config.CorrelationConfig{
					Enabled:              true,
					IncludeWorkflowStage: true,
					IncludeAgentMetadata: true,
				},
			},
		}
		
		logger := NewStructuredLogger(cfg)
		require.NotNil(t, logger)
		
		// Create context with correlation context
		ctx := context.Background()
		corrCtx := NewCorrelationContext("test-agent", 123)
		corrCtx.WorkflowStage = WorkflowStageAnalyze
		corrCtx.Metadata["test_key"] = "test_value"
		ctx = WithCorrelationContext(ctx, corrCtx)
		
		// These should not panic and should include correlation context
		assert.NotPanics(t, func() {
			logger.LogAgentStart(ctx, "test-agent", "testing structured logging")
		})
		
		assert.NotPanics(t, func() {
			logger.LogWorkflowTransition(ctx, 123, "start", "analyze", "test transition")
		})
		
		assert.NotPanics(t, func() {
			logger.LogAgentHandoff(ctx, "agent1", "agent2", "test", 512)
		})
		
		assert.NotPanics(t, func() {
			logger.LogAgentStop(ctx, "test-agent", 5*time.Second, nil)
		})
	})
}

func TestWorkflowStageConstants(t *testing.T) {
	// Ensure all workflow stage constants are properly defined
	stages := []WorkflowStage{
		WorkflowStageStart,
		WorkflowStageClaim,
		WorkflowStageAnalyze,
		WorkflowStageDecompose,
		WorkflowStageImplement,
		WorkflowStageCommit,
		WorkflowStagePR,
		WorkflowStageReview,
		WorkflowStageComplete,
		WorkflowStageHandoff,
		WorkflowStageIdle,
		WorkflowStageError,
	}
	
	for _, stage := range stages {
		assert.NotEmpty(t, string(stage), "Stage should have non-empty string value")
		assert.True(t, len(string(stage)) > 0, "Stage should have meaningful string representation")
	}
}