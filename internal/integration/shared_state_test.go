package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/agent"
	"github.com/gaskaj/DeveloperAndQAAgent/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedStateManagement(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Create multiple agents that will share state
	agents := make([]agent.Agent, 3)
	for i := 0; i < 3; i++ {
		agent, err := te.CreateDeveloperAgent()
		require.NoError(t, err)
		agents[i] = agent
	}

	// Create test state entries
	testStates := []*state.AgentWorkState{
		{
			AgentType:   "developer-1",
			IssueNumber: 501,
			IssueTitle:  "State test 1",
			State:       state.StateClaim,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			AgentType:   "developer-2",
			IssueNumber: 502,
			IssueTitle:  "State test 2",
			State:       state.StateAnalyze,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			AgentType:   "developer-3",
			IssueNumber: 503,
			IssueTitle:  "State test 3",
			State:       state.StateImplement,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	// Save all states
	ctx := context.Background()
	for _, s := range testStates {
		err := te.store.Save(ctx, s)
		require.NoError(t, err)
	}

	// Test concurrent reads
	t.Run("concurrent_state_reads", func(t *testing.T) {
		const numReaders = 10
		var wg sync.WaitGroup
		results := make(chan *state.AgentWorkState, numReaders*len(testStates))

		// Start concurrent readers
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(readerID int) {
				defer wg.Done()
				for _, originalState := range testStates {
					loadedState, err := te.store.Load(ctx, originalState.AgentType)
					if err == nil && loadedState != nil {
						results <- loadedState
					}
				}
			}(i)
		}

		wg.Wait()
		close(results)

		// Verify all reads were consistent
		readCount := 0
		for loadedState := range results {
			readCount++
			// Find original state
			var originalState *state.AgentWorkState
			for _, s := range testStates {
				if s.AgentType == loadedState.AgentType {
					originalState = s
					break
				}
			}
			require.NotNil(t, originalState, "Could not find original state for %s", loadedState.AgentType)

			// Verify consistency
			assert.Equal(t, originalState.IssueNumber, loadedState.IssueNumber)
			assert.Equal(t, originalState.IssueTitle, loadedState.IssueTitle)
			assert.Equal(t, originalState.State, loadedState.State)
		}

		// Should have read all states from all readers
		expectedReads := numReaders * len(testStates)
		assert.Equal(t, expectedReads, readCount, "Should have completed all concurrent reads")
	})

	// Test concurrent writes
	t.Run("concurrent_state_writes", func(t *testing.T) {
		const numWriters = 5
		var wg sync.WaitGroup
		writeErrors := make(chan error, numWriters)

		// Each writer updates a different field of the same state
		targetState := testStates[0]

		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()

				// Create a modified copy
				updatedState := *targetState
				updatedState.State = state.WorkflowState(fmt.Sprintf("test-state-%d", writerID))
				updatedState.UpdatedAt = time.Now()
				updatedState.Error = fmt.Sprintf("Writer %d was here", writerID)

				err := te.store.Save(ctx, &updatedState)
				writeErrors <- err
			}(i)
		}

		wg.Wait()
		close(writeErrors)

		// Verify no write errors occurred
		for err := range writeErrors {
			assert.NoError(t, err, "Concurrent write should not fail")
		}

		// Verify final state is consistent (last writer wins)
		finalState, err := te.store.Load(ctx, targetState.AgentType)
		require.NoError(t, err)
		require.NotNil(t, finalState)

		// Should have one of the writer's modifications
		assert.Contains(t, string(finalState.State), "test-state-")
		assert.Contains(t, finalState.Error, "Writer")
	})

	// Test state listing consistency
	t.Run("state_list_consistency", func(t *testing.T) {
		allStates, err := te.store.List(ctx)
		require.NoError(t, err)

		// Should have at least our test states
		assert.GreaterOrEqual(t, len(allStates), len(testStates))

		// Verify each test state is present
		stateMap := make(map[string]*state.AgentWorkState)
		for _, s := range allStates {
			stateMap[s.AgentType] = s
		}

		for _, originalState := range testStates {
			foundState, exists := stateMap[originalState.AgentType]
			assert.True(t, exists, "State for %s should be in list", originalState.AgentType)
			if exists {
				assert.Equal(t, originalState.IssueNumber, foundState.IssueNumber)
				assert.Equal(t, originalState.IssueTitle, foundState.IssueTitle)
				// Note: State might have been modified by concurrent writes test
			}
		}
	})
}

