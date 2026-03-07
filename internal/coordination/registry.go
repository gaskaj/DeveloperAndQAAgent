package coordination

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentRegistry manages the real-time registry of active agents and their capabilities.
type AgentRegistry struct {
	mu      sync.RWMutex
	agents  map[string]*AgentInfo
	logger  *slog.Logger
}

// AgentInfo represents information about a registered agent.
type AgentInfo struct {
	ID           string                 `json:"id"`
	OrgID        uuid.UUID              `json:"org_id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Hostname     string                 `json:"hostname"`
	Version      string                 `json:"version"`
	Capabilities []string               `json:"capabilities"`
	Tags         []string               `json:"tags"`
	GitHubOwner  string                 `json:"github_owner"`
	GitHubRepo   string                 `json:"github_repo"`
	RegisteredAt time.Time              `json:"registered_at"`
	LastSeen     time.Time              `json:"last_seen"`
	Status       string                 `json:"status"`
	CurrentWork  map[string]*WorkItem   `json:"current_work"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// AgentStatus represents the current status of an agent.
type AgentStatus struct {
	AgentID       string                 `json:"agent_id"`
	OrgID         uuid.UUID              `json:"org_id"`
	Status        string                 `json:"status"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	Capabilities  []string               `json:"capabilities"`
	CurrentLoad   int                    `json:"current_load"`
	MaxLoad       int                    `json:"max_load"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// NewAgentRegistry creates a new agent registry.
func NewAgentRegistry(logger *slog.Logger) *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentInfo),
		logger: logger,
	}
}

// RegisterAgent adds an agent to the registry.
func (ar *AgentRegistry) RegisterAgent(agent *AgentInfo) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	agent.RegisteredAt = time.Now()
	agent.LastSeen = time.Now()
	agent.Status = "active"
	agent.CurrentWork = make(map[string]*WorkItem)
	
	ar.agents[agent.ID] = agent
	ar.logger.Info("agent registered in registry", "agent_id", agent.ID, "type", agent.Type)
}

// UnregisterAgent removes an agent from the registry.
func (ar *AgentRegistry) UnregisterAgent(agentID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	delete(ar.agents, agentID)
	ar.logger.Info("agent unregistered from registry", "agent_id", agentID)
}

// GetAgent returns information about a specific agent.
func (ar *AgentRegistry) GetAgent(agentID string) *AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		// Return a copy to avoid race conditions
		return ar.copyAgentInfo(agent)
	}
	return nil
}

// GetAllAgents returns information about all registered agents.
func (ar *AgentRegistry) GetAllAgents() []*AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	agents := make([]*AgentInfo, 0, len(ar.agents))
	for _, agent := range ar.agents {
		agents = append(agents, ar.copyAgentInfo(agent))
	}
	return agents
}

// GetAvailableAgents returns agents that are available and have required capabilities.
func (ar *AgentRegistry) GetAvailableAgents(requiredCapabilities []string) []*AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	var available []*AgentInfo
	for _, agent := range ar.agents {
		if ar.isAgentAvailable(agent) && ar.hasCapabilities(agent, requiredCapabilities) {
			available = append(available, ar.copyAgentInfo(agent))
		}
	}
	return available
}

// GetAgentsByOrg returns agents for a specific organization.
func (ar *AgentRegistry) GetAgentsByOrg(orgID uuid.UUID) []*AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	var orgAgents []*AgentInfo
	for _, agent := range ar.agents {
		if agent.OrgID == orgID {
			orgAgents = append(orgAgents, ar.copyAgentInfo(agent))
		}
	}
	return orgAgents
}

// GetAgentsByType returns agents of a specific type.
func (ar *AgentRegistry) GetAgentsByType(agentType string) []*AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	var typeAgents []*AgentInfo
	for _, agent := range ar.agents {
		if agent.Type == agentType {
			typeAgents = append(typeAgents, ar.copyAgentInfo(agent))
		}
	}
	return typeAgents
}

// AssignWork assigns work to an agent.
func (ar *AgentRegistry) AssignWork(agentID string, work *WorkItem) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		agent.CurrentWork[work.ID] = work
		ar.logger.Debug("work assigned to agent", "agent_id", agentID, "work_id", work.ID)
	}
}

