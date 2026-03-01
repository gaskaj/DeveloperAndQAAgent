package observability

import (
	"context"
	"testing"
	"time"

	"github.com/gaskaj/DeveloperAndQAAgent/internal/config"
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
	
	// Keys should contain the expected components
	// (Note: order may vary based on map iteration)
	assert.Contains(t, key1, "test")
	assert.Contains(t, key1, "a=1")
	assert.Contains(t, key1, "b=2")
	
	// Both keys should contain the same components
	assert.Contains(t, key2, "test")
	assert.Contains(t, key2, "a=1")
	assert.Contains(t, key2, "b=2")
}