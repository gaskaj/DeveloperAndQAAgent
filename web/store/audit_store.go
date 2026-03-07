package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgAuditStore implements audit log persistence with PostgreSQL.
type PgAuditStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgAuditStore) Log(ctx context.Context, entry *AuditLog) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO audit_logs (org_id, user_id, action, resource_type, resource_id, details, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		entry.OrgID, entry.UserID, entry.Action, entry.ResourceType, entry.ResourceID,
		entry.Details, entry.IPAddress, entry.UserAgent,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting audit log: %w", err)
	}
	return nil
}

func (s *PgAuditStore) Query(ctx context.Context, filter AuditFilter) ([]AuditLog, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("a.org_id = $%d", argIdx))
	args = append(args, filter.OrgID)
	argIdx++

	if filter.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("a.user_id = $%d", argIdx))
		args = append(args, *filter.UserID)
		argIdx++
	}
	if filter.Action != "" {
		conditions = append(conditions, fmt.Sprintf("a.action = $%d", argIdx))
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("a.resource_type = $%d", argIdx))
		args = append(args, filter.ResourceType)
		argIdx++
	}
	if filter.Since != nil {
		conditions = append(conditions, fmt.Sprintf("a.created_at >= $%d", argIdx))
		args = append(args, *filter.Since)
		argIdx++
	}
	if filter.Until != nil {
		conditions = append(conditions, fmt.Sprintf("a.created_at <= $%d", argIdx))
		args = append(args, *filter.Until)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	var total int
	err := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM audit_logs a WHERE %s", where), args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting audit logs: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := fmt.Sprintf(
		`SELECT a.id, a.org_id, a.user_id, a.action, a.resource_type, a.resource_id,
		        a.details, a.ip_address, a.user_agent, a.created_at,
		        COALESCE(u.email, '') as user_email
		 FROM audit_logs a
		 LEFT JOIN users u ON a.user_id = u.id
		 WHERE %s
		 ORDER BY a.created_at DESC
		 LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.OrgID, &l.UserID, &l.Action, &l.ResourceType, &l.ResourceID,
			&l.Details, &l.IPAddress, &l.UserAgent, &l.CreatedAt, &l.UserEmail); err != nil {
			return nil, 0, fmt.Errorf("scanning audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}
