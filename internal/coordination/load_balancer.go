package coordination

import (
	"log/slog"
	"math"
	"strings"
)

// LoadBalancer distributes work based on agent type, current load, and repository affinity.
type LoadBalancer struct {
	repositoryAffinityWeight float64
	maxConcurrentPerAgent    int
	logger                   *slog.Logger
}

// NewLoadBalancer creates a new load balancer.
func NewLoadBalancer(repositoryAffinityWeight float64, maxConcurrentPerAgent int, logger *slog.Logger) *LoadBalancer {
	return &LoadBalancer{
		repositoryAffinityWeight: repositoryAffinityWeight,
		maxConcurrentPerAgent:    maxConcurrentPerAgent,
		logger:                   logger,
	}
}

// SelectAgent selects the best agent for the given work item.
func (lb *LoadBalancer) SelectAgent(agents []*AgentInfo, work *WorkItem) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}
	
	// Filter out agents that are at capacity
	availableAgents := make([]*AgentInfo, 0, len(agents))
	for _, agent := range agents {
		if len(agent.CurrentWork) < lb.maxConcurrentPerAgent {
			availableAgents = append(availableAgents, agent)
		}
	}
	
	if len(availableAgents) == 0 {
		lb.logger.Debug("no agents available within capacity limits", "work_id", work.ID)
		return nil
	}
	
	// Score each available agent
	bestAgent := availableAgents[0]
	bestScore := lb.scoreAgent(bestAgent, work)
	
	for _, agent := range availableAgents[1:] {
		score := lb.scoreAgent(agent, work)
		if score > bestScore {
			bestAgent = agent
			bestScore = score
		}
	}
	
	lb.logger.Debug("selected agent for work", 
		"work_id", work.ID, 
		"agent_id", bestAgent.ID, 
		"score", bestScore,
		"current_load", len(bestAgent.CurrentWork),
	)
	
	return bestAgent
}

// scoreAgent calculates a score for an agent's suitability for a work item.
// Higher scores indicate better matches.
func (lb *LoadBalancer) scoreAgent(agent *AgentInfo, work *WorkItem) float64 {
	score := 0.0
	
	// Base score for being available
	score += 10.0
	
	// Agent type matching
	score += lb.scoreAgentType(agent, work)
	
	// Repository affinity
	score += lb.scoreRepositoryAffinity(agent, work) * lb.repositoryAffinityWeight
	
	// Load balancing - prefer agents with lower current load
	score += lb.scoreLoadBalance(agent)
	
	// Tag matching
	score += lb.scoreTagMatching(agent, work)
	
	// Capability matching bonus (already filtered, but give bonus for extra capabilities)
	score += lb.scoreCapabilityMatch(agent, work)
	
	return score
}

// scoreAgentType scores based on agent type suitability for work type.
func (lb *LoadBalancer) scoreAgentType(agent *AgentInfo, work *WorkItem) float64 {
	switch work.Type {
	case "github_issue":
		if agent.Type == "developer" {
			return 20.0 // Strong match
		} else if agent.Type == "devmanager" {
			return 15.0 // Good match
		} else if agent.Type == "qa" {
			return 5.0 // Weak match
		}
	case "github_pr":
		if agent.Type == "qa" {
			return 20.0 // Strong match for PR review
		} else if agent.Type == "developer" {
			return 15.0 // Good match
		} else if agent.Type == "devmanager" {
			return 10.0 // Fair match
		}
	case "custom":
		// For custom work, all agent types are considered equal
		return 10.0
	}
	
	return 0.0
}

// scoreRepositoryAffinity scores based on the agent's familiarity with the repository.
func (lb *LoadBalancer) scoreRepositoryAffinity(agent *AgentInfo, work *WorkItem) float64 {
	if work.GitHubOwner == "" || work.GitHubRepo == "" {
		return 0.0 // No repository context
	}
	
	// Exact repository match
	if agent.GitHubOwner == work.GitHubOwner && agent.GitHubRepo == work.GitHubRepo {
		return 30.0
	}
	
	// Same owner, different repo
	if agent.GitHubOwner == work.GitHubOwner {
		return 10.0
	}
	
	// Check if agent has worked on this repo recently
	repoKey := work.GitHubOwner + "/" + work.GitHubRepo
	for _, workItem := range agent.CurrentWork {
		if workItem.GitHubOwner+"/"+workItem.GitHubRepo == repoKey {
			return 20.0 // Currently working on same repo
		}
	}
	
	return 0.0
}

