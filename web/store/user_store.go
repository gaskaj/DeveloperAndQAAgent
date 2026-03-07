package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgUserStore implements user persistence with PostgreSQL.
type PgUserStore struct {
	pool    *pgxpool.Pool
	monitor *QueryMonitor
}

func (s *PgUserStore) Create(ctx context.Context, user *User) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name, password_hash, avatar_url, email_verified)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		user.Email, user.DisplayName, user.PasswordHash, user.AvatarURL, user.EmailVerified,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}
	return nil
}

func (s *PgUserStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, password_hash, avatar_url, email_verified, is_active, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.AvatarURL,
		&user.EmailVerified, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting user by ID: %w", err)
	}
	return user, nil
}

func (s *PgUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, password_hash, avatar_url, email_verified, is_active, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.AvatarURL,
		&user.EmailVerified, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting user by email: %w", err)
	}
	return user, nil
}

func (s *PgUserStore) Update(ctx context.Context, user *User) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET email = $1, display_name = $2, avatar_url = $3, email_verified = $4 WHERE id = $5`,
		user.Email, user.DisplayName, user.AvatarURL, user.EmailVerified, user.ID,
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	return nil
}

func (s *PgUserStore) CreateOAuthLink(ctx context.Context, link *UserOAuthLink) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO user_oauth_links (user_id, provider, provider_uid, provider_email, access_token, refresh_token, token_expires)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		link.UserID, link.Provider, link.ProviderUID, link.ProviderEmail, link.AccessToken, link.RefreshToken, link.TokenExpires,
	).Scan(&link.ID, &link.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating OAuth link: %w", err)
	}
	return nil
}

func (s *PgUserStore) GetOAuthLink(ctx context.Context, provider, providerUID string) (*UserOAuthLink, error) {
	link := &UserOAuthLink{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, provider, provider_uid, provider_email, access_token, refresh_token, token_expires, created_at
		 FROM user_oauth_links WHERE provider = $1 AND provider_uid = $2`, provider, providerUID,
	).Scan(&link.ID, &link.UserID, &link.Provider, &link.ProviderUID, &link.ProviderEmail,
		&link.AccessToken, &link.RefreshToken, &link.TokenExpires, &link.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting OAuth link: %w", err)
	}
	return link, nil
}
