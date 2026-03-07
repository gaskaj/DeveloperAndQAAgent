package coordination

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
)

// WorkCoordinator is the central service for intelligent work assignment and agent coordination.
type WorkCoordinator struct {
	mu            sync.RWMutex
	registry      *AgentRegistry
	queue         *WorkQueue
	loadBalancer  *LoadBalancer
	assignmentStore WorkAssignmentStore
	agentStore    AgentStatusStore
	github        ghub.Client
	logger        *slog.Logger

	// Configuration
	assignmentTimeout     time.Duration
	heartbeatInterval     time.Duration
	maxConcurrentPerAgent int
	repositoryAffinityWeight float64
	
	// Channels for coordination
	assignmentCh   chan *WorkAssignment
	completionCh   chan *WorkCompletion
	heartbeatCh    chan *AgentHeartbeat
	disconnectCh   chan string // agent ID
	
	// Stop channel
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Config holds configuration for the work coordinator.
type Config struct {
	AssignmentTimeout        time.Duration
	HeartbeatInterval        time.Duration
	MaxConcurrentPerAgent    int
	RepositoryAffinityWeight float64
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		AssignmentTimeout:        5 * time.Minute,
		HeartbeatInterval:        30 * time.Second,
		MaxConcurrentPerAgent:    3,
		RepositoryAffinityWeight: 0.7,
	}
}

// NewWorkCoordinator creates a new work coordinator.
func NewWorkCoordinator(
	assignmentStore WorkAssignmentStore,
	agentStore AgentStatusStore,
	github ghub.Client,
	config Config,
	logger *slog.Logger,
) *WorkCoordinator {
	registry := NewAgentRegistry(logger)
	queue := NewWorkQueue(logger)
	loadBalancer := NewLoadBalancer(config.RepositoryAffinityWeight, config.MaxConcurrentPerAgent, logger)
	
	return &WorkCoordinator{
		registry:                 registry,
		queue:                    queue,
		loadBalancer:             loadBalancer,
		assignmentStore:          assignmentStore,
		agentStore:               agentStore,
		github:                   github,
		logger:                   logger,
		assignmentTimeout:        config.AssignmentTimeout,
		heartbeatInterval:        config.HeartbeatInterval,
		maxConcurrentPerAgent:    config.MaxConcurrentPerAgent,
		repositoryAffinityWeight: config.RepositoryAffinityWeight,
		assignmentCh:             make(chan *WorkAssignment, 100),
		completionCh:             make(chan *WorkCompletion, 100),
		heartbeatCh:              make(chan *AgentHeartbeat, 100),
		disconnectCh:             make(chan string, 100),
		stopCh:                   make(chan struct{}),
	}
}

// Start begins the coordination service.
func (wc *WorkCoordinator) Start(ctx context.Context) error {
	wc.logger.Info("starting work coordinator")
	
	// Recover any existing assignments from the database
	if err := wc.recoverAssignments(ctx); err != nil {
		return fmt.Errorf("failed to recover assignments: %w", err)
	}
	
	// Start main coordination loop
	wc.wg.Add(1)
	go wc.coordinationLoop(ctx)
	
	// Start heartbeat monitor
	wc.wg.Add(1)
	go wc.heartbeatMonitor(ctx)
	
	// Start timeout monitor
	wc.wg.Add(1)
	go wc.timeoutMonitor(ctx)
	
	return nil
}

// Stop stops the coordination service.
func (wc *WorkCoordinator) Stop() error {
	wc.logger.Info("stopping work coordinator")
	close(wc.stopCh)
	wc.wg.Wait()
	return nil
}

// RegisterAgent registers a new agent with the coordinator.
func (wc *WorkCoordinator) RegisterAgent(ctx context.Context, agent *AgentInfo) error {
	wc.logger.Info("registering agent", "agent_id", agent.ID, "type", agent.Type)
	
	// Register with registry
	wc.registry.RegisterAgent(agent)
	
	// Store in database
	status := &AgentStatus{
		AgentID:       agent.ID,
		OrgID:         agent.OrgID,
		Status:        "active",
		LastHeartbeat: time.Now(),
		Capabilities:  agent.Capabilities,
		CurrentLoad:   0,
		MaxLoad:       wc.maxConcurrentPerAgent,
	}
	
	return wc.agentStore.UpdateStatus(ctx, status)
}

// UnregisterAgent removes an agent from the coordinator.
func (wc *WorkCoordinator) UnregisterAgent(ctx context.Context, agentID string) error {
	wc.logger.Info("unregistering agent", "agent_id", agentID)
	
	// Signal disconnection
	select {
	case wc.disconnectCh <- agentID:
	default:
		wc.logger.Warn("disconnect channel full, processing inline", "agent_id", agentID)
		wc.handleAgentDisconnect(ctx, agentID)
	}
	
	return nil
}

