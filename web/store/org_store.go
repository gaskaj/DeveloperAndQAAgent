package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgOrgStore implements organization persistence with PostgreSQL.
type PgOrgStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgOrgStore) Create(ctx context.Context, org *Organization) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug, plan, settings) VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at, updated_at`,
		org.Name, org.Slug, org.Plan, org.Settings,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting organization: %w", err)
	}
	return nil
}

func (s *PgOrgStore) GetByID(ctx context.Context, id uuid.UUID) (*Organization, error) {
	org := &Organization{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, settings, created_at, updated_at FROM organizations WHERE id = $1`, id,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.Settings, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting organization by ID: %w", err)
	}
	return org, nil
}

func (s *PgOrgStore) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	org := &Organization{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, settings, created_at, updated_at FROM organizations WHERE slug = $1`, slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.Settings, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting organization by slug: %w", err)
	}
	return org, nil
}

func (s *PgOrgStore) Update(ctx context.Context, org *Organization) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE organizations SET name = $1, slug = $2, plan = $3, settings = $4 WHERE id = $5`,
		org.Name, org.Slug, org.Plan, org.Settings, org.ID,
	)
	if err != nil {
		return fmt.Errorf("updating organization: %w", err)
	}
	return nil
}

func (s *PgOrgStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting organization: %w", err)
	}
	return nil
}

func (s *PgOrgStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT o.id, o.name, o.slug, o.plan, o.settings, o.created_at, o.updated_at
		 FROM organizations o
		 JOIN org_members m ON o.id = m.org_id
		 WHERE m.user_id = $1
		 ORDER BY o.name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing organizations for user: %w", err)
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.Settings, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning organization: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

func (s *PgOrgStore) AddMember(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role = $3`,
		orgID, userID, role,
	)
	if err != nil {
		return fmt.Errorf("adding org member: %w", err)
	}
	return nil
}

func (s *PgOrgStore) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM org_members WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	if err != nil {
		return fmt.Errorf("removing org member: %w", err)
	}
	return nil
}

func (s *PgOrgStore) UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE org_members SET role = $1 WHERE org_id = $2 AND user_id = $3`,
		role, orgID, userID,
	)
	if err != nil {
		return fmt.Errorf("updating member role: %w", err)
	}
	return nil
}

func (s *PgOrgStore) ListMembers(ctx context.Context, orgID uuid.UUID) ([]OrgMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.joined_at, u.email, u.display_name, u.avatar_url
		 FROM org_members m
		 JOIN users u ON m.user_id = u.id
		 WHERE m.org_id = $1
		 ORDER BY m.joined_at`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing org members: %w", err)
	}
	defer rows.Close()

	var members []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.JoinedAt, &m.Email, &m.DisplayName, &m.AvatarURL); err != nil {
			return nil, fmt.Errorf("scanning org member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

func (s *PgOrgStore) GetMembership(ctx context.Context, orgID, userID uuid.UUID) (*OrgMember, error) {
	m := &OrgMember{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, user_id, role, joined_at FROM org_members WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting membership: %w", err)
	}
	return m, nil
}
