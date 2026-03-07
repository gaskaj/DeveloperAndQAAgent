package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgEventStore implements agent event persistence with PostgreSQL.
type PgEventStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgEventStore) Insert(ctx context.Context, event *AgentEvent) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_events (org_id, agent_id, event_type, severity, payload, issue_number, pr_number, workflow_state, correlation_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		event.OrgID, event.AgentID, event.EventType, event.Severity, event.Payload,
		event.IssueNumber, event.PRNumber, event.WorkflowState, event.CorrelationID,
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}
	return nil
}

func (s *PgEventStore) InsertBatch(ctx context.Context, events []AgentEvent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning batch transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for i := range events {
		e := &events[i]
		_, err := tx.Exec(ctx,
			`INSERT INTO agent_events (org_id, agent_id, event_type, severity, payload, issue_number, pr_number, workflow_state, correlation_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			e.OrgID, e.AgentID, e.EventType, e.Severity, e.Payload,
			e.IssueNumber, e.PRNumber, e.WorkflowState, e.CorrelationID,
		)
		if err != nil {
			return fmt.Errorf("inserting event in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing batch: %w", err)
	}
	return nil
}

func (s *PgEventStore) Query(ctx context.Context, filter EventFilter) ([]AgentEvent, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("e.org_id = $%d", argIdx))
	args = append(args, filter.OrgID)
	argIdx++

	if filter.AgentID != nil {
		conditions = append(conditions, fmt.Sprintf("e.agent_id = $%d", argIdx))
		args = append(args, *filter.AgentID)
		argIdx++
	}
	if filter.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("e.event_type = $%d", argIdx))
		args = append(args, filter.EventType)
		argIdx++
	}
	if filter.Severity != "" {
		conditions = append(conditions, fmt.Sprintf("e.severity = $%d", argIdx))
		args = append(args, filter.Severity)
		argIdx++
	}
	if filter.Since != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at >= $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}
	if filter.Until != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at <= $%d", argIdx))
		args = append(args, *filter.Until)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM agent_events e WHERE %s", where)
	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting events: %w", err)
	}

	// Query
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := fmt.Sprintf(
		`SELECT e.id, e.org_id, e.agent_id, e.event_type, e.severity, e.payload,
		        e.issue_number, e.pr_number, e.workflow_state, e.correlation_id, e.created_at,
		        a.name as agent_name
		 FROM agent_events e
		 JOIN agents a ON e.agent_id = a.id
		 WHERE %s
		 ORDER BY e.created_at DESC
		 LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []AgentEvent
	for rows.Next() {
		var e AgentEvent
		if err := rows.Scan(&e.ID, &e.OrgID, &e.AgentID, &e.EventType, &e.Severity, &e.Payload,
			&e.IssueNumber, &e.PRNumber, &e.WorkflowState, &e.CorrelationID, &e.CreatedAt,
			&e.AgentName); err != nil {
			return nil, 0, fmt.Errorf("scanning event: %w", err)
		}
		events = append(events, e)
	}
	return events, total, nil
}

func (s *PgEventStore) GetLatestByAgent(ctx context.Context, agentID uuid.UUID, limit int) ([]AgentEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.org_id, e.agent_id, e.event_type, e.severity, e.payload,
		        e.issue_number, e.pr_number, e.workflow_state, e.correlation_id, e.created_at
		 FROM agent_events e
		 WHERE e.agent_id = $1
		 ORDER BY e.created_at DESC
		 LIMIT $2`, agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting latest events: %w", err)
	}
	defer rows.Close()

	var events []AgentEvent
	for rows.Next() {
		var e AgentEvent
		if err := rows.Scan(&e.ID, &e.OrgID, &e.AgentID, &e.EventType, &e.Severity, &e.Payload,
			&e.IssueNumber, &e.PRNumber, &e.WorkflowState, &e.CorrelationID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *PgEventStore) CountByType(ctx context.Context, orgID uuid.UUID, since time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT event_type, COUNT(*) FROM agent_events
		 WHERE org_id = $1 AND created_at >= $2
		 GROUP BY event_type`, orgID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("counting events by type: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("scanning event count: %w", err)
		}
		counts[eventType] = count
	}
	return counts, nil
}

func (s *PgEventStore) CountBySeverity(ctx context.Context, orgID uuid.UUID, since time.Time) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT severity, COUNT(*) FROM agent_events
		 WHERE org_id = $1 AND created_at >= $2
		 GROUP BY severity`, orgID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("counting events by severity: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var severity string
		var count int64
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("scanning severity count: %w", err)
		}
		counts[severity] = count
	}
	return counts, nil
}

// MarshalPayload converts a map to JSON for storage.
func MarshalPayload(payload map[string]any) json.RawMessage {
	if payload == nil {
		return json.RawMessage("{}")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