// AssignWork queues work for assignment to agents.
func (wc *WorkCoordinator) AssignWork(ctx context.Context, work *WorkItem) error {
	wc.logger.Info("queueing work for assignment", "work_id", work.ID, "type", work.Type)
	
	// Add to queue
	wc.queue.Enqueue(work)
	
	// Try immediate assignment
	return wc.tryAssignWork(ctx)
}

// CompleteWork marks work as completed by an agent.
func (wc *WorkCoordinator) CompleteWork(ctx context.Context, agentID string, workID string, result *WorkResult) error {
	completion := &WorkCompletion{
		AgentID:     agentID,
		WorkID:      workID,
		Result:      result,
		CompletedAt: time.Now(),
	}
	
	select {
	case wc.completionCh <- completion:
		return nil
	default:
		return fmt.Errorf("completion channel full")
	}
}

// SendHeartbeat processes agent heartbeats.
func (wc *WorkCoordinator) SendHeartbeat(ctx context.Context, agentID string, payload *HeartbeatPayload) error {
	heartbeat := &AgentHeartbeat{
		AgentID:   agentID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	
	select {
	case wc.heartbeatCh <- heartbeat:
		return nil
	default:
		return fmt.Errorf("heartbeat channel full")
	}
}

// GetAgentStatus returns the current status of all agents.
func (wc *WorkCoordinator) GetAgentStatus(ctx context.Context, orgID uuid.UUID) ([]*AgentStatus, error) {
	return wc.agentStore.GetByOrg(ctx, orgID)
}

// GetWorkAssignments returns current work assignments for an organization.
func (wc *WorkCoordinator) GetWorkAssignments(ctx context.Context, orgID uuid.UUID) ([]*WorkAssignment, error) {
	return wc.assignmentStore.GetActiveByOrg(ctx, orgID)
}

// coordinationLoop is the main event processing loop.
func (wc *WorkCoordinator) coordinationLoop(ctx context.Context) {
	defer wc.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second) // Regular work assignment attempts
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-wc.stopCh:
			return
		case <-ticker.C:
			wc.tryAssignWork(ctx)
		case completion := <-wc.completionCh:
			wc.handleWorkCompletion(ctx, completion)
		case heartbeat := <-wc.heartbeatCh:
			wc.handleHeartbeat(ctx, heartbeat)
		case agentID := <-wc.disconnectCh:
			wc.handleAgentDisconnect(ctx, agentID)
		}
	}
}

// tryAssignWork attempts to assign queued work to available agents.
func (wc *WorkCoordinator) tryAssignWork(ctx context.Context) error {
	for {
		work := wc.queue.Dequeue()
		if work == nil {
			break // No more work to assign
		}
		
		// Find best agent for this work
		agents := wc.registry.GetAvailableAgents(work.RequiredCapabilities)
		agent := wc.loadBalancer.SelectAgent(agents, work)
		
		if agent == nil {
			wc.logger.Debug("no available agent for work, re-queuing", "work_id", work.ID)
			wc.queue.Enqueue(work) // Put back in queue
			break
		}
		
		// Create assignment
		assignment := &WorkAssignment{
			ID:        uuid.New().String(),
			AgentID:   agent.ID,
			OrgID:     work.OrgID,
			WorkItem:  work,
			Status:    "assigned",
			AssignedAt: time.Now(),
			ExpiresAt: time.Now().Add(wc.assignmentTimeout),
		}
		
		// Store assignment
		if err := wc.assignmentStore.Create(ctx, assignment); err != nil {
			wc.logger.Error("failed to store work assignment", "error", err, "work_id", work.ID)
			wc.queue.Enqueue(work) // Put back in queue
			continue
		}
		
		// Update agent status
		wc.registry.AssignWork(agent.ID, work)
		
		// Send assignment notification
		select {
		case wc.assignmentCh <- assignment:
		default:
			wc.logger.Warn("assignment channel full, assignment stored but not notified", "work_id", work.ID)
		}
		
		wc.logger.Info("assigned work to agent", "work_id", work.ID, "agent_id", agent.ID)
	}
	
	return nil
}

// handleWorkCompletion processes work completion notifications.
func (wc *WorkCoordinator) handleWorkCompletion(ctx context.Context, completion *WorkCompletion) {
	wc.logger.Info("processing work completion", "work_id", completion.WorkID, "agent_id", completion.AgentID)
	
	// Update assignment status
	if err := wc.assignmentStore.Complete(ctx, completion.WorkID, completion.Result); err != nil {
		wc.logger.Error("failed to update assignment completion", "error", err, "work_id", completion.WorkID)
		return
	}
	
	// Update agent status
	wc.registry.CompleteWork(completion.AgentID, completion.WorkID)
	
	// Try to assign more work
	wc.tryAssignWork(ctx)
}

