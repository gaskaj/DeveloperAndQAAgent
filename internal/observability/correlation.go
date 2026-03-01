package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// CorrelationKey is the context key for correlation IDs
type CorrelationKey struct{}

// CorrelationContextKey is the context key for correlation context
type CorrelationContextKey struct{}

// WorkflowStage represents different stages in the agent workflow
type WorkflowStage string

const (
	WorkflowStageStart       WorkflowStage = "start"
	WorkflowStageClaim       WorkflowStage = "claim"
	WorkflowStageAnalyze     WorkflowStage = "analyze"
	WorkflowStageDecompose   WorkflowStage = "decompose"
	WorkflowStageImplement   WorkflowStage = "implement"
	WorkflowStageCommit      WorkflowStage = "commit"
	WorkflowStagePR          WorkflowStage = "pr"
	WorkflowStageReview      WorkflowStage = "review"
	WorkflowStageComplete    WorkflowStage = "complete"
	WorkflowStageHandoff     WorkflowStage = "handoff"
	WorkflowStageIdle        WorkflowStage = "idle"
	WorkflowStageError       WorkflowStage = "error"
)

// CorrelationContext holds enriched context for multi-agent traceability
type CorrelationContext struct {
	// Core identification
	CorrelationID string    `json:"correlation_id"`
	CreatedAt     time.Time `json:"created_at"`
	
	// Agent context
	AgentType     string `json:"agent_type"`
	AgentInstance string `json:"agent_instance,omitempty"`
	
	// Workflow context  
	WorkflowStage WorkflowStage `json:"workflow_stage"`
	IssueID       int           `json:"issue_id,omitempty"`
	TaskID        string        `json:"task_id,omitempty"`
	
	// Tracing context
	ParentCorrelationID string            `json:"parent_correlation_id,omitempty"`
	HandoffChain        []HandoffInfo     `json:"handoff_chain,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	
	// Performance context
	StartTime    time.Time     `json:"start_time,omitempty"`
	StageEntries []StageEntry  `json:"stage_entries,omitempty"`
}

// HandoffInfo tracks agent-to-agent handoffs
type HandoffInfo struct {
	FromAgent   string    `json:"from_agent"`
	ToAgent     string    `json:"to_agent"`
	Timestamp   time.Time `json:"timestamp"`
	Reason      string    `json:"reason"`
	PayloadSize int       `json:"payload_size,omitempty"`
}

// StageEntry tracks workflow stage transitions  
type StageEntry struct {
	Stage     WorkflowStage `json:"stage"`
	EnteredAt time.Time     `json:"entered_at"`
	Duration  time.Duration `json:"duration,omitempty"`
}

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, corrID string) context.Context {
	return context.WithValue(ctx, CorrelationKey{}, corrID)
}

// GetCorrelationID retrieves the correlation ID from the context
func GetCorrelationID(ctx context.Context) string {
	if corrID, ok := ctx.Value(CorrelationKey{}).(string); ok {
		return corrID
	}
	// Fall back to correlation context if available
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		return corrCtx.CorrelationID
	}
	return ""
}

// WithCorrelationContext adds enriched correlation context to the context
func WithCorrelationContext(ctx context.Context, corrCtx *CorrelationContext) context.Context {
	// Also set the simple correlation ID for backward compatibility
	ctx = WithCorrelationID(ctx, corrCtx.CorrelationID)
	return context.WithValue(ctx, CorrelationContextKey{}, corrCtx)
}

// GetCorrelationContext retrieves the correlation context from the context
func GetCorrelationContext(ctx context.Context) *CorrelationContext {
	if corrCtx, ok := ctx.Value(CorrelationContextKey{}).(*CorrelationContext); ok {
		return corrCtx
	}
	return nil
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

// NewCorrelationContext creates a new correlation context with the given parameters
func NewCorrelationContext(agentType string, issueID int) *CorrelationContext {
	return &CorrelationContext{
		CorrelationID:  NewCorrelationID(),
		CreatedAt:      time.Now(),
		AgentType:      agentType,
		WorkflowStage:  WorkflowStageStart,
		IssueID:        issueID,
		StartTime:      time.Now(),
		Metadata:       make(map[string]string),
		StageEntries:   []StageEntry{{Stage: WorkflowStageStart, EnteredAt: time.Now()}},
	}
}

// EnsureCorrelationID ensures the context has a correlation ID, creating one if needed
func EnsureCorrelationID(ctx context.Context) context.Context {
	if GetCorrelationID(ctx) == "" {
		return WithCorrelationID(ctx, NewCorrelationID())
	}
	return ctx
}

// EnsureCorrelationContext ensures the context has correlation context, creating one if needed
func EnsureCorrelationContext(ctx context.Context, agentType string, issueID int) context.Context {
	if corrCtx := GetCorrelationContext(ctx); corrCtx != nil {
		return ctx
	}
	
	// Check if we at least have a correlation ID to reuse
	if existingID := GetCorrelationID(ctx); existingID != "" {
		corrCtx := NewCorrelationContext(agentType, issueID)
		corrCtx.CorrelationID = existingID
		return WithCorrelationContext(ctx, corrCtx)
	}
	
	// Create completely new correlation context
	corrCtx := NewCorrelationContext(agentType, issueID)
	return WithCorrelationContext(ctx, corrCtx)
}

// WithWorkflowStage updates the correlation context with a new workflow stage
func WithWorkflowStage(ctx context.Context, stage WorkflowStage) context.Context {
	corrCtx := GetCorrelationContext(ctx)
	if corrCtx == nil {
		return ctx
	}
	
	// Update the current stage entry duration if there was a previous stage
	if len(corrCtx.StageEntries) > 0 {
		lastEntry := &corrCtx.StageEntries[len(corrCtx.StageEntries)-1]
		if lastEntry.Duration == 0 {
			lastEntry.Duration = time.Since(lastEntry.EnteredAt)
		}
	}
	
	// Create new stage entry
	corrCtx.WorkflowStage = stage
	corrCtx.StageEntries = append(corrCtx.StageEntries, StageEntry{
		Stage:     stage,
		EnteredAt: time.Now(),
	})
	
	return WithCorrelationContext(ctx, corrCtx)
}

// WithHandoff records an agent handoff in the correlation context
func WithHandoff(ctx context.Context, fromAgent, toAgent, reason string, payloadSize int) context.Context {
	corrCtx := GetCorrelationContext(ctx)
	if corrCtx == nil {
		return ctx
	}
	
	handoff := HandoffInfo{
		FromAgent:   fromAgent,
		ToAgent:     toAgent,
		Timestamp:   time.Now(),
		Reason:      reason,
		PayloadSize: payloadSize,
	}
	
	corrCtx.HandoffChain = append(corrCtx.HandoffChain, handoff)
	corrCtx.AgentType = toAgent // Update current agent
	
	return WithCorrelationContext(ctx, corrCtx)
}

// WithMetadata adds metadata to the correlation context
func WithMetadata(ctx context.Context, key, value string) context.Context {
	corrCtx := GetCorrelationContext(ctx)
	if corrCtx == nil {
		return ctx
	}
	
	if corrCtx.Metadata == nil {
		corrCtx.Metadata = make(map[string]string)
	}
	corrCtx.Metadata[key] = value
	
	return WithCorrelationContext(ctx, corrCtx)
}

// GetWorkflowDuration returns the total duration of the workflow
func (cc *CorrelationContext) GetWorkflowDuration() time.Duration {
	if cc.StartTime.IsZero() {
		return 0
	}
	return time.Since(cc.StartTime)
}

// GetStageDuration returns the duration spent in a specific stage
func (cc *CorrelationContext) GetStageDuration(stage WorkflowStage) time.Duration {
	for _, entry := range cc.StageEntries {
		if entry.Stage == stage {
			if entry.Duration > 0 {
				return entry.Duration
			}
			// If it's the current stage, calculate duration from entry time
			return time.Since(entry.EnteredAt)
		}
	}
	return 0
}

// GetHandoffCount returns the number of handoffs in this workflow
func (cc *CorrelationContext) GetHandoffCount() int {
	return len(cc.HandoffChain)
}

// IsCurrentStage checks if the given stage is the current workflow stage
func (cc *CorrelationContext) IsCurrentStage(stage WorkflowStage) bool {
	return cc.WorkflowStage == stage
}