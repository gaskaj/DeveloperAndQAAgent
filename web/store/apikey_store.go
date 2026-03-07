package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgAPIKeyStore implements API key persistence with PostgreSQL.
type PgAPIKeyStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgAPIKeyStore) Create(ctx context.Context, key *APIKey) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (org_id, created_by, name, key_hash, key_prefix, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		key.OrgID, key.CreatedBy, key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating API key: %w", err)
	}
	return nil
}

func (s *PgAPIKeyStore) GetByPrefix(ctx context.Context, prefix string) (*APIKey, error) {
	key := &APIKey{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, created_by, name, key_hash, key_prefix, scopes, last_used, expires_at, revoked, created_at
		 FROM api_keys
		 WHERE key_prefix = $1 AND revoked = FALSE AND (expires_at IS NULL OR expires_at > NOW())`, prefix,
	).Scan(&key.ID, &key.OrgID, &key.CreatedBy, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.LastUsed, &key.ExpiresAt, &key.Revoked, &key.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting API key by prefix: %w", err)
	}
	return key, nil
}

func (s *PgAPIKeyStore) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, created_by, name, key_prefix, scopes, last_used, expires_at, revoked, created_at
		 FROM api_keys
		 WHERE org_id = $1 AND revoked = FALSE
		 ORDER BY created_at DESC`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing API keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.CreatedBy, &k.Name, &k.KeyPrefix,
			&k.Scopes, &k.LastUsed, &k.ExpiresAt, &k.Revoked, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning API key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *PgAPIKeyStore) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_keys SET revoked = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoking API key: %w", err)
	}
	return nil
}

func (s *PgAPIKeyStore) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_keys SET last_used = $1 WHERE id = $2`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("updating API key last used: %w", err)
	}
	return nil
}
