package apitypes

import "time"

// EventType identifies the kind of agent event.
type EventType string

const (
	EventAgentStarted       EventType = "agent.started"
	EventAgentStopped       EventType = "agent.stopped"
	EventAgentHeartbeat     EventType = "agent.heartbeat"
	EventIssueClaimed       EventType = "issue.claimed"
	EventIssueAnalyzed      EventType = "issue.analyzed"
	EventIssueDecomposed    EventType = "issue.decomposed"
	EventIssueImplemented   EventType = "issue.implemented"
	EventIssueCommitted     EventType = "issue.committed"
	EventPRCreated          EventType = "pr.created"
	EventPRValidated        EventType = "pr.validated"
	EventIssueFailed        EventType = "issue.failed"
	EventIssueCompleted     EventType = "issue.completed"
	EventWorkflowTransition EventType = "workflow.transition"
	EventSuggestionCreated  EventType = "suggestion.created"
	EventAgentLog           EventType = "agent.log"
)

// LogEntry represents a structured log line from an agent.
type LogEntry struct {
	AgentName string         `json:"agent_name"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// LogBatchRequest is the request body for batch log ingestion.
type LogBatchRequest struct {
	AgentName string     `json:"agent_name"`
	Entries   []LogEntry `json:"entries"`
}

// Severity represents the severity level of an event.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// AgentEvent is the payload agents send to the control plane.
type AgentEvent struct {
	EventType     EventType      `json:"event_type"`
	Severity      Severity       `json:"severity"`
	Payload       map[string]any `json:"payload,omitempty"`
	IssueNumber   int            `json:"issue_number,omitempty"`
	PRNumber      int            `json:"pr_number,omitempty"`
	WorkflowState string         `json:"workflow_state,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	APIVersion    string         `json:"api_version,omitempty"`
}

// AgentRegistration is the payload agents send to register with the control plane.
type AgentRegistration struct {
	Name        string         `json:"name"`
	AgentType   string         `json:"agent_type"`
	Version     string         `json:"version,omitempty"`
	Hostname    string         `json:"hostname,omitempty"`
	GitHubOwner string         `json:"github_owner,omitempty"`
	GitHubRepo  string         `json:"github_repo,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	APIVersion  string         `json:"api_version,omitempty"`
}

// BatchEventRequest is the request body for batch event ingestion.
type BatchEventRequest struct {
	AgentName string       `json:"agent_name"`
	Events    []AgentEvent `json:"events"`
}

// SingleEventRequest is the request body for single event ingestion.
type SingleEventRequest struct {
	AgentName string     `json:"agent_name"`
	Event     AgentEvent `json:"event"`
}

// HeartbeatRequest is the request body for agent heartbeat.
type HeartbeatRequest struct {
	AgentName string         `json:"agent_name"`
	Status    string         `json:"status,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}
