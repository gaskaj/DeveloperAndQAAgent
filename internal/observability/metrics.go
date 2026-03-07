package observability

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Metrics provides performance monitoring and metrics collection
type Metrics struct {
	mu         sync.RWMutex
	counters   map[string]int64
	gauges     map[string]float64
	histograms map[string]*Histogram
	logger     *StructuredLogger
}

// Histogram tracks distribution of values over time
type Histogram struct {
	mu     sync.RWMutex
	values []float64
	sum    float64
	count  int64
}

// NewMetrics creates a new metrics collector
func NewMetrics(logger *StructuredLogger) *Metrics {
	return &Metrics{
		counters:   make(map[string]int64),
		gauges:     make(map[string]float64),
		histograms: make(map[string]*Histogram),
		logger:     logger,
	}
}

// Inc increments a counter metric
func (m *Metrics) Inc(name string, labels map[string]string) {
	key := m.metricKey(name, labels)
	m.mu.Lock()
	m.counters[key]++
	m.mu.Unlock()
}

// Add adds a value to a counter metric
func (m *Metrics) Add(name string, value int64, labels map[string]string) {
	key := m.metricKey(name, labels)
	m.mu.Lock()
	m.counters[key] += value
	m.mu.Unlock()
}

// Set sets a gauge metric to a specific value
func (m *Metrics) Set(name string, value float64, labels map[string]string) {
	key := m.metricKey(name, labels)
	m.mu.Lock()
	m.gauges[key] = value
	m.mu.Unlock()
}

// Observe records an observation in a histogram
func (m *Metrics) Observe(name string, value float64, labels map[string]string) {
	key := m.metricKey(name, labels)
	m.mu.Lock()
	hist, exists := m.histograms[key]
	if !exists {
		hist = &Histogram{values: make([]float64, 0)}
		m.histograms[key] = hist
	}
	m.mu.Unlock()

	hist.mu.Lock()
	hist.values = append(hist.values, value)
	hist.sum += value
	hist.count++
	hist.mu.Unlock()
}

// Timer creates a timer that will record duration when stopped
func (m *Metrics) Timer(name string, labels map[string]string) *Timer {
	return &Timer{
		name:    name,
		labels:  labels,
		start:   time.Now(),
		metrics: m,
	}
}

// Timer tracks the duration of an operation
type Timer struct {
	name    string
	labels  map[string]string
	start   time.Time
	metrics *Metrics
}

// Stop stops the timer and records the duration
func (t *Timer) Stop() time.Duration {
	duration := time.Since(t.start)
	t.metrics.Observe(t.name+"_duration_ms", float64(duration.Milliseconds()), t.labels)
	return duration
}

// StopWithContext stops the timer and logs to structured logger
func (t *Timer) StopWithContext(ctx context.Context, operation string) time.Duration {
	duration := t.Stop()
	if t.metrics.logger != nil {
		t.metrics.logger.LogPerformanceMetric(ctx, t.name+"_duration", float64(duration.Milliseconds()), "ms", t.labels)
	}
	return duration
}

// GetCounters returns a snapshot of all counter metrics
func (m *Metrics) GetCounters() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]int64)
	for k, v := range m.counters {
		result[k] = v
	}
	return result
}

// GetGauges returns a snapshot of all gauge metrics
func (m *Metrics) GetGauges() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for k, v := range m.gauges {
		result[k] = v
	}
	return result
}

// GetHistogramSummary returns summary statistics for a histogram
func (m *Metrics) GetHistogramSummary(name string, labels map[string]string) *HistogramSummary {
	key := m.metricKey(name, labels)
	m.mu.RLock()
	hist, exists := m.histograms[key]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	hist.mu.RLock()
	defer hist.mu.RUnlock()

	if hist.count == 0 {
		return &HistogramSummary{}
	}

	return &HistogramSummary{
		Count: hist.count,
		Sum:   hist.sum,
		Avg:   hist.sum / float64(hist.count),
	}
}

// HistogramSummary provides summary statistics for histogram metrics
type HistogramSummary struct {
	Count int64
	Sum   float64
	Avg   float64
}

// RecordAgentOperation records metrics for an agent operation
func (m *Metrics) RecordAgentOperation(ctx context.Context, agentType, operation string, duration time.Duration, success bool) {
	labels := map[string]string{
		"agent_type": agentType,
		"operation":  operation,
	}

	m.Inc("agent_operations_total", labels)
	m.Observe("agent_operation_duration_ms", float64(duration.Milliseconds()), labels)

	if success {
		m.Inc("agent_operations_success_total", labels)
	} else {
		m.Inc("agent_operations_failure_total", labels)
	}

	// Log to structured logger
	if m.logger != nil {
		successStr := "success"
		if !success {
			successStr = "failure"
		}
		m.logger.LogPerformanceMetric(ctx, "agent_operation", float64(duration.Milliseconds()), "ms",
			map[string]string{
				"agent_type": agentType,
				"operation":  operation,
				"status":     successStr,
			})
	}
}