// handleHeartbeat processes agent heartbeat messages.
func (wc *WorkCoordinator) handleHeartbeat(ctx context.Context, heartbeat *AgentHeartbeat) {
	// Update registry
	wc.registry.UpdateHeartbeat(heartbeat.AgentID, heartbeat.Timestamp)
	
	// Update database
	status := &AgentStatus{
		AgentID:       heartbeat.AgentID,
		Status:        "active",
		LastHeartbeat: heartbeat.Timestamp,
	}
	
	if heartbeat.Payload != nil {
		status.CurrentLoad = heartbeat.Payload.CurrentLoad
		status.Metadata = heartbeat.Payload.Metadata
	}
	
	if err := wc.agentStore.UpdateStatus(ctx, status); err != nil {
		wc.logger.Error("failed to update agent heartbeat", "error", err, "agent_id", heartbeat.AgentID)
	}
}

// handleAgentDisconnect processes agent disconnection.
func (wc *WorkCoordinator) handleAgentDisconnect(ctx context.Context, agentID string) {
	wc.logger.Info("processing agent disconnect", "agent_id", agentID)
	
	// Unregister from registry
	wc.registry.UnregisterAgent(agentID)
	
	// Reassign any active work
	assignments, err := wc.assignmentStore.GetActiveByAgent(ctx, agentID)
	if err != nil {
		wc.logger.Error("failed to get assignments for disconnected agent", "error", err, "agent_id", agentID)
		return
	}
	
	for _, assignment := range assignments {
		wc.logger.Info("reassigning work from disconnected agent", "work_id", assignment.WorkItem.ID, "agent_id", agentID)
		
		// Mark assignment as failed
		if err := wc.assignmentStore.Fail(ctx, assignment.ID, "agent_disconnected"); err != nil {
			wc.logger.Error("failed to mark assignment as failed", "error", err, "assignment_id", assignment.ID)
			continue
		}
		
		// Re-queue the work
		wc.queue.Enqueue(assignment.WorkItem)
	}
	
	// Update agent status in database
	status := &AgentStatus{
		AgentID: agentID,
		Status:  "disconnected",
	}
	
	if err := wc.agentStore.UpdateStatus(ctx, status); err != nil {
		wc.logger.Error("failed to update agent status on disconnect", "error", err, "agent_id", agentID)
	}
	
	// Try to reassign work
	wc.tryAssignWork(ctx)
}

// heartbeatMonitor monitors agent heartbeats and detects stale agents.
func (wc *WorkCoordinator) heartbeatMonitor(ctx context.Context) {
	defer wc.wg.Done()
	
	ticker := time.NewTicker(wc.heartbeatInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-wc.stopCh:
			return
		case <-ticker.C:
			wc.checkStaleAgents(ctx)
		}
	}
}

// checkStaleAgents checks for agents that haven't sent heartbeats recently.
func (wc *WorkCoordinator) checkStaleAgents(ctx context.Context) {
	staleThreshold := time.Now().Add(-wc.heartbeatInterval * 3) // 3 missed heartbeats
	staleAgents := wc.registry.GetStaleAgents(staleThreshold)
	
	for _, agentID := range staleAgents {
		wc.logger.Warn("detected stale agent", "agent_id", agentID)
		wc.handleAgentDisconnect(ctx, agentID)
	}
}

// timeoutMonitor monitors work assignments for timeouts.
func (wc *WorkCoordinator) timeoutMonitor(ctx context.Context) {
	defer wc.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-wc.stopCh:
			return
		case <-ticker.C:
			wc.checkExpiredAssignments(ctx)
		}
	}
}

// checkExpiredAssignments checks for and reassigns expired work assignments.
func (wc *WorkCoordinator) checkExpiredAssignments(ctx context.Context) {
	expired, err := wc.assignmentStore.GetExpired(ctx)
	if err != nil {
		wc.logger.Error("failed to get expired assignments", "error", err)
		return
	}
	
	for _, assignment := range expired {
		wc.logger.Warn("reassigning expired work", "work_id", assignment.WorkItem.ID, "agent_id", assignment.AgentID)
		
		// Mark assignment as failed
		if err := wc.assignmentStore.Fail(ctx, assignment.ID, "timeout"); err != nil {
			wc.logger.Error("failed to mark assignment as failed", "error", err, "assignment_id", assignment.ID)
			continue
		}
		
		// Update agent status
		wc.registry.CompleteWork(assignment.AgentID, assignment.WorkItem.ID)
		
		// Re-queue the work
		wc.queue.Enqueue(assignment.WorkItem)
	}
	
	// Try to reassign work
	if len(expired) > 0 {
		wc.tryAssignWork(ctx)
	}
}

// recoverAssignments recovers any existing assignments from database on startup.
func (wc *WorkCoordinator) recoverAssignments(ctx context.Context) error {
	// This would typically scan all organizations, but for simplicity
	// we'll implement it later when we have org context
	wc.logger.Info("assignment recovery not implemented yet")
	return nil
}

// GetAssignmentChannel returns the assignment notification channel.
func (wc *WorkCoordinator) GetAssignmentChannel() <-chan *WorkAssignment {
	return wc.assignmentCh
}