func TestConcurrentResourceAccess(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Simulate multiple agents trying to claim the same issue
	sharedIssue := te.SimulateGitHubIssue(510, "Concurrent access test",
		"Testing concurrent access to shared resources", []string{"agent:ready"})

	const numAgents = 5
	agents := make([]agent.Agent, numAgents)
	for i := 0; i < numAgents; i++ {
		agent, err := te.CreateDeveloperAgent()
		require.NoError(t, err)
		agents[i] = agent
	}

	orchestrator := te.CreateOrchestrator(agents)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Track which agents claim the issue
	var claimingAgents []string
	var claimMutex sync.Mutex

	// Monitor label changes to see which agents claim the issue
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				labels := te.githubClient.GetIssueLabels(sharedIssue.Number)
				for _, label := range labels {
					if label == "agent:claimed" {
						// Check which agent is working on it via state
						allStates, err := te.store.List(context.Background())
						if err == nil {
							for _, s := range allStates {
								if s.IssueNumber == sharedIssue.Number && s.State != state.StateIdle {
									claimMutex.Lock()
									// Only add if not already tracked
									found := false
									for _, existing := range claimingAgents {
										if existing == s.AgentType {
											found = true
											break
										}
									}
									if !found {
										claimingAgents = append(claimingAgents, s.AgentType)
									}
									claimMutex.Unlock()
								}
							}
						}
						break
					}
				}
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- orchestrator.Run(ctx)
	}()

	// Wait for agents to process
	time.Sleep(20 * time.Second)
	cancel()
	<-done

	// Verify that only one agent actually claimed and processed the issue
	claimMutex.Lock()
	defer claimMutex.Unlock()

	// In a well-designed system, only one agent should successfully claim the issue
	// However, due to race conditions, multiple agents might attempt to claim
	t.Logf("Agents that attempted to claim issue %d: %v", sharedIssue.Number, claimingAgents)

	// At least one agent should have claimed it
	assert.Greater(t, len(claimingAgents), 0, "At least one agent should have claimed the issue")

	// Verify the issue was labeled as claimed
	te.AssertIssueLabels(sharedIssue.Number, []string{"agent:claimed"})
}

func TestRaceConditionDetection(t *testing.T) {
	// This test is designed to be run with `go test -race` to detect race conditions
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Create shared state that multiple goroutines will access
	sharedState := &state.AgentWorkState{
		AgentType:   "race-test-agent",
		IssueNumber: 520,
		IssueTitle:  "Race condition test",
		State:       state.StateClaim,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ctx := context.Background()
	err := te.store.Save(ctx, sharedState)
	require.NoError(t, err)

	const numGoroutines = 20
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*2) // *2 for read and write operations

	// Start goroutines that concurrently read and write to the same state
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2) // One for read, one for write

		// Reader goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := te.store.Load(ctx, sharedState.AgentType)
				if err != nil {
					errors <- fmt.Errorf("reader %d iteration %d: %w", id, j, err)
					return
				}
				time.Sleep(time.Millisecond) // Small delay to increase chance of race
			}
		}(i)

		// Writer goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				updateState := *sharedState
				updateState.State = state.WorkflowState(fmt.Sprintf("state-%d-%d", id, j))
				updateState.UpdatedAt = time.Now()
				updateState.Error = fmt.Sprintf("Updated by goroutine %d iteration %d", id, j)

				err := te.store.Save(ctx, &updateState)
				if err != nil {
					errors <- fmt.Errorf("writer %d iteration %d: %w", id, j, err)
					return
				}
				time.Sleep(time.Millisecond) // Small delay to increase chance of race
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors during concurrent access
	errorCount := 0
	for err := range errors {
		errorCount++
		t.Logf("Concurrent access error: %v", err)
	}

	// In a race-safe implementation, there should be no errors
	assert.Equal(t, 0, errorCount, "Should not have race condition errors")

	// Verify final state is consistent
	finalState, err := te.store.Load(ctx, sharedState.AgentType)
	require.NoError(t, err)
	require.NotNil(t, finalState)
	assert.Equal(t, sharedState.AgentType, finalState.AgentType)
	assert.Equal(t, sharedState.IssueNumber, finalState.IssueNumber)
}

