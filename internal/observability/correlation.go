package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// CorrelationKey is the context key for correlation IDs
type CorrelationKey struct{}

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, corrID string) context.Context {
	return context.WithValue(ctx, CorrelationKey{}, corrID)
}

// GetCorrelationID retrieves the correlation ID from the context
func GetCorrelationID(ctx context.Context) string {
	if corrID, ok := ctx.Value(CorrelationKey{}).(string); ok {
		return corrID
	}
	return ""
}

// NewCorrelationID generates a new random correlation ID
func NewCorrelationID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple counter-based ID if random fails
		return "unknown"
	}
	return hex.EncodeToString(bytes)
}

// EnsureCorrelationID ensures the context has a correlation ID, creating one if needed
func EnsureCorrelationID(ctx context.Context) context.Context {
	if GetCorrelationID(ctx) == "" {
		return WithCorrelationID(ctx, NewCorrelationID())
	}
	return ctx
}