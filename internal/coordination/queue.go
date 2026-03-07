package coordination

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkQueue manages a prioritized queue with conflict detection and resolution.
type WorkQueue struct {
	mu       sync.RWMutex
	items    []*WorkItem
	byID     map[string]*WorkItem
	byRepo   map[string][]*WorkItem // GitHub repo -> work items
	byIssue  map[string]*WorkItem   // repo:issue -> work item
	logger   *slog.Logger
}

// WorkItem represents a unit of work to be assigned to an agent.
type WorkItem struct {
	ID                   string                 `json:"id"`
	OrgID                uuid.UUID              `json:"org_id"`
	Type                 string                 `json:"type"`                   // "github_issue", "github_pr", "custom"
	Priority             int                    `json:"priority"`               // Higher number = higher priority
	RequiredCapabilities []string               `json:"required_capabilities"`
	GitHubOwner          string                 `json:"github_owner,omitempty"`
	GitHubRepo           string                 `json:"github_repo,omitempty"`
	IssueNumber          int                    `json:"issue_number,omitempty"`
	PRNumber             int                    `json:"pr_number,omitempty"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	Labels               []string               `json:"labels"`
	Metadata             map[string]interface{} `json:"metadata"`
	CreatedAt            time.Time              `json:"created_at"`
	EstimatedDuration    time.Duration          `json:"estimated_duration,omitempty"`
}

// WorkAssignment represents an assignment of work to an agent.
type WorkAssignment struct {
	ID         string     `json:"id"`
	AgentID    string     `json:"agent_id"`
	OrgID      uuid.UUID  `json:"org_id"`
	WorkItem   *WorkItem  `json:"work_item"`
	Status     string     `json:"status"` // "assigned", "accepted", "in_progress", "completed", "failed"
	AssignedAt time.Time  `json:"assigned_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	Result     *WorkResult `json:"result,omitempty"`
}

