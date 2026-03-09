package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
	"github.com/gaskaj/OpenAgentFramework/web/middleware"
	"github.com/gaskaj/OpenAgentFramework/web/store"
	"github.com/gaskaj/OpenAgentFramework/web/ws"
)

// EventHandler handles event ingestion and query endpoints.
type EventHandler struct {
	events *store.PgEventStore
	agents *store.PgAgentStore
	hub    *ws.Hub
	logger *slog.Logger
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(events *store.PgEventStore, agents *store.PgAgentStore, hub *ws.Hub, logger *slog.Logger) *EventHandler {
	return &EventHandler{events: events, agents: agents, hub: hub, logger: logger}
}

func (h *EventHandler) HandleIngestSingle(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req apitypes.SingleEventRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer agent identity from API key context; fall back to request body
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = req.AgentName
	}

	agent, err := h.agents.GetByOrgAndName(r.Context(), orgCtx.OrgID, agentName)
	if err != nil || agent == nil {
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}

	event := &store.AgentEvent{
		OrgID:         orgCtx.OrgID,
		AgentID:       agent.ID,
		EventType:     string(req.Event.EventType),
		Severity:      string(req.Event.Severity),
		Payload:       store.MarshalPayload(req.Event.Payload),
		WorkflowState: req.Event.WorkflowState,
		CorrelationID: req.Event.CorrelationID,
	}
	if req.Event.IssueNumber > 0 {
		event.IssueNumber = &req.Event.IssueNumber
	}
	if req.Event.PRNumber > 0 {
		event.PRNumber = &req.Event.PRNumber
	}

	if err := h.events.Insert(r.Context(), event); err != nil {
		h.logger.Error("inserting event", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to store event")
		return
	}

	// Update agent status based on event type
	h.updateAgentStatusFromEvent(r.Context(), agent.ID, req.Event.EventType)

	// Broadcast to WebSocket clients
	h.broadcastEvent(orgCtx.OrgID, event)

	respondJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *EventHandler) HandleIngestBatch(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req apitypes.BatchEventRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer agent identity from API key context; fall back to request body
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = req.AgentName
	}

	agent, err := h.agents.GetByOrgAndName(r.Context(), orgCtx.OrgID, agentName)
	if err != nil || agent == nil {
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}

	var storeEvents []store.AgentEvent
	for _, e := range req.Events {
		se := store.AgentEvent{
			OrgID:         orgCtx.OrgID,
			AgentID:       agent.ID,
			EventType:     string(e.EventType),
			Severity:      string(e.Severity),
			Payload:       store.MarshalPayload(e.Payload),
			WorkflowState: e.WorkflowState,
			CorrelationID: e.CorrelationID,
		}
		if e.IssueNumber > 0 {
			se.IssueNumber = &e.IssueNumber
		}
		if e.PRNumber > 0 {
			se.PRNumber = &e.PRNumber
		}
		storeEvents = append(storeEvents, se)
	}

	if err := h.events.InsertBatch(r.Context(), storeEvents); err != nil {
		h.logger.Error("inserting event batch", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to store events")
		return
	}

	// Update agent status based on event types in batch
	for _, e := range req.Events {
		h.updateAgentStatusFromEvent(r.Context(), agent.ID, e.EventType)
	}

	// Broadcast last event to WebSocket
	if len(storeEvents) > 0 {
		h.broadcastEvent(orgCtx.OrgID, &storeEvents[len(storeEvents)-1])
	}

	respondJSON(w, http.StatusCreated, map[string]any{"status": "ok", "count": len(storeEvents)})
}

func (h *EventHandler) HandleIngestRegister(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req apitypes.AgentRegistration
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer agent identity from API key context; fall back to request body
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = req.Name
	}
	agentType := orgCtx.AgentType
	if agentType == "" {
		agentType = req.AgentType
	}

	var configSnapshot json.RawMessage
	if req.Config != nil {
		configSnapshot, _ = json.Marshal(req.Config)
	}

	agent := &store.Agent{
		OrgID:          orgCtx.OrgID,
		Name:           agentName,
		AgentType:      agentType,
		Version:        req.Version,
		Hostname:       req.Hostname,
		GitHubOwner:    req.GitHubOwner,
		GitHubRepo:     req.GitHubRepo,
		Tags:           req.Tags,
		Status:         "online",
		ConfigSnapshot: configSnapshot,
	}
	if agent.Tags == nil {
		agent.Tags = []string{}
	}

	if err := h.agents.Register(r.Context(), agent); err != nil {
		h.logger.Error("registering agent via ingestion", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to register agent")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": agent})
}

