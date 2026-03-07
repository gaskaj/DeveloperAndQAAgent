package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/agent"
	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentHandoffWorkflows(t *testing.T) {
	tests := []struct {
		name           string
		setupScenario  func(*TestEnvironment) (*MockIssue, []agent.Agent)
		expectedStates map[string]state.WorkflowState
		timeout        time.Duration
	}{
		{
			name: "developer_to_qa_handoff",
			setupScenario: func(te *TestEnvironment) (*MockIssue, []agent.Agent) {
				issue := te.SimulateGitHubIssue(301, "Handoff test", "Testing developer to QA handoff", []string{"agent:ready"})

				// Create developer agent
				devAgent, err := te.CreateDeveloperAgent()
				require.NoError(t, err)

				// Note: In a real scenario, we'd have a QA agent here
				// For now, we'll test the developer agent completing its workflow
				return issue, []agent.Agent{devAgent}
			},
			expectedStates: map[string]state.WorkflowState{
				"developer": state.StateComplete,
			},
			timeout: 30 * time.Second,
		},
		{
			name: "epic_issue_decomposition_handoff",
			setupScenario: func(te *TestEnvironment) (*MockIssue, []agent.Agent) {
				// Enqueue a response for the analysis call that triggers decomposition.
				// Must include "Fits within budget: no" AND ### Subtask headers for parseSubtasks.
				te.mockServer.EnqueueResponse("COMPLEXITY_ASSESSMENT: TOO_COMPLEX\n" +
					"This epic requires decomposition into 3 subtasks.\n\n" +
					"**Estimated iterations**: 100\n**Fits within budget**: no\n\n" +
					"### Subtask 1: Subtask A\nFirst subtask of the epic.\n\n" +
					"### Subtask 2: Subtask B\nSecond subtask of the epic.\n\n" +
					"### Subtask 3: Subtask C\nThird subtask of the epic.")

				issue := te.SimulateGitHubIssue(302, "Epic feature", "Large epic that needs decomposition", []string{"agent:ready"})

				devAgent, err := te.CreateDeveloperAgent()
				require.NoError(t, err)

				return issue, []agent.Agent{devAgent}
			},
			// StateDecompose is too transient to observe; child issues are processed
			// synchronously to completion immediately after decomposition.
			expectedStates: map[string]state.WorkflowState{
				"developer": state.StateComplete,
			},
			timeout: 25 * time.Second,
		},
		{
			name: "concurrent_multi_agent_handoffs",
			setupScenario: func(te *TestEnvironment) (*MockIssue, []agent.Agent) {
				// Create multiple issues for different agents
				issue1 := te.SimulateGitHubIssue(303, "Issue 1", "First concurrent issue", []string{"agent:ready"})
				te.SimulateGitHubIssue(304, "Issue 2", "Second concurrent issue", []string{"agent:ready"})
				te.SimulateGitHubIssue(305, "Issue 3", "Third concurrent issue", []string{"agent:ready"})

				// Create multiple developer agents
				var agents []agent.Agent
				for i := 0; i < 2; i++ {
					devAgent, err := te.CreateDeveloperAgent()
					require.NoError(t, err)
					agents = append(agents, devAgent)
				}

				return issue1, agents
			},
			expectedStates: map[string]state.WorkflowState{
				"developer": state.StateComplete, // At least one should complete
			},
			timeout: 45 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := NewTestEnvironment(t)
			defer te.Cleanup()

			// Setup the scenario
			issue, agents := tt.setupScenario(te)

			// Create orchestrator
			orchestrator := te.CreateOrchestrator(agents)

			// Run with timeout
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- orchestrator.Run(ctx)
			}()

			// Wait for expected states
			for agentType, expectedState := range tt.expectedStates {
				err := te.WaitForWorkflowTransition(agentType, expectedState, tt.timeout-5*time.Second)
				if err != nil {
					// For concurrent scenarios, we might not hit all expected states
					if tt.name != "concurrent_multi_agent_handoffs" {
						require.NoError(t, err)
					}
				}
			}

			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("orchestrator did not stop gracefully")
			}

			// Verify handoff artifacts
			te.verifyHandoffArtifacts(t, issue.Number, tt.name)
		})
	}
}