// RecordLLMCall records metrics for LLM API calls
func (m *Metrics) RecordLLMCall(ctx context.Context, model string, inputTokens, outputTokens int, duration time.Duration, success bool) {
	labels := map[string]string{
		"model": model,
	}

	m.Inc("llm_calls_total", labels)
	m.Add("llm_input_tokens_total", int64(inputTokens), labels)
	m.Add("llm_output_tokens_total", int64(outputTokens), labels)
	m.Observe("llm_call_duration_ms", float64(duration.Milliseconds()), labels)

	if success {
		m.Inc("llm_calls_success_total", labels)
	} else {
		m.Inc("llm_calls_failure_total", labels)
	}
}

// RecordWorkflowTransition records metrics for workflow state transitions
func (m *Metrics) RecordWorkflowTransition(ctx context.Context, fromState, toState string) {
	labels := map[string]string{
		"from_state": fromState,
		"to_state":   toState,
	}

	m.Inc("workflow_transitions_total", labels)
}

// RecordOrphanedWorkDetected records metrics for orphaned work detection
func (m *Metrics) RecordOrphanedWorkDetected(ctx context.Context, agentType string, recoveryType string, ageHours float64) {
	labels := map[string]string{
		"agent_type":    agentType,
		"recovery_type": recoveryType,
	}

	m.Inc("orphaned_work_detected_total", labels)
	m.Observe("orphaned_work_age_hours", ageHours, labels)
}

// RecordStateDriftResolved records metrics for state drift resolution
func (m *Metrics) RecordStateDriftResolved(ctx context.Context, agentType string, driftType string, success bool) {
	labels := map[string]string{
		"agent_type": agentType,
		"drift_type": driftType,
	}

	m.Inc("state_drift_resolution_total", labels)
	if success {
		m.Inc("state_drift_resolved_total", labels)
	} else {
		m.Inc("state_drift_resolution_failed_total", labels)
	}
}

// RecordRecoveryAction records metrics for recovery actions
func (m *Metrics) RecordRecoveryAction(ctx context.Context, agentType string, actionType string, duration time.Duration, success bool) {
	labels := map[string]string{
		"agent_type":  agentType,
		"action_type": actionType,
	}

	m.Inc("recovery_actions_total", labels)
	m.Observe("recovery_action_duration_ms", float64(duration.Milliseconds()), labels)

	if success {
		m.Inc("recovery_actions_success_total", labels)
	} else {
		m.Inc("recovery_actions_failure_total", labels)
	}
}

// RecordValidationResults records metrics for validation results
func (m *Metrics) RecordValidationResults(ctx context.Context, agentType string, valid bool, issuesCount int, driftCount int, duration time.Duration) {
	labels := map[string]string{
		"agent_type": agentType,
	}

	m.Inc("validation_runs_total", labels)
	m.Observe("validation_duration_ms", float64(duration.Milliseconds()), labels)
	m.Observe("validation_issues_found", float64(issuesCount), labels)
	m.Observe("validation_drifts_found", float64(driftCount), labels)

	if valid {
		m.Inc("validation_passed_total", labels)
	} else {
		m.Inc("validation_failed_total", labels)
	}
}

// RecordDatabaseOperation records metrics for database operations
func (m *Metrics) RecordDatabaseOperation(ctx context.Context, operation string, duration time.Duration, success bool) {
	labels := map[string]string{
		"operation": operation,
	}

	m.Inc("database_operations_total", labels)
	m.Observe("database_operation_duration_ms", float64(duration.Milliseconds()), labels)

	if success {
		m.Inc("database_operations_success_total", labels)
	} else {
		m.Inc("database_operations_failure_total", labels)
	}

	// Log to structured logger
	if m.logger != nil {
		successStr := "success"
		if !success {
			successStr = "failure"
		}
		m.logger.LogPerformanceMetric(ctx, "database_operation", float64(duration.Milliseconds()), "ms",
			map[string]string{
				"operation": operation,
				"status":    successStr,
			})
	}
}

// RecordDatabaseConnectionPool records connection pool metrics
func (m *Metrics) RecordDatabaseConnectionPool(ctx context.Context, totalConns, idleConns, acquiredConns, maxConns int) {
	poolLabels := map[string]string{
		"pool_type": "postgresql",
	}

	m.Set("database_pool_total_connections", float64(totalConns), poolLabels)
	m.Set("database_pool_idle_connections", float64(idleConns), poolLabels)
	m.Set("database_pool_acquired_connections", float64(acquiredConns), poolLabels)
	m.Set("database_pool_max_connections", float64(maxConns), poolLabels)

	// Calculate utilization percentage
	utilization := float64(acquiredConns) / float64(maxConns) * 100
	m.Set("database_pool_utilization_percent", utilization, poolLabels)
}

// RecordSlowQuery records metrics for slow database queries
func (m *Metrics) RecordSlowQuery(ctx context.Context, operation string, duration time.Duration) {
	labels := map[string]string{
		"operation": operation,
	}

	m.Inc("database_slow_queries_total", labels)
	m.Observe("database_slow_query_duration_ms", float64(duration.Milliseconds()), labels)

	// Log to structured logger
	if m.logger != nil {
		m.logger.LogPerformanceMetric(ctx, "slow_database_query", float64(duration.Milliseconds()), "ms",
			map[string]string{
				"operation": operation,
			})
	}
}

// metricKey creates a unique key for a metric with labels
func (m *Metrics) metricKey(name string, labels map[string]string) string {
	key := name
	// Sort label keys for deterministic key ordering
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		key += ":" + k + "=" + labels[k]
	}
	return key
}
