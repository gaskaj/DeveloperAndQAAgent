package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicAgentLifecycle tests the basic agent lifecycle without complex mocks
func TestBasicAgentLifecycle(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Create a simple issue
	_ = te.SimulateGitHubIssue(123, "Simple test", "Basic agent lifecycle test", []string{"agent:ready"})

	// Create agent
	devAgent, err := te.CreateDeveloperAgent()
	require.NoError(t, err)

	// Verify agent type
	assert.Equal(t, "developer", string(devAgent.Type()))

	// Test basic status reporting
	status := devAgent.Status()
	assert.Equal(t, "developer", string(status.Type))
	assert.Equal(t, "waiting for issues", status.Message)

	// Verify GitHub mock is working
	labels := te.githubClient.GetIssueLabels(123)
	assert.Contains(t, labels, "agent:ready")

	// Verify store mock is working
	ctx := context.Background()
	testState := &state.AgentWorkState{
		AgentType:   "test-agent",
		IssueNumber: 999,
		State:       state.StateClaim,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = te.store.Save(ctx, testState)
	require.NoError(t, err)

	loadedState, err := te.store.Load(ctx, "test-agent")
	require.NoError(t, err)
	require.NotNil(t, loadedState)
	assert.Equal(t, testState.IssueNumber, loadedState.IssueNumber)
	assert.Equal(t, testState.State, loadedState.State)
}

// TestGitHubMockOperations tests that our GitHub mock works correctly
func TestGitHubMockOperations(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	ctx := context.Background()
	issueNumber := 456

	// Create an issue
	_ = te.SimulateGitHubIssue(issueNumber, "Mock test", "Testing GitHub mock", []string{"agent:ready", "bug"})

	// Test ListIssuesByLabels
	issues, err := te.githubClient.ListIssuesByLabels(ctx, []string{"agent:ready"})
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, issueNumber, *issues[0].Number)
	assert.Equal(t, "Mock test", *issues[0].Title)

	// Test AddLabels
	err = te.githubClient.AddLabels(ctx, issueNumber, []string{"agent:claimed"})
	require.NoError(t, err)
	te.AssertIssueLabels(issueNumber, []string{"agent:ready", "bug", "agent:claimed"})

	// Test RemoveLabel
	err = te.githubClient.RemoveLabel(ctx, issueNumber, "agent:ready")
	require.NoError(t, err)
	labels := te.githubClient.GetIssueLabels(issueNumber)
	assert.NotContains(t, labels, "agent:ready")
	assert.Contains(t, labels, "agent:claimed")

	// Test CreateComment
	err = te.githubClient.CreateComment(ctx, issueNumber, "Test comment")
	require.NoError(t, err)
	te.AssertCommentCreated(issueNumber, "Test comment")

	// Test AssignIssue
	err = te.githubClient.AssignIssue(ctx, issueNumber, []string{"test-user"})
	require.NoError(t, err)

	// Verify assignment via GetIssue
	retrievedIssue, err := te.githubClient.GetIssue(ctx, issueNumber)
	require.NoError(t, err)
	require.Len(t, retrievedIssue.Assignees, 1)
	assert.Equal(t, "test-user", *retrievedIssue.Assignees[0].Login)
}

// TestStateMockOperations tests that our state store mock works correctly
func TestStateMockOperations(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	ctx := context.Background()

	// Test Save and Load
	state1 := &state.AgentWorkState{
		AgentType:   "developer",
		IssueNumber: 100,
		IssueTitle:  "Test Issue 1",
		State:       state.StateAnalyze,
		BranchName:  "feature-branch-1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := te.store.Save(ctx, state1)
	require.NoError(t, err)

	loaded, err := te.store.Load(ctx, "developer")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, state1.IssueNumber, loaded.IssueNumber)
	assert.Equal(t, state1.State, loaded.State)
	assert.Equal(t, state1.BranchName, loaded.BranchName)

	// Test multiple agents
	state2 := &state.AgentWorkState{
		AgentType:   "qa",
		IssueNumber: 200,
		IssueTitle:  "Test Issue 2",
		State:       state.StateImplement,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = te.store.Save(ctx, state2)
	require.NoError(t, err)

	// Test List
	allStates, err := te.store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, allStates, 2)

	// Verify both states are present
	stateMap := make(map[string]*state.AgentWorkState)
	for _, s := range allStates {
		stateMap[s.AgentType] = s
	}

	assert.Contains(t, stateMap, "developer")
	assert.Contains(t, stateMap, "qa")
	assert.Equal(t, 100, stateMap["developer"].IssueNumber)
	assert.Equal(t, 200, stateMap["qa"].IssueNumber)

	// Test Delete
	err = te.store.Delete(ctx, "developer")
	require.NoError(t, err)

	deletedState, err := te.store.Load(ctx, "developer")
	require.NoError(t, err)
	assert.Nil(t, deletedState)

	// qa state should still exist
	qaState, err := te.store.Load(ctx, "qa")
	require.NoError(t, err)
	require.NotNil(t, qaState)
	assert.Equal(t, 200, qaState.IssueNumber)
}

// TestConcurrentStateOperations tests concurrent access to the state store
func TestConcurrentStateOperations(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	ctx := context.Background()
	const numGoroutines = 10

	// Test concurrent saves
	err := te.SimulateConcurrentAccess(numGoroutines, func(id int) error {
		state := &state.AgentWorkState{
			AgentType:   fmt.Sprintf("agent-%d", id),
			IssueNumber: 300 + id,
			IssueTitle:  fmt.Sprintf("Concurrent test %d", id),
			State:       state.StateClaim,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		return te.store.Save(ctx, state)
	})
	require.NoError(t, err)

	// Verify all states were saved
	allStates, err := te.store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, allStates, numGoroutines)

	// Test concurrent reads
	err = te.SimulateConcurrentAccess(numGoroutines, func(id int) error {
		_, err := te.store.Load(ctx, fmt.Sprintf("agent-%d", id))
		return err
	})
	require.NoError(t, err)
}

// TestErrorSimulation tests that error simulation works correctly
func TestErrorSimulation(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	ctx := context.Background()

	// Test GitHub error simulation
	te.SimulateAgentFailure("github", fmt.Errorf("simulated GitHub error"))

	_, err := te.githubClient.ListIssuesByLabels(ctx, []string{"test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated GitHub error")

	// Clear error and verify recovery
	te.githubClient.ClearError()
	_, err = te.githubClient.ListIssuesByLabels(ctx, []string{"test"})
	assert.NoError(t, err)

	// Test state store error simulation
	te.SimulateAgentFailure("store", fmt.Errorf("simulated store error"))

	testState := &state.AgentWorkState{
		AgentType: "test",
		State:     state.StateIdle,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = te.store.Save(ctx, testState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated store error")

	// Clear error and verify recovery
	te.store.ClearError()
	err = te.store.Save(ctx, testState)
	assert.NoError(t, err)
}