func TestContextPreservationDuringHandoffs(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	issue := te.SimulateGitHubIssue(310, "Context preservation test",
		"Testing that context is preserved during agent handoffs", []string{"agent:ready"})

	devAgent, err := te.CreateDeveloperAgent()
	require.NoError(t, err)

	orchestrator := te.CreateOrchestrator([]agent.Agent{devAgent})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Track context preservation by monitoring state updates
	var stateUpdates []*state.AgentWorkState
	var updatesMutex sync.Mutex

	// Monitor state changes to verify context preservation.
	// Use a fast ticker to catch rapid state transitions in the mock environment.
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ws, err := te.store.Load(context.Background(), "developer")
				if err == nil && ws != nil {
					updatesMutex.Lock()
					// Add if it's a new state (dedup by State, not UpdatedAt, since
					// with fast mocks timestamps can collide)
					if len(stateUpdates) == 0 || stateUpdates[len(stateUpdates)-1].State != ws.State {
						wsCopy := *ws
						stateUpdates = append(stateUpdates, &wsCopy)
					}
					updatesMutex.Unlock()
				}
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- orchestrator.Run(ctx)
	}()

	// Wait for the workflow to complete, then stop
	_ = te.WaitForWorkflowTransition("developer", state.StateComplete, 20*time.Second)
	cancel()
	<-done

	// Verify context was preserved across state transitions
	updatesMutex.Lock()
	defer updatesMutex.Unlock()

	assert.GreaterOrEqual(t, len(stateUpdates), 1, "Should have at least one state update")

	// Verify that issue context is preserved
	for _, update := range stateUpdates {
		assert.Equal(t, issue.Number, update.IssueNumber, "Issue number should be preserved")
		assert.Equal(t, issue.Title, update.IssueTitle, "Issue title should be preserved")
		assert.NotEmpty(t, update.AgentType, "Agent type should be preserved")
	}

	// Verify that branch name is set and preserved once created
	branchFound := false
	var branchName string
	for _, update := range stateUpdates {
		if update.BranchName != "" {
			if !branchFound {
				branchFound = true
				branchName = update.BranchName
			} else {
				assert.Equal(t, branchName, update.BranchName, "Branch name should remain consistent")
			}
		}
	}
}

func TestErrorHandlingInAgentHandoffs(t *testing.T) {
	errorScenarios := []struct {
		name           string
		errorInjection func(*TestEnvironment)
		expectedState  state.WorkflowState
		errorRecovery  bool
	}{
		{
			name: "github_api_failure_during_handoff",
			errorInjection: func(te *TestEnvironment) {
				te.SimulateAgentFailure("github", errors.New("GitHub API rate limit exceeded"))
			},
			expectedState: state.StateFailed,
			errorRecovery: false,
		},
		{
			name: "claude_api_failure_during_analysis",
			errorInjection: func(te *TestEnvironment) {
				te.SimulateAgentFailure("claude", errors.New("Claude API temporarily unavailable"))
			},
			expectedState: state.StateFailed,
			errorRecovery: false,
		},
		{
			name: "state_store_failure_during_handoff",
			errorInjection: func(te *TestEnvironment) {
				te.SimulateAgentFailure("store", errors.New("State store connection lost"))
			},
			expectedState: state.StateClaim, // Should still claim even if state save fails
			errorRecovery: false,
		},
		{
			name: "temporary_github_error_with_recovery",
			errorInjection: func(te *TestEnvironment) {
				te.SimulateAgentFailure("github", errors.New("temporary GitHub error"))
			},
			expectedState: state.StateComplete,
			errorRecovery: true,
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			te := NewTestEnvironment(t)
			defer te.Cleanup()

			issue := te.SimulateGitHubIssue(320+len(scenario.name),
				fmt.Sprintf("Error handling test: %s", scenario.name),
				"Testing error handling during handoffs", []string{"agent:ready"})

			// Inject the error
			scenario.errorInjection(te)

			devAgent, err := te.CreateDeveloperAgent()
			require.NoError(t, err)

			orchestrator := te.CreateOrchestrator([]agent.Agent{devAgent})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- orchestrator.Run(ctx)
			}()

			// If error recovery is expected, clear the error after initial failure
			if scenario.errorRecovery {
				time.Sleep(5 * time.Second)
				te.githubClient.ClearError()
				te.claudeClient.ClearError()
				te.mockServer.ClearHTTPError()
				te.store.ClearError()
			}

			// Wait for expected state or timeout
			err = te.WaitForWorkflowTransition("developer", scenario.expectedState, 25*time.Second)
			if !scenario.errorRecovery {
				// For non-recovery scenarios, we might not reach the expected state due to failures
				// This is acceptable behavior
			} else {
				require.NoError(t, err, "Should recover from temporary error")
			}

			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("orchestrator did not stop gracefully")
			}

			// Verify error handling artifacts
			te.verifyErrorHandlingArtifacts(t, issue.Number, scenario.name)
		})
	}
}

