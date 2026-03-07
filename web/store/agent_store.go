package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgAgentStore implements agent persistence with PostgreSQL.
type PgAgentStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgAgentStore) Register(ctx context.Context, agent *Agent) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agents (org_id, name, agent_type, description, github_owner, github_repo, config_snapshot, status, version, hostname, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (org_id, name) DO UPDATE SET
		   agent_type = EXCLUDED.agent_type,
		   github_owner = EXCLUDED.github_owner,
		   github_repo = EXCLUDED.github_repo,
		   config_snapshot = EXCLUDED.config_snapshot,
		   version = EXCLUDED.version,
		   hostname = EXCLUDED.hostname,
		   tags = EXCLUDED.tags,
		   status = 'online',
		   last_heartbeat = NOW()
		 RETURNING id, created_at, updated_at`,
		agent.OrgID, agent.Name, agent.AgentType, agent.Description,
		agent.GitHubOwner, agent.GitHubRepo, agent.ConfigSnapshot,
		agent.Status, agent.Version, agent.Hostname, agent.Tags,
	).Scan(&agent.ID, &agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		return fmt.Errorf("registering agent: %w", err)
	}
	return nil
}

func (s *PgAgentStore) GetByID(ctx context.Context, id uuid.UUID) (*Agent, error) {
	agent := &Agent{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, agent_type, description, github_owner, github_repo, config_snapshot,
		        last_heartbeat, status, version, hostname, tags, created_at, updated_at
		 FROM agents WHERE id = $1`, id,
	).Scan(&agent.ID, &agent.OrgID, &agent.Name, &agent.AgentType, &agent.Description,
		&agent.GitHubOwner, &agent.GitHubRepo, &agent.ConfigSnapshot,
		&agent.LastHeartbeat, &agent.Status, &agent.Version, &agent.Hostname, &agent.Tags,
		&agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting agent by ID: %w", err)
	}
	return agent, nil
}

func (s *PgAgentStore) GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*Agent, error) {
	agent := &Agent{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, agent_type, description, github_owner, github_repo, config_snapshot,
		        last_heartbeat, status, version, hostname, tags, created_at, updated_at
		 FROM agents WHERE org_id = $1 AND name = $2`, orgID, name,
	).Scan(&agent.ID, &agent.OrgID, &agent.Name, &agent.AgentType, &agent.Description,
		&agent.GitHubOwner, &agent.GitHubRepo, &agent.ConfigSnapshot,
		&agent.LastHeartbeat, &agent.Status, &agent.Version, &agent.Hostname, &agent.Tags,
		&agent.CreatedAt, &agent.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting agent by org and name: %w", err)
	}
	return agent, nil
}

func (s *PgAgentStore) ListByOrg(ctx context.Context, orgID uuid.UUID, opts ListOpts) ([]Agent, int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE org_id = $1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting agents: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, agent_type, description, github_owner, github_repo, config_snapshot,
		        last_heartbeat, status, version, hostname, tags, created_at, updated_at
		 FROM agents WHERE org_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, orgID, opts.Limit, opts.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Name, &a.AgentType, &a.Description,
			&a.GitHubOwner, &a.GitHubRepo, &a.ConfigSnapshot,
			&a.LastHeartbeat, &a.Status, &a.Version, &a.Hostname, &a.Tags,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, total, nil
}

func (s *PgAgentStore) UpdateHeartbeat(ctx context.Context, agentID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agents SET last_heartbeat = NOW(), status = 'online' WHERE id = $1`, agentID)
	if err != nil {
		return fmt.Errorf("updating heartbeat: %w", err)
	}
	return nil
}

func (s *PgAgentStore) UpdateStatus(ctx context.Context, agentID uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agents SET status = $1 WHERE id = $2`, status, agentID)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}
	return nil
}

func (s *PgAgentStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	return nil
}
