// internal/spotify/tokenstore.go
package spotify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLTokenStore persists exactly one row — the Spotify refresh token
// from the host's one-time authorization.
type SQLTokenStore struct {
	db *sql.DB
}

func NewSQLTokenStore(db *sql.DB) *SQLTokenStore {
	return &SQLTokenStore{db: db}
}

func (s *SQLTokenStore) LoadRefreshToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx,
		`SELECT refresh_token FROM oauth_tokens WHERE provider = 'spotify'`,
	).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("spotify: loading refresh token: %w", err)
	}
	return token, nil
}

func (s *SQLTokenStore) SaveTokens(ctx context.Context, refreshToken string, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oauth_tokens (provider, refresh_token, scopes, updated_at)
		VALUES ('spotify', ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET refresh_token = excluded.refresh_token, updated_at = excluded.updated_at`,
		refreshToken, Scopes, updatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("spotify: saving refresh token: %w", err)
	}
	return nil
}