func (h *EventHandler) HandleIngestHeartbeat(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req apitypes.HeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer agent identity from API key context; fall back to request body
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = req.AgentName
	}

	agent, err := h.agents.GetByOrgAndName(r.Context(), orgCtx.OrgID, agentName)
	if err != nil || agent == nil {
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}

	if err := h.agents.UpdateHeartbeat(r.Context(), agent.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleIngestLogs receives agent log entries and broadcasts them via WebSocket
// without persisting to the database. This provides a real-time log stream view.
func (h *EventHandler) HandleIngestLogs(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req apitypes.LogBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer agent identity from API key context
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = req.AgentName
	}

	// Broadcast each log entry to WebSocket clients — no DB persistence
	for _, entry := range req.Entries {
		entry.AgentName = agentName
		h.broadcastLogEntry(orgCtx.OrgID, &entry)
	}

	respondJSON(w, http.StatusOK, map[string]any{"status": "ok", "count": len(req.Entries)})
}

func (h *EventHandler) broadcastLogEntry(orgID uuid.UUID, entry *apitypes.LogEntry) {
	if h.hub == nil {
		return
	}
	// Wrap in an envelope so the frontend can distinguish log messages from events
	msg := map[string]any{
		"type": "agent.log",
		"log":  entry,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.hub.Broadcast(orgID, data)
}

func (h *EventHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	filter := store.EventFilter{
		OrgID:     orgCtx.OrgID,
		EventType: r.URL.Query().Get("event_type"),
		Severity:  r.URL.Query().Get("severity"),
		ListOpts: store.ListOpts{
			Limit:  parseIntParam(r, "limit", 20),
			Offset: parseIntParam(r, "offset", 0),
		},
	}

	if agentIDStr := r.URL.Query().Get("agent_id"); agentIDStr != "" {
		agentID, err := uuid.Parse(agentIDStr)
		if err == nil {
			filter.AgentID = &agentID
		}
	}

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &t
		}
	}

	events, total, err := h.events.Query(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query events")
		return
	}

	respondList(w, events, total, filter.Limit, filter.Offset)
}

func (h *EventHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	counts, err := h.events.CountByType(r.Context(), orgCtx.OrgID, since)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	// Count agents
	agents, agentsTotal, err := h.agents.ListByOrg(r.Context(), orgCtx.OrgID, store.ListOpts{Limit: 10000, Offset: 0})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get agent stats")
		return
	}
	agentsOnline := 0
	for _, a := range agents {
		if a.Status == "online" {
			agentsOnline++
		}
	}

	// Count severity
	severityCounts, err := h.events.CountBySeverity(r.Context(), orgCtx.OrgID, since)
	if err != nil {
		severityCounts = map[string]int64{}
	}

	// Count specific event types for issues/PRs
	var totalEvents int64
	for _, v := range counts {
		totalEvents += v
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"total_events":           totalEvents,
		"events_today":          totalEvents,
		"events_by_type":        counts,
		"events_by_severity":    severityCounts,
		"agents_online":         agentsOnline,
		"agents_total":          agentsTotal,
		"issues_processed_today": counts["issue_claimed"] + counts["issue_completed"],
		"prs_created_today":     counts["pr_created"],
	})
}

func (h *EventHandler) HandleAgentEvents(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	filter := store.EventFilter{
		OrgID:   orgCtx.OrgID,
		AgentID: &agentID,
		ListOpts: store.ListOpts{
			Limit:  parseIntParam(r, "limit", 20),
			Offset: parseIntParam(r, "offset", 0),
		},
	}

	events, total, err := h.events.Query(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query events")
		return
	}

	respondList(w, events, total, filter.Limit, filter.Offset)
}

func (h *EventHandler) updateAgentStatusFromEvent(ctx context.Context, agentID uuid.UUID, eventType apitypes.EventType) {
	var status string
	switch eventType {
	case apitypes.EventAgentStarted:
		status = "online"
	case apitypes.EventAgentStopped:
		status = "offline"
	default:
		return
	}
	if err := h.agents.UpdateStatus(ctx, agentID, status); err != nil {
		h.logger.Error("updating agent status from event", "agent_id", agentID, "status", status, "error", err)
	}
}

func (h *EventHandler) broadcastEvent(orgID uuid.UUID, event *store.AgentEvent) {
	if h.hub == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.hub.Broadcast(orgID, data)
}
