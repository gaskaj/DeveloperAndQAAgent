package creativity

import (
	"context"
	"fmt"
)

const (
	labelReady              = "agent:ready"
	labelSuggestion         = "agent:suggestion"
	labelSuggestionRejected = "agent:suggestion-rejected"
)

// checkForAvailableWork returns true if there are open issues with the agent:ready label.
func (e *CreativityEngine) checkForAvailableWork(ctx context.Context) (bool, error) {
	issues, err := e.gh.ListIssuesByLabel(ctx, labelReady)
	if err != nil {
		return false, fmt.Errorf("checking for available work: %w", err)
	}
	return len(issues) > 0, nil
}

// hasPendingSuggestion returns true if the number of open suggestion issues
// is at or above the configured maximum.
func (e *CreativityEngine) hasPendingSuggestion(ctx context.Context) (bool, error) {
	issues, err := e.gh.ListIssuesByLabel(ctx, labelSuggestion)
	if err != nil {
		return false, fmt.Errorf("checking pending suggestions: %w", err)
	}
	return len(issues) >= e.cfg.MaxPendingSuggestions, nil
}
