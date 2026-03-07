package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgInvitationStore implements invitation persistence with PostgreSQL.
type PgInvitationStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgInvitationStore) Create(ctx context.Context, inv *Invitation) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO invitations (org_id, invited_by, email, role, token, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		inv.OrgID, inv.InvitedBy, inv.Email, inv.Role, inv.Token, inv.ExpiresAt,
	).Scan(&inv.ID, &inv.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating invitation: %w", err)
	}
	return nil
}

func (s *PgInvitationStore) GetByToken(ctx context.Context, token string) (*Invitation, error) {
	inv := &Invitation{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, invited_by, email, role, token, accepted, expires_at, created_at
		 FROM invitations WHERE token = $1`, token,
	).Scan(&inv.ID, &inv.OrgID, &inv.InvitedBy, &inv.Email, &inv.Role, &inv.Token,
		&inv.Accepted, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting invitation by token: %w", err)
	}
	return inv, nil
}

func (s *PgInvitationStore) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Invitation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, invited_by, email, role, accepted, expires_at, created_at
		 FROM invitations
		 WHERE org_id = $1 AND accepted = FALSE AND expires_at > NOW()
		 ORDER BY created_at DESC`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing invitations: %w", err)
	}
	defer rows.Close()

	var invitations []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.InvitedBy, &inv.Email, &inv.Role,
			&inv.Accepted, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning invitation: %w", err)
		}
		invitations = append(invitations, inv)
	}
	return invitations, nil
}

func (s *PgInvitationStore) MarkAccepted(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE invitations SET accepted = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("marking invitation accepted: %w", err)
	}
	return nil
}

func (s *PgInvitationStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM invitations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting invitation: %w", err)
	}
	return nil
}
