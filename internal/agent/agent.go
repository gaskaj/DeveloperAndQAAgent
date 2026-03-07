package agent

import (
	"context"
	"log/slog"

	"github.com/gaskaj/OpenAgentFramework/internal/claude"
	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/gaskaj/OpenAgentFramework/internal/errors"
	"github.com/gaskaj/OpenAgentFramework/internal/ghub"
	"github.com/gaskaj/OpenAgentFramework/internal/observability"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
)

// AgentType identifies the kind of agent.
type AgentType string

const (
	TypeDeveloper  AgentType = "developer"
	TypeQA         AgentType = "qa"
	TypeDevManager AgentType = "devmanager"
)

// Agent is the interface that all agent types must implement.
type Agent interface {
	// Type returns the agent type identifier.
	Type() AgentType

	// Run starts the agent's main loop. It blocks until the context is cancelled.
	Run(ctx context.Context) error

	// Status returns the agent's current status report.
	Status() StatusReport
}

// StatusReport describes the current state of an agent.
type StatusReport struct {
	Type           AgentType       `json:"type"`
	State          string          `json:"state"`
	IssueID        int             `json:"issue_id,omitempty"`
	Message        string          `json:"message"`
	WorkspaceStats *WorkspaceStats `json:"workspace_stats,omitempty"`
}

// WorkspaceStats represents workspace usage statistics for the status report.
type WorkspaceStats struct {
	TotalWorkspaces  int   `json:"total_workspaces"`
	ActiveWorkspaces int   `json:"active_workspaces"`
	TotalSizeMB      int64 `json:"total_size_mb"`
	DiskFreeMB       int64 `json:"disk_free_mb"`
}

// Dependencies holds shared dependencies injected into agents.
type Dependencies struct {
	Config           *config.Config
	GitHub           ghub.Client
	Claude           *claude.Client
	Store            state.Store
	Logger           *slog.Logger
	StructuredLogger *observability.StructuredLogger
	Metrics          *observability.Metrics
	ErrorManager     *errors.Manager
}

// BaseAgent provides common functionality for all agents.
type BaseAgent struct {
	Deps Dependencies
}

// NewBaseAgent creates a BaseAgent with the given dependencies.
func NewBaseAgent(deps Dependencies) BaseAgent {
	return BaseAgent{Deps: deps}
}