// WorkResult represents the result of completed work.
type WorkResult struct {
	Status     string                 `json:"status"` // "success", "failure", "partial"
	Message    string                 `json:"message"`
	Artifacts  []WorkArtifact         `json:"artifacts,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Duration   time.Duration          `json:"duration"`
	PRNumber   int                    `json:"pr_number,omitempty"`
}

// WorkArtifact represents an artifact produced by work completion.
type WorkArtifact struct {
	Type        string `json:"type"`        // "pull_request", "commit", "comment", "file"
	URL         string `json:"url,omitempty"`
	Path        string `json:"path,omitempty"`
	SHA         string `json:"sha,omitempty"`
	Description string `json:"description,omitempty"`
}

// WorkCompletion represents a work completion event.
type WorkCompletion struct {
	AgentID     string      `json:"agent_id"`
	WorkID      string      `json:"work_id"`
	Result      *WorkResult `json:"result"`
	CompletedAt time.Time   `json:"completed_at"`
}

// AgentHeartbeat represents a heartbeat from an agent.
type AgentHeartbeat struct {
	AgentID   string           `json:"agent_id"`
	Payload   *HeartbeatPayload `json:"payload,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// HeartbeatPayload contains heartbeat information from an agent.
type HeartbeatPayload struct {
	Status       string                 `json:"status"`
	CurrentLoad  int                    `json:"current_load"`
	WorkItems    []string               `json:"work_items,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// NewWorkQueue creates a new work queue.
func NewWorkQueue(logger *slog.Logger) *WorkQueue {
	return &WorkQueue{
		items:   make([]*WorkItem, 0),
		byID:    make(map[string]*WorkItem),
		byRepo:  make(map[string][]*WorkItem),
		byIssue: make(map[string]*WorkItem),
		logger:  logger,
	}
}

// Enqueue adds a work item to the queue with conflict detection.
func (wq *WorkQueue) Enqueue(item *WorkItem) bool {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	
	// Check for conflicts
	if wq.hasConflicts(item) {
		wq.logger.Warn("work item conflicts with existing work", "work_id", item.ID, "type", item.Type)
		return false
	}
	
	// Add to queue
	wq.addItem(item)
	wq.logger.Info("work item enqueued", "work_id", item.ID, "priority", item.Priority, "queue_size", len(wq.items))
	
	return true
}

// Dequeue removes and returns the highest priority work item.
func (wq *WorkQueue) Dequeue() *WorkItem {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	
	if len(wq.items) == 0 {
		return nil
	}
	
	// Find highest priority item
	maxPriority := -1
	maxIndex := -1
	
	for i, item := range wq.items {
		if item.Priority > maxPriority {
			maxPriority = item.Priority
			maxIndex = i
		}
	}
	
	if maxIndex == -1 {
		return nil
	}
	
	// Remove item
	item := wq.items[maxIndex]
	wq.removeItem(maxIndex)
	
	wq.logger.Debug("work item dequeued", "work_id", item.ID, "priority", item.Priority)
	return item
}

// Peek returns the highest priority work item without removing it.
func (wq *WorkQueue) Peek() *WorkItem {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	
	if len(wq.items) == 0 {
		return nil
	}
	
	maxPriority := -1
	var maxItem *WorkItem
	
	for _, item := range wq.items {
		if item.Priority > maxPriority {
			maxPriority = item.Priority
			maxItem = item
		}
	}
	
	return maxItem
}

// Remove removes a specific work item from the queue.
func (wq *WorkQueue) Remove(workID string) bool {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	
	for i, item := range wq.items {
		if item.ID == workID {
			wq.removeItem(i)
			wq.logger.Debug("work item removed from queue", "work_id", workID)
			return true
		}
	}
	
	return false
}

// Size returns the current size of the queue.
func (wq *WorkQueue) Size() int {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	
	return len(wq.items)
}

// List returns all work items in the queue.
func (wq *WorkQueue) List() []*WorkItem {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	
	items := make([]*WorkItem, len(wq.items))
	copy(items, wq.items)
	return items
}

// GetByRepo returns work items for a specific repository.
func (wq *WorkQueue) GetByRepo(owner, repo string) []*WorkItem {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	
	repoKey := owner + "/" + repo
	items := wq.byRepo[repoKey]
	
	result := make([]*WorkItem, len(items))
	copy(result, items)
	return result
}

// Clear removes all items from the queue.
func (wq *WorkQueue) Clear() {
	wq.mu.Lock()
	defer wq.mu.Unlock()
	
	wq.items = wq.items[:0]
	wq.byID = make(map[string]*WorkItem)
	wq.byRepo = make(map[string][]*WorkItem)
	wq.byIssue = make(map[string]*WorkItem)
	
	wq.logger.Info("work queue cleared")
}

// hasConflicts checks if the work item conflicts with existing work.
func (wq *WorkQueue) hasConflicts(item *WorkItem) bool {
	// Check for duplicate ID
	if _, exists := wq.byID[item.ID]; exists {
		return true
	}
	
	// Check for same issue conflict
	if item.Type == "github_issue" && item.IssueNumber > 0 {
		issueKey := item.GitHubOwner + "/" + item.GitHubRepo + ":" + string(rune(item.IssueNumber))
		if _, exists := wq.byIssue[issueKey]; exists {
			return true
		}
	}
	
	// Check for same PR conflict
	if item.Type == "github_pr" && item.PRNumber > 0 {
		prKey := item.GitHubOwner + "/" + item.GitHubRepo + ":pr:" + string(rune(item.PRNumber))
		if _, exists := wq.byIssue[prKey]; exists {
			return true
		}
	}
	
	return false
}

// addItem adds a work item to all internal data structures.
func (wq *WorkQueue) addItem(item *WorkItem) {
	// Add to main queue
	wq.items = append(wq.items, item)
	
	// Add to ID index
	wq.byID[item.ID] = item
	
	// Add to repo index
	if item.GitHubOwner != "" && item.GitHubRepo != "" {
		repoKey := item.GitHubOwner + "/" + item.GitHubRepo
		wq.byRepo[repoKey] = append(wq.byRepo[repoKey], item)
	}
	
	// Add to issue/PR index
	if item.Type == "github_issue" && item.IssueNumber > 0 {
		issueKey := item.GitHubOwner + "/" + item.GitHubRepo + ":" + string(rune(item.IssueNumber))
		wq.byIssue[issueKey] = item
	} else if item.Type == "github_pr" && item.PRNumber > 0 {
		prKey := item.GitHubOwner + "/" + item.GitHubRepo + ":pr:" + string(rune(item.PRNumber))
		wq.byIssue[prKey] = item
	}
}

// removeItem removes a work item from all internal data structures by index.
func (wq *WorkQueue) removeItem(index int) {
	item := wq.items[index]
	
	// Remove from main queue
	wq.items = append(wq.items[:index], wq.items[index+1:]...)
	
	// Remove from ID index
	delete(wq.byID, item.ID)
	
	// Remove from repo index
	if item.GitHubOwner != "" && item.GitHubRepo != "" {
		repoKey := item.GitHubOwner + "/" + item.GitHubRepo
		repoItems := wq.byRepo[repoKey]
		
		for i, repoItem := range repoItems {
			if repoItem.ID == item.ID {
				wq.byRepo[repoKey] = append(repoItems[:i], repoItems[i+1:]...)
				break
			}
		}
		
		// Clean up empty repo entries
		if len(wq.byRepo[repoKey]) == 0 {
			delete(wq.byRepo, repoKey)
		}
	}
	
	// Remove from issue/PR index
	if item.Type == "github_issue" && item.IssueNumber > 0 {
		issueKey := item.GitHubOwner + "/" + item.GitHubRepo + ":" + string(rune(item.IssueNumber))
		delete(wq.byIssue, issueKey)
	} else if item.Type == "github_pr" && item.PRNumber > 0 {
		prKey := item.GitHubOwner + "/" + item.GitHubRepo + ":pr:" + string(rune(item.PRNumber))
		delete(wq.byIssue, prKey)
	}
}

// GetStats returns queue statistics.
func (wq *WorkQueue) GetStats() map[string]interface{} {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	
	typeCount := make(map[string]int)
	priorityCount := make(map[int]int)
	repoCount := len(wq.byRepo)
	
	for _, item := range wq.items {
		typeCount[item.Type]++
		priorityCount[item.Priority]++
	}
	
	return map[string]interface{}{
		"total_items":     len(wq.items),
		"items_by_type":   typeCount,
		"items_by_priority": priorityCount,
		"repositories":    repoCount,
	}
}

// MarshalJSON implements json.Marshaler for WorkItem.
func (wi *WorkItem) MarshalJSON() ([]byte, error) {
	type Alias WorkItem
	return json.Marshal(&struct {
		*Alias
		EstimatedDurationSeconds int64 `json:"estimated_duration_seconds,omitempty"`
	}{
		Alias: (*Alias)(wi),
		EstimatedDurationSeconds: int64(wi.EstimatedDuration.Seconds()),
	})
}

// UnmarshalJSON implements json.Unmarshaler for WorkItem.
func (wi *WorkItem) UnmarshalJSON(data []byte) error {
	type Alias WorkItem
	aux := &struct {
		*Alias
		EstimatedDurationSeconds int64 `json:"estimated_duration_seconds,omitempty"`
	}{
		Alias: (*Alias)(wi),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	if aux.EstimatedDurationSeconds > 0 {
		wi.EstimatedDuration = time.Duration(aux.EstimatedDurationSeconds) * time.Second
	}
	
	return nil
}

// MarshalJSON implements json.Marshaler for WorkResult.
func (wr *WorkResult) MarshalJSON() ([]byte, error) {
	type Alias WorkResult
	return json.Marshal(&struct {
		*Alias
		DurationSeconds int64 `json:"duration_seconds"`
	}{
		Alias:           (*Alias)(wr),
		DurationSeconds: int64(wr.Duration.Seconds()),
	})
}

// UnmarshalJSON implements json.Unmarshaler for WorkResult.
func (wr *WorkResult) UnmarshalJSON(data []byte) error {
	type Alias WorkResult
	aux := &struct {
		*Alias
		DurationSeconds int64 `json:"duration_seconds"`
	}{
		Alias: (*Alias)(wr),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	wr.Duration = time.Duration(aux.DurationSeconds) * time.Second
	
	return nil
}