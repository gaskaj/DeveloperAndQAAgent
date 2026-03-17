package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
	"github.com/gaskaj/OpenAgentFramework/web/middleware"
	"github.com/gaskaj/OpenAgentFramework/web/store"
)

// ConfigHandler handles agent configuration management endpoints.
type ConfigHandler struct {
	configs *store.PgConfigStore
	agents  *store.PgAgentStore
	logger  *slog.Logger
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(configs *store.PgConfigStore, agents *store.PgAgentStore, logger *slog.Logger) *ConfigHandler {
	return &ConfigHandler{configs: configs, agents: agents, logger: logger}
}

// HandleGetAgentTypeConfig returns the type-level config for an agent type.
func (h *ConfigHandler) HandleGetAgentTypeConfig(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	agentType := chi.URLParam(r, "agentType")
	if agentType == "" {
		respondError(w, http.StatusBadRequest, "agent type is required")
		return
	}

	config, err := h.configs.GetAgentTypeConfig(r.Context(), orgCtx.OrgID, agentType)
	if err != nil {
		h.logger.Error("getting agent type config", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to get config")
		return
	}

	if config == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"agent_type": agentType,
				"config":     json.RawMessage("{}"),
				"version":    0,
			},
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": config})
}

// HandleListAgentTypeConfigs returns all type-level configs for the org.
func (h *ConfigHandler) HandleListAgentTypeConfigs(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	configs, err := h.configs.ListAgentTypeConfigs(r.Context(), orgCtx.OrgID)
	if err != nil {
		h.logger.Error("listing agent type configs", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list configs")
		return
	}

	if configs == nil {
		configs = []store.AgentTypeConfig{}
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": configs})
}

// HandleUpsertAgentTypeConfig creates or updates a type-level config.
func (h *ConfigHandler) HandleUpsertAgentTypeConfig(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}
	userCtx := middleware.GetUserFromContext(r.Context())
	if userCtx == nil {
		respondError(w, http.StatusForbidden, "no user context")
		return
	}

	agentType := chi.URLParam(r, "agentType")
	if agentType == "" {
		respondError(w, http.StatusBadRequest, "agent type is required")
		return
	}

	var req struct {
		Config      json.RawMessage `json:"config"`
		Description string          `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Config) == 0 {
		req.Config = json.RawMessage("{}")
	}

	config, err := h.configs.UpsertAgentTypeConfig(r.Context(), orgCtx.OrgID, agentType, req.Config, req.Description, userCtx.UserID)
	if err != nil {
		h.logger.Error("upserting agent type config", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": config})
}

// HandleGetAgentOverride returns the per-agent config override.
func (h *ConfigHandler) HandleGetAgentOverride(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	override, err := h.configs.GetAgentOverride(r.Context(), agentID)
	if err != nil {
		h.logger.Error("getting agent override", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to get override")
		return
	}

	if override == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"agent_id": agentID,
				"config":   json.RawMessage("{}"),
				"version":  0,
			},
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": override})
}

// HandleUpsertAgentOverride creates or updates a per-agent config override.
func (h *ConfigHandler) HandleUpsertAgentOverride(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}
	userCtx := middleware.GetUserFromContext(r.Context())
	if userCtx == nil {
		respondError(w, http.StatusForbidden, "no user context")
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	var req struct {
		Config      json.RawMessage `json:"config"`
		Description string          `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Config) == 0 {
		req.Config = json.RawMessage("{}")
	}

	override, err := h.configs.UpsertAgentOverride(r.Context(), agentID, orgCtx.OrgID, req.Config, req.Description, userCtx.UserID)
	if err != nil {
		h.logger.Error("upserting agent override", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to save override")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": override})
}

// HandleDeleteAgentOverride removes a per-agent config override.
func (h *ConfigHandler) HandleDeleteAgentOverride(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid agent ID")
		return
	}

	if err := h.configs.DeleteAgentOverride(r.Context(), agentID); err != nil {
		h.logger.Error("deleting agent override", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to delete override")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleGetMergedConfig returns the fully merged config for an agent (type + override).
func (h *ConfigHandler) HandleGetMergedConfig(w http.ResponseWriter, r *http.Request) {
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

	agent, err := h.agents.GetByID(r.Context(), agentID)
	if err != nil || agent == nil {
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}

	merged, version, err := h.configs.GetMergedConfig(r.Context(), orgCtx.OrgID, agentID, agent.AgentType)
	if err != nil {
		h.logger.Error("getting merged config", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to get merged config")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"config":  merged,
			"version": version,
		},
	})
}

// HandleIngestConfig is called by agents to fetch their merged configuration.
// Uses API key authentication (not JWT).
func (h *ConfigHandler) HandleIngestConfig(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	// Prefer agent identity from API key context; fall back to query param
	agentName := orgCtx.AgentName
	if agentName == "" {
		agentName = r.URL.Query().Get("agent_name")
	}
	agentType := orgCtx.AgentType
	if agentType == "" {
		agentType = r.URL.Query().Get("agent_type")
	}

	// Try to find the registered agent for per-agent overrides
	var agentID uuid.UUID
	if agentName != "" {
		agent, err := h.agents.GetByOrgAndName(r.Context(), orgCtx.OrgID, agentName)
		if err == nil && agent != nil {
			agentID = agent.ID
			if agentType == "" {
				agentType = agent.AgentType
			}
		}
	}

	// Fall back to "developer" if we still can't determine the type
	if agentType == "" {
		agentType = "developer"
	}

	merged, version, err := h.configs.GetMergedConfig(r.Context(), orgCtx.OrgID, agentID, agentType)
	if err != nil {
		h.logger.Error("getting merged config for agent", "error", err, "agent", agentName, "agent_type", agentType)
		respondError(w, http.StatusInternalServerError, "failed to get config")
		return
	}

	// Unmarshal, apply server-side defaults for missing fields, then return.
	// This ensures agents receive complete configuration even when only some
	// fields were explicitly set in the control plane UI.
	var remoteConfig apitypes.RemoteConfig
	if err := json.Unmarshal(merged, &remoteConfig); err != nil {
		h.logger.Error("unmarshalling merged config", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to process config")
		return
	}
	remoteConfig.ApplyServerDefaults()

	// Support ETag-based conditional requests
	etag := fmt.Sprintf(`"%d"`, version)
	w.Header().Set("ETag", etag)

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"config":  remoteConfig,
		"version": version,
	})
}

// HandleConfigAudit returns the config change audit trail for an org.
func (h *ConfigHandler) HandleConfigAudit(w http.ResponseWriter, r *http.Request) {
	orgCtx := middleware.GetOrgFromContext(r.Context())
	if orgCtx == nil {
		respondError(w, http.StatusForbidden, "no org context")
		return
	}

	opts := store.ListOpts{
		Limit:  parseIntParam(r, "limit", 20),
		Offset: parseIntParam(r, "offset", 0),
	}

	entries, total, err := h.configs.ListConfigAudit(r.Context(), orgCtx.OrgID, opts)
	if err != nil {
		h.logger.Error("listing config audit", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to list audit")
		return
	}

	respondList(w, entries, total, opts.Limit, opts.Offset)
}
