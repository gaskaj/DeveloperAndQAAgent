package state

import "time"

// WorkflowState represents a step in the agent's workflow state machine.
type WorkflowState string

// State is an alias for WorkflowState for backward compatibility
type State = WorkflowState

const (
	StateIdle        WorkflowState = "idle"
	StateClaim       WorkflowState = "claim"
	StateAnalyze     WorkflowState = "analyze"
	StateWorkspace   WorkflowState = "workspace"
	StateImplement   WorkflowState = "implement"
	StateCommit      WorkflowState = "commit"
	StatePR          WorkflowState = "pr"
	StateValidation  WorkflowState = "validation"
	StateReview      WorkflowState = "review"
	StateComplete    WorkflowState = "complete"
	StateFailed      WorkflowState = "failed"
	StateCreativeThink WorkflowState = "creative_thinking"
	StateDecompose   WorkflowState = "decompose"
)

// AgentWorkState tracks the current work state of an agent.
type AgentWorkState struct {
	AgentType           string                 `json:"agent_type"`
	IssueNumber         int                    `json:"issue_number,omitempty"`
	IssueTitle          string                 `json:"issue_title,omitempty"`
	State               WorkflowState          `json:"state"`
	BranchName          string                 `json:"branch_name,omitempty"`
	WorkspaceDir        string                 `json:"workspace_dir,omitempty"`
	PRNumber            int                    `json:"pr_number,omitempty"`
	ParentIssue         int                    `json:"parent_issue,omitempty"`
	ChildIssues         []int                  `json:"child_issues,omitempty"`
	Error               string                 `json:"error,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at"`
	CreatedAt           time.Time              `json:"created_at"`
	// Checkpoint fields for graceful shutdown support
	CheckpointedAt      time.Time              `json:"checkpointed_at,omitempty"`
	CheckpointStage     string                 `json:"checkpoint_stage,omitempty"`
	CheckpointMetadata  map[string]interface{} `json:"checkpoint_metadata,omitempty"`
	InterruptedBy       string                 `json:"interrupted_by,omitempty"`
}