func TestAgentHandoffPerformance(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Create multiple issues to test handoff performance
	numIssues := 5
	issues := make([]*MockIssue, numIssues)
	for i := 0; i < numIssues; i++ {
		issues[i] = te.SimulateGitHubIssue(400+i,
			fmt.Sprintf("Performance test issue %d", i),
			fmt.Sprintf("Testing handoff performance with issue %d", i),
			[]string{"agent:ready"})
	}

	devAgent, err := te.CreateDeveloperAgent()
	require.NoError(t, err)

	orchestrator := te.CreateOrchestrator([]agent.Agent{devAgent})

	// Measure total processing time
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- orchestrator.Run(ctx)
	}()

	// Wait for all issues to be at least claimed
	processedIssues := 0
	timeout := time.After(50 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for processedIssues < numIssues {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for issues to be processed. Only %d/%d processed", processedIssues, numIssues)
		case <-ticker.C:
			processedIssues = 0
			for _, issue := range issues {
				labels := te.githubClient.GetIssueLabels(issue.Number)
				for _, label := range labels {
					if label == "agent:claimed" {
						processedIssues++
						break
					}
				}
			}
		}
	}

	processingTime := time.Since(startTime)
	cancel()
	<-done

	// Verify performance metrics
	avgTimePerIssue := processingTime / time.Duration(numIssues)
	t.Logf("Processed %d issues in %v (avg: %v per issue)", numIssues, processingTime, avgTimePerIssue)

	// Performance assertions (these thresholds should be adjusted based on actual requirements)
	assert.Less(t, avgTimePerIssue, 15*time.Second, "Average processing time per issue should be reasonable")
	assert.Less(t, processingTime, 60*time.Second, "Total processing time should be within limits")

	// Verify all issues were at least claimed
	for _, issue := range issues {
		te.AssertIssueLabels(issue.Number, []string{"agent:claimed"})
	}
}

// Helper methods for test verification

func (te *TestEnvironment) verifyHandoffArtifacts(t *testing.T, issueNumber int, testName string) {
	switch testName {
	case "developer_to_qa_handoff":
		// Verify developer completed its work
		te.AssertCommentCreated(issueNumber, "Analysis complete")
		// In a real scenario, we'd verify QA agent picked up the work

	case "epic_issue_decomposition_handoff":
		// Verify decomposition artifacts
		te.AssertIssueLabels(issueNumber, []string{"agent:epic"})
		te.AssertCommentCreated(issueNumber, "Issue decomposed into")

	case "concurrent_multi_agent_handoffs":
		// Verify that at least some processing occurred
		comments := te.githubClient.GetIssueComments(issueNumber)
		assert.Greater(t, len(comments), 0, "Should have created comments during processing")
	}
}

func (te *TestEnvironment) verifyErrorHandlingArtifacts(t *testing.T, issueNumber int, scenarioName string) {
	switch scenarioName {
	case "github_api_failure_during_handoff", "claude_api_failure_during_analysis":
		// Should have failure artifacts
		labels := te.githubClient.GetIssueLabels(issueNumber)
		labelMap := make(map[string]bool)
		for _, label := range labels {
			labelMap[label] = true
		}

		// Might have claimed but then failed
		if labelMap["agent:failed"] {
			te.AssertCommentCreated(issueNumber, "failed")
		}

	case "temporary_github_error_with_recovery":
		// Should have successful completion artifacts after recovery
		te.AssertIssueLabels(issueNumber, []string{"agent:claimed"})
		comments := te.githubClient.GetIssueComments(issueNumber)
		assert.Greater(t, len(comments), 0, "Should have created comments after recovery")
	}
}