// CompleteWork marks work as completed for an agent.
func (ar *AgentRegistry) CompleteWork(agentID string, workID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		delete(agent.CurrentWork, workID)
		ar.logger.Debug("work completed by agent", "agent_id", agentID, "work_id", workID)
	}
}

// UpdateHeartbeat updates the last seen timestamp for an agent.
func (ar *AgentRegistry) UpdateHeartbeat(agentID string, timestamp time.Time) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		agent.LastSeen = timestamp
		agent.Status = "active"
	}
}

// UpdateMetadata updates agent metadata.
func (ar *AgentRegistry) UpdateMetadata(agentID string, metadata map[string]interface{}) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		agent.Metadata = metadata
	}
}

// GetStaleAgents returns agents that haven't been seen since the threshold.
func (ar *AgentRegistry) GetStaleAgents(threshold time.Time) []string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	var staleAgents []string
	for agentID, agent := range ar.agents {
		if agent.LastSeen.Before(threshold) && agent.Status == "active" {
			staleAgents = append(staleAgents, agentID)
		}
	}
	return staleAgents
}

// GetAgentLoad returns the current work load for an agent.
func (ar *AgentRegistry) GetAgentLoad(agentID string) int {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		return len(agent.CurrentWork)
	}
	return 0
}

// GetAgentCapabilities returns the capabilities of an agent.
func (ar *AgentRegistry) GetAgentCapabilities(agentID string) []string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	if agent, exists := ar.agents[agentID]; exists {
		return append([]string{}, agent.Capabilities...) // Return a copy
	}
	return nil
}

// isAgentAvailable checks if an agent is available for work.
func (ar *AgentRegistry) isAgentAvailable(agent *AgentInfo) bool {
	return agent.Status == "active"
}

// hasCapabilities checks if an agent has the required capabilities.
func (ar *AgentRegistry) hasCapabilities(agent *AgentInfo, required []string) bool {
	if len(required) == 0 {
		return true // No specific requirements
	}
	
	capabilitySet := make(map[string]bool)
	for _, cap := range agent.Capabilities {
		capabilitySet[cap] = true
	}
	
	for _, required := range required {
		if !capabilitySet[required] {
			return false
		}
	}
	return true
}

// copyAgentInfo creates a deep copy of agent info to avoid race conditions.
func (ar *AgentRegistry) copyAgentInfo(agent *AgentInfo) *AgentInfo {
	copy := &AgentInfo{
		ID:           agent.ID,
		OrgID:        agent.OrgID,
		Name:         agent.Name,
		Type:         agent.Type,
		Hostname:     agent.Hostname,
		Version:      agent.Version,
		Capabilities: append([]string{}, agent.Capabilities...),
		Tags:         append([]string{}, agent.Tags...),
		GitHubOwner:  agent.GitHubOwner,
		GitHubRepo:   agent.GitHubRepo,
		RegisteredAt: agent.RegisteredAt,
		LastSeen:     agent.LastSeen,
		Status:       agent.Status,
		CurrentWork:  make(map[string]*WorkItem),
		Metadata:     make(map[string]interface{}),
	}
	
	// Copy current work
	for k, v := range agent.CurrentWork {
		copy.CurrentWork[k] = v
	}
	
	// Copy metadata
	for k, v := range agent.Metadata {
		copy.Metadata[k] = v
	}
	
	return copy
}

// GetRegistryStats returns statistics about the agent registry.
func (ar *AgentRegistry) GetRegistryStats() map[string]interface{} {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	
	stats := map[string]interface{}{
		"total_agents":  len(ar.agents),
		"active_agents": 0,
		"agents_by_type": make(map[string]int),
		"total_work_items": 0,
	}
	
	typeStats := stats["agents_by_type"].(map[string]int)
	
	for _, agent := range ar.agents {
		if agent.Status == "active" {
			stats["active_agents"] = stats["active_agents"].(int) + 1
		}
		typeStats[agent.Type]++
		stats["total_work_items"] = stats["total_work_items"].(int) + len(agent.CurrentWork)
	}
	
	return stats
}