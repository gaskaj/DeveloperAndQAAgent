package observability

import (
	"context"
	"testing"
	"time"

	"github.com/gaskaj/OpenAgentFramework/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	logger := &StructuredLogger{} // minimal logger for testing
	metrics := NewMetrics(logger)

	require.NotNil(t, metrics)
	assert.NotNil(t, metrics.counters)
	assert.NotNil(t, metrics.gauges)
	assert.NotNil(t, metrics.histograms)
	assert.Equal(t, logger, metrics.logger)
}

func TestCounterMetrics(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"agent": "developer", "status": "success"}

	t.Run("increment counter", func(t *testing.T) {
		metrics.Inc("test_counter", labels)
		counters := metrics.GetCounters()

		// Check that counter was incremented
		found := false
		for key, value := range counters {
			if value == 1 {
				found = true
				assert.Contains(t, key, "test_counter")
			}
		}
		assert.True(t, found, "Counter should be incremented")
	})

	t.Run("add to counter", func(t *testing.T) {
		metrics.Add("test_add_counter", 5, labels)
		counters := metrics.GetCounters()

		found := false
		for key, value := range counters {
			if value == 5 {
				found = true
				assert.Contains(t, key, "test_add_counter")
			}
		}
		assert.True(t, found, "Counter should be added to")
	})
}

func TestGaugeMetrics(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"type": "memory"}

	metrics.Set("test_gauge", 42.5, labels)
	gauges := metrics.GetGauges()

	found := false
	for key, value := range gauges {
		if value == 42.5 {
			found = true
			assert.Contains(t, key, "test_gauge")
		}
	}
	assert.True(t, found, "Gauge should be set")
}

func TestHistogramMetrics(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"operation": "file_read"}

	// Add some observations
	values := []float64{100.0, 200.0, 300.0}
	for _, v := range values {
		metrics.Observe("test_histogram", v, labels)
	}

	summary := metrics.GetHistogramSummary("test_histogram", labels)
	require.NotNil(t, summary)

	assert.Equal(t, int64(3), summary.Count)
	assert.Equal(t, 600.0, summary.Sum)
	assert.Equal(t, 200.0, summary.Avg)
}

func TestHistogramEmptyCase(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"test": "empty"}

	// Get summary for non-existent histogram
	summary := metrics.GetHistogramSummary("nonexistent", labels)
	assert.Nil(t, summary)

	// Create histogram but don't observe anything
	metrics.Observe("empty_histogram", 0, labels)
	metrics.Observe("empty_histogram", 0, labels)

	// Reset to empty state
	metrics.histograms = make(map[string]*Histogram)
	summary = metrics.GetHistogramSummary("empty_histogram", labels)
	assert.Nil(t, summary)
}

func TestTimer(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"operation": "test"}

	timer := metrics.Timer("test_operation", labels)
	require.NotNil(t, timer)

	// Sleep briefly to ensure measurable duration
	time.Sleep(10 * time.Millisecond)

	duration := timer.Stop()
	assert.Greater(t, duration, time.Duration(0))

	// Check that histogram was updated
	summary := metrics.GetHistogramSummary("test_operation_duration_ms", labels)
	require.NotNil(t, summary)
	assert.Equal(t, int64(1), summary.Count)
	assert.Greater(t, summary.Sum, 0.0)
}

func TestTimerWithContext(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "debug"})
	metrics := NewMetrics(logger)
	labels := map[string]string{"operation": "test_with_context"}

	timer := metrics.Timer("test_operation_ctx", labels)
	ctx := WithCorrelationID(context.Background(), "test-timer-correlation")

	time.Sleep(5 * time.Millisecond)

	duration := timer.StopWithContext(ctx, "test operation")
	assert.Greater(t, duration, time.Duration(0))

	// Verify histogram was updated
	summary := metrics.GetHistogramSummary("test_operation_ctx_duration_ms", labels)
	require.NotNil(t, summary)
	assert.Equal(t, int64(1), summary.Count)
}

func TestRecordAgentOperation(t *testing.T) {
	logger := NewStructuredLogger(config.LoggingConfig{Level: "info"})
	metrics := NewMetrics(logger)
	ctx := WithCorrelationID(context.Background(), "test-agent-op")

	// Test successful operation
	metrics.RecordAgentOperation(ctx, "developer", "process_issue", 5*time.Second, true)

	counters := metrics.GetCounters()

	// Verify counters were incremented
	totalFound := false
	successFound := false

	for key, value := range counters {
		if key == "agent_operations_total:agent_type=developer:operation=process_issue" {
			totalFound = true
			assert.Equal(t, int64(1), value)
		}
		if key == "agent_operations_success_total:agent_type=developer:operation=process_issue" {
			successFound = true
			assert.Equal(t, int64(1), value)
		}
	}

	assert.True(t, totalFound, "Total operations counter should be incremented")
	assert.True(t, successFound, "Success counter should be incremented")

	// Test failed operation
	metrics.RecordAgentOperation(ctx, "developer", "process_issue", 2*time.Second, false)

	counters = metrics.GetCounters()

	failureFound := false
	for key, value := range counters {
		if key == "agent_operations_failure_total:agent_type=developer:operation=process_issue" {
			failureFound = true
			assert.Equal(t, int64(1), value)
		}
		if key == "agent_operations_total:agent_type=developer:operation=process_issue" {
			assert.Equal(t, int64(2), value) // Should be 2 now
		}
	}

	assert.True(t, failureFound, "Failure counter should be incremented")
}

func TestRecordLLMCall(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	metrics.RecordLLMCall(ctx, "claude-3-sonnet", 100, 50, 2*time.Second, true)

	counters := metrics.GetCounters()

	// Verify LLM call metrics
	expectedKeys := []string{
		"llm_calls_total:model=claude-3-sonnet",
		"llm_input_tokens_total:model=claude-3-sonnet",
		"llm_output_tokens_total:model=claude-3-sonnet",
		"llm_calls_success_total:model=claude-3-sonnet",
	}

	for _, key := range expectedKeys {
		_, found := counters[key]
		assert.True(t, found, "Expected counter key %s not found", key)
	}

	assert.Equal(t, int64(100), counters["llm_input_tokens_total:model=claude-3-sonnet"])
	assert.Equal(t, int64(50), counters["llm_output_tokens_total:model=claude-3-sonnet"])
}

func TestRecordWorkflowTransition(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	metrics.RecordWorkflowTransition(ctx, "idle", "processing")

	counters := metrics.GetCounters()

	key := "workflow_transitions_total:from_state=idle:to_state=processing"
	assert.Equal(t, int64(1), counters[key])
}

func TestMetricKeysWithLabels(t *testing.T) {
	metrics := NewMetrics(nil)

	labels1 := map[string]string{"a": "1", "b": "2"}
	labels2 := map[string]string{"b": "2", "a": "1"} // Different order

	key1 := metrics.metricKey("test", labels1)
	key2 := metrics.metricKey("test", labels2)

	// Keys should be deterministic regardless of map iteration order
	assert.Equal(t, key1, key2)
	assert.Contains(t, key1, "test")
	assert.Contains(t, key1, "a=1")
	assert.Contains(t, key1, "b=2")
}

func TestRecordOrphanedWorkDetected(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	metrics.RecordOrphanedWorkDetected(ctx, "developer", "reassign", 24.5)

	counters := metrics.GetCounters()
	key := "orphaned_work_detected_total:agent_type=developer:recovery_type=reassign"
	assert.Equal(t, int64(1), counters[key])

	summary := metrics.GetHistogramSummary("orphaned_work_age_hours", map[string]string{
		"agent_type":    "developer",
		"recovery_type": "reassign",
	})
	require.NotNil(t, summary)
	assert.Equal(t, int64(1), summary.Count)
	assert.Equal(t, 24.5, summary.Sum)
}

func TestRecordStateDriftResolved(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	t.Run("successful resolution", func(t *testing.T) {
		metrics.RecordStateDriftResolved(ctx, "developer", "label_mismatch", true)

		counters := metrics.GetCounters()
		totalKey := "state_drift_resolution_total:agent_type=developer:drift_type=label_mismatch"
		resolvedKey := "state_drift_resolved_total:agent_type=developer:drift_type=label_mismatch"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[resolvedKey])
	})

	t.Run("failed resolution", func(t *testing.T) {
		metrics.RecordStateDriftResolved(ctx, "qa", "state_mismatch", false)

		counters := metrics.GetCounters()
		totalKey := "state_drift_resolution_total:agent_type=qa:drift_type=state_mismatch"
		failedKey := "state_drift_resolution_failed_total:agent_type=qa:drift_type=state_mismatch"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[failedKey])
	})
}

func TestRecordRecoveryAction(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	t.Run("successful recovery", func(t *testing.T) {
		metrics.RecordRecoveryAction(ctx, "developer", "workspace_cleanup", 2*time.Second, true)

		counters := metrics.GetCounters()
		totalKey := "recovery_actions_total:action_type=workspace_cleanup:agent_type=developer"
		successKey := "recovery_actions_success_total:action_type=workspace_cleanup:agent_type=developer"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[successKey])
	})

	t.Run("failed recovery", func(t *testing.T) {
		metrics.RecordRecoveryAction(ctx, "developer", "state_reset", 1*time.Second, false)

		counters := metrics.GetCounters()
		totalKey := "recovery_actions_total:action_type=state_reset:agent_type=developer"
		failedKey := "recovery_actions_failure_total:action_type=state_reset:agent_type=developer"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[failedKey])
	})
}

func TestRecordValidationResults(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	t.Run("valid results", func(t *testing.T) {
		metrics.RecordValidationResults(ctx, "developer", true, 0, 0, 500*time.Millisecond)

		counters := metrics.GetCounters()
		totalKey := "validation_runs_total:agent_type=developer"
		passedKey := "validation_passed_total:agent_type=developer"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[passedKey])
	})

	t.Run("invalid results", func(t *testing.T) {
		metrics.RecordValidationResults(ctx, "qa", false, 3, 2, 1*time.Second)

		counters := metrics.GetCounters()
		totalKey := "validation_runs_total:agent_type=qa"
		failedKey := "validation_failed_total:agent_type=qa"

		assert.Equal(t, int64(1), counters[totalKey])
		assert.Equal(t, int64(1), counters[failedKey])

		// Check histogram for issues and drifts
		issuesSummary := metrics.GetHistogramSummary("validation_issues_found", map[string]string{"agent_type": "qa"})
		require.NotNil(t, issuesSummary)
		assert.Equal(t, 3.0, issuesSummary.Sum)

		driftsSummary := metrics.GetHistogramSummary("validation_drifts_found", map[string]string{"agent_type": "qa"})
		require.NotNil(t, driftsSummary)
		assert.Equal(t, 2.0, driftsSummary.Sum)
	})
}

func TestRecordLLMCallFailure(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	metrics.RecordLLMCall(ctx, "claude-3-opus", 200, 0, 5*time.Second, false)

	counters := metrics.GetCounters()
	failKey := "llm_calls_failure_total:model=claude-3-opus"
	assert.Equal(t, int64(1), counters[failKey])
}

func TestRecordAgentOperationWithNilLogger(t *testing.T) {
	metrics := NewMetrics(nil)
	ctx := context.Background()

	// Should not panic even without logger
	assert.NotPanics(t, func() {
		metrics.RecordAgentOperation(ctx, "developer", "test", time.Second, true)
	})
}

func TestTimerStopWithContextNilLogger(t *testing.T) {
	metrics := NewMetrics(nil)
	labels := map[string]string{"op": "test"}

	timer := metrics.Timer("test_timer", labels)
	ctx := context.Background()

	// Should not panic with nil logger
	assert.NotPanics(t, func() {
		timer.StopWithContext(ctx, "test")
	})
}