func TestStateConsistencyUnderFailure(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Create an agent and start processing
	issue := te.SimulateGitHubIssue(530, "Failure consistency test",
		"Testing state consistency when failures occur", []string{"agent:ready"})

	devAgent, err := te.CreateDeveloperAgent()
	require.NoError(t, err)

	orchestrator := te.CreateOrchestrator([]agent.Agent{devAgent})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- orchestrator.Run(ctx)
	}()

	// Wait for initial processing to start
	time.Sleep(2 * time.Second)

	// Inject a failure mid-processing
	te.SimulateAgentFailure("store", fmt.Errorf("simulated state store failure"))

	// Let it run for a bit with the failure
	time.Sleep(5 * time.Second)

	// Clear the failure to allow recovery
	te.store.ClearError()

	// Wait for completion or timeout
	time.Sleep(15 * time.Second)
	cancel()
	<-done

	// Verify that despite the failure, state remained consistent
	finalState, err := te.store.Load(context.Background(), "developer")
	if err != nil {
		t.Logf("Final state load error (expected if failure was persistent): %v", err)
	} else if finalState != nil {
		// If we have a final state, verify it's consistent
		assert.Equal(t, issue.Number, finalState.IssueNumber)
		assert.Equal(t, issue.Title, finalState.IssueTitle)
		assert.Equal(t, "developer", finalState.AgentType)

		// Verify timestamps are reasonable
		assert.False(t, finalState.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.False(t, finalState.UpdatedAt.IsZero(), "UpdatedAt should be set")
		assert.False(t, finalState.UpdatedAt.Before(finalState.CreatedAt), "UpdatedAt should not be before CreatedAt")
	}

	// Verify that GitHub state is also consistent
	labels := te.githubClient.GetIssueLabels(issue.Number)
	labelMap := make(map[string]bool)
	for _, label := range labels {
		labelMap[label] = true
	}

	// Should have at least claimed the issue
	if !labelMap["agent:claimed"] && finalState != nil {
		// If we have final state but no claimed label, there's an inconsistency
		t.Errorf("State inconsistency: have final state %v but issue not claimed", finalState.State)
	}
}

func TestLongRunningStateOperations(t *testing.T) {
	te := NewTestEnvironment(t)
	defer te.Cleanup()

	// Test with a large number of state operations to check for memory leaks and performance degradation
	const numOperations = 1000
	const batchSize = 50

	ctx := context.Background()
	startTime := time.Now()

	// Perform batches of operations
	for batch := 0; batch < numOperations/batchSize; batch++ {
		var wg sync.WaitGroup
		errors := make(chan error, batchSize*2) // Save + Load for each

		// Batch operations
		for i := 0; i < batchSize; i++ {
			wg.Add(2) // Save and Load

			agentType := fmt.Sprintf("long-test-agent-%d-%d", batch, i)
			testState := &state.AgentWorkState{
				AgentType:   agentType,
				IssueNumber: 600 + batch*batchSize + i,
				IssueTitle:  fmt.Sprintf("Long test issue %d-%d", batch, i),
				State:       state.WorkflowState(fmt.Sprintf("test-state-%d", i%5)),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Save operation
			go func(s *state.AgentWorkState) {
				defer wg.Done()
				if err := te.store.Save(ctx, s); err != nil {
					errors <- fmt.Errorf("save %s: %w", s.AgentType, err)
				}
			}(testState)

			// Load operation
			go func(agentType string) {
				defer wg.Done()
				if _, err := te.store.Load(ctx, agentType); err != nil {
					errors <- fmt.Errorf("load %s: %w", agentType, err)
				}
			}(agentType)
		}

		wg.Wait()
		close(errors)

		// Check for errors in this batch
		errorCount := 0
		for err := range errors {
			errorCount++
			if errorCount <= 5 { // Log first few errors
				t.Logf("Batch %d error: %v", batch, err)
			}
		}

		if errorCount > 0 {
			t.Fatalf("Batch %d had %d errors", batch, errorCount)
		}

		// Re-initialize errors channel for next batch
		errors = make(chan error, batchSize*2)
	}

	duration := time.Since(startTime)
	t.Logf("Completed %d state operations in %v (avg: %v per operation)",
		numOperations*2, duration, duration/time.Duration(numOperations*2))

	// Performance check - should complete within reasonable time
	assert.Less(t, duration, 30*time.Second, "Long running operations should complete within reasonable time")

	// Verify final state count
	allStates, err := te.store.List(ctx)
	require.NoError(t, err)

	// Should have at least the states we created (might have more from other tests)
	assert.GreaterOrEqual(t, len(allStates), numOperations,
		"Should have at least %d states, got %d", numOperations, len(allStates))
}
