package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrelationID(t *testing.T) {
	t.Run("NewCorrelationID generates unique IDs", func(t *testing.T) {
		id1 := NewCorrelationID()
		id2 := NewCorrelationID()

		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2)

		// Should be hex-encoded 8-byte string (16 characters)
		assert.Len(t, id1, 16)
		assert.Len(t, id2, 16)
	})

	t.Run("WithCorrelationID and GetCorrelationID", func(t *testing.T) {
		ctx := context.Background()
		testID := "test-correlation-123"

		// Initially, no correlation ID
		assert.Empty(t, GetCorrelationID(ctx))

		// Add correlation ID
		ctx = WithCorrelationID(ctx, testID)
		assert.Equal(t, testID, GetCorrelationID(ctx))
	})

	t.Run("EnsureCorrelationID creates when missing", func(t *testing.T) {
		ctx := context.Background()

		// Initially no correlation ID
		assert.Empty(t, GetCorrelationID(ctx))

		// EnsureCorrelationID should create one
		ctx = EnsureCorrelationID(ctx)
		corrID := GetCorrelationID(ctx)

		assert.NotEmpty(t, corrID)
		assert.Len(t, corrID, 16) // Should be 16-char hex string
	})

	t.Run("EnsureCorrelationID preserves existing", func(t *testing.T) {
		ctx := context.Background()
		existingID := "existing-correlation-id"

		// Set an existing correlation ID
		ctx = WithCorrelationID(ctx, existingID)

		// EnsureCorrelationID should not change it
		ctx = EnsureCorrelationID(ctx)

		assert.Equal(t, existingID, GetCorrelationID(ctx))
	})

	t.Run("GetCorrelationID with wrong type in context", func(t *testing.T) {
		// Create context with wrong type value
		ctx := context.WithValue(context.Background(), CorrelationKey{}, 12345)

		// Should return empty string when type assertion fails
		assert.Empty(t, GetCorrelationID(ctx))
	})
}

func TestCorrelationIDFormat(t *testing.T) {
	id := NewCorrelationID()

	// Should only contain hex characters
	for _, char := range id {
		assert.True(t,
			(char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
			"Correlation ID should only contain hex characters, found: %c", char)
	}
}

func TestWithWorkflowStage_NilContext(t *testing.T) {
	ctx := context.Background()
	// No correlation context set - should return ctx unchanged
	result := WithWorkflowStage(ctx, WorkflowStageAnalyze)
	assert.Nil(t, GetCorrelationContext(result))
}

func TestWithHandoff_NilContext(t *testing.T) {
	ctx := context.Background()
	// No correlation context set - should return ctx unchanged
	result := WithHandoff(ctx, "a", "b", "reason", 100)
	assert.Nil(t, GetCorrelationContext(result))
}

func TestWithMetadata_NilMetadataMap(t *testing.T) {
	// Create a context with correlation context that has nil metadata
	corrCtx := &CorrelationContext{
		CorrelationID: "test-nil-meta",
		Metadata:      nil,
	}
	ctx := WithCorrelationContext(context.Background(), corrCtx)

	// WithMetadata should initialize the map
	ctx = WithMetadata(ctx, "key", "value")
	retrieved := GetCorrelationContext(ctx)
	require.NotNil(t, retrieved)
	assert.Equal(t, "value", retrieved.Metadata["key"])
}

func TestGetCorrelationID_FallsBackToCorrelationContext(t *testing.T) {
	corrCtx := &CorrelationContext{
		CorrelationID: "fallback-id",
	}
	// Only set CorrelationContextKey, not CorrelationKey
	ctx := context.WithValue(context.Background(), CorrelationContextKey{}, corrCtx)

	id := GetCorrelationID(ctx)
	assert.Equal(t, "fallback-id", id)
}

func TestGetMetadataCopy_NilMetadata(t *testing.T) {
	corrCtx := &CorrelationContext{
		CorrelationID: "test",
		Metadata:      nil,
	}
	result := corrCtx.GetMetadataCopy()
	assert.Nil(t, result)
}

func TestGetStageEntriesCopy_Empty(t *testing.T) {
	corrCtx := &CorrelationContext{
		CorrelationID: "test",
		StageEntries:  nil,
	}
	result := corrCtx.GetStageEntriesCopy()
	assert.Empty(t, result)
}

func TestGetHandoffChainCopy_Empty(t *testing.T) {
	corrCtx := &CorrelationContext{
		CorrelationID: "test",
		HandoffChain:  nil,
	}
	result := corrCtx.GetHandoffChainCopy()
	assert.Empty(t, result)
}

func TestGetStageDuration_CurrentStage(t *testing.T) {
	// Test the path where Duration is 0 (current active stage)
	corrCtx := &CorrelationContext{
		CorrelationID: "test",
		StageEntries: []StageEntry{
			{Stage: WorkflowStageAnalyze, EnteredAt: time.Now().Add(-1 * time.Second), Duration: 0},
		},
	}
	duration := corrCtx.GetStageDuration(WorkflowStageAnalyze)
	assert.True(t, duration > 0, "should calculate duration for active stage")
}

func TestGetAgentType(t *testing.T) {
	corrCtx := NewCorrelationContext("developer", 42)
	assert.Equal(t, "developer", corrCtx.GetAgentType())
}

func TestGetCurrentWorkflowStage(t *testing.T) {
	corrCtx := NewCorrelationContext("developer", 42)
	assert.Equal(t, WorkflowStageStart, corrCtx.GetCurrentWorkflowStage())
}