// scoreLoadBalance scores based on current agent load.
func (lb *LoadBalancer) scoreLoadBalance(agent *AgentInfo) float64 {
	currentLoad := len(agent.CurrentWork)
	maxLoad := lb.maxConcurrentPerAgent
	
	// Linear scoring: more available capacity = higher score
	loadRatio := float64(currentLoad) / float64(maxLoad)
	return (1.0 - loadRatio) * 15.0 // Max 15 points for being completely free
}

// scoreTagMatching scores based on matching tags between agent and work.
func (lb *LoadBalancer) scoreTagMatching(agent *AgentInfo, work *WorkItem) float64 {
	if len(work.Labels) == 0 {
		return 0.0
	}
	
	agentTagSet := make(map[string]bool)
	for _, tag := range agent.Tags {
		agentTagSet[strings.ToLower(tag)] = true
	}
	
	matches := 0
	for _, label := range work.Labels {
		if agentTagSet[strings.ToLower(label)] {
			matches++
		}
	}
	
	if matches == 0 {
		return 0.0
	}
	
	// Score based on percentage of labels matched
	matchRatio := float64(matches) / float64(len(work.Labels))
	return matchRatio * 10.0 // Max 10 points for perfect tag matching
}

// scoreCapabilityMatch scores extra capabilities beyond the minimum requirements.
func (lb *LoadBalancer) scoreCapabilityMatch(agent *AgentInfo, work *WorkItem) float64 {
	if len(work.RequiredCapabilities) == 0 {
		return 0.0
	}
	
	agentCapSet := make(map[string]bool)
	for _, cap := range agent.Capabilities {
		agentCapSet[cap] = true
	}
	
	extraCapabilities := 0
	totalCapabilities := len(agent.Capabilities)
	requiredCapabilities := len(work.RequiredCapabilities)
	
	// Count capabilities beyond the required ones
	extraCapabilities = totalCapabilities - requiredCapabilities
	if extraCapabilities < 0 {
		extraCapabilities = 0
	}
	
	// Small bonus for having extra capabilities
	return math.Min(float64(extraCapabilities), 5.0)
}

// GetLoadBalanceStats returns statistics about load balancing decisions.
func (lb *LoadBalancer) GetLoadBalanceStats(agents []*AgentInfo) map[string]interface{} {
	if len(agents) == 0 {
		return map[string]interface{}{
			"total_agents": 0,
			"load_distribution": map[string]int{},
			"agent_types": map[string]int{},
		}
	}
	
	loadDistribution := make(map[string]int)
	agentTypes := make(map[string]int)
	totalLoad := 0
	
	for _, agent := range agents {
		load := len(agent.CurrentWork)
		totalLoad += load
		
		loadKey := ""
		switch {
		case load == 0:
			loadKey = "idle"
		case load < lb.maxConcurrentPerAgent/2:
			loadKey = "light"
		case load < lb.maxConcurrentPerAgent:
			loadKey = "moderate"
		default:
			loadKey = "heavy"
		}
		
		loadDistribution[loadKey]++
		agentTypes[agent.Type]++
	}
	
	avgLoad := 0.0
	if len(agents) > 0 {
		avgLoad = float64(totalLoad) / float64(len(agents))
	}
	
	return map[string]interface{}{
		"total_agents": len(agents),
		"average_load": avgLoad,
		"total_work_items": totalLoad,
		"load_distribution": loadDistribution,
		"agent_types": agentTypes,
		"max_concurrent_per_agent": lb.maxConcurrentPerAgent,
		"repository_affinity_weight": lb.repositoryAffinityWeight,
	}
}

// RebalanceWorkload suggests work reassignments for better load distribution.
// This is a more advanced feature that could be implemented later.
func (lb *LoadBalancer) RebalanceWorkload(agents []*AgentInfo) []WorkRebalanceAction {
	// For now, return empty - this could be implemented as an enhancement
	return []WorkRebalanceAction{}
}

// WorkRebalanceAction represents a suggested work reassignment.
type WorkRebalanceAction struct {
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	WorkItem  *WorkItem `json:"work_item"`
	Reason    string    `json:"reason"`
	Score     float64   `json:"score"`
}