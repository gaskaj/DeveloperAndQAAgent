package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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