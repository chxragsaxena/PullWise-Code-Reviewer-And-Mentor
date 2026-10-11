package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
)

// ErrSessionNotFound means no active, unexpired session matches the token.
var ErrSessionNotFound = errors.New("session not found")

// UpsertGitHubUser creates or updates the user row for a GitHub account,
// keyed by their stable github_id. Profile fields refresh on every login,
// since a user's name or avatar can change on GitHub between logins.
func (s *Store) UpsertGitHubUser(ctx context.Context, p auth.Profile) (auth.User, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (github_id, name, email, github_login, avatar_url)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (github_id) DO UPDATE
		SET name = EXCLUDED.name, email = EXCLUDED.email,
		    github_login = EXCLUDED.github_login, avatar_url = EXCLUDED.avatar_url
		RETURNING id`,
		p.ID, p.Name, nullable(p.Email), p.Login, p.AvatarURL).Scan(&id)
	if err != nil {
		return auth.User{}, fmt.Errorf("upsert github user: %w", err)
	}
	return auth.User{ID: strconv.FormatInt(id, 10), GitHubLogin: p.Login, Name: p.Name, AvatarURL: p.AvatarURL}, nil
}

func nullable(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// SaveTokens stores a user's encrypted GitHub OAuth tokens. refreshEnc and
// expiresAt are nil for a token that does not expire.
func (s *Store) SaveTokens(ctx context.Context, userID string, accessEnc, refreshEnc []byte, expiresAt *time.Time) error {
	id, ok := parseID(userID)
	if !ok {
		return fmt.Errorf("save tokens: invalid user id %q", userID)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET access_token_enc = $2, refresh_token_enc = $3, token_expires_at = $4
		WHERE id = $1`, id, accessEnc, refreshEnc, expiresAt)
	if err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}
	return nil
}

// GetTokens returns a user's stored encrypted tokens.
func (s *Store) GetTokens(ctx context.Context, userID string) (auth.Tokens, error) {
	id, ok := parseID(userID)
	if !ok {
		return auth.Tokens{}, fmt.Errorf("get tokens: invalid user id %q", userID)
	}
	var t auth.Tokens
	err := s.db.QueryRowContext(ctx,
		"SELECT access_token_enc, refresh_token_enc, token_expires_at FROM users WHERE id = $1", id).
		Scan(&t.AccessEnc, &t.RefreshEnc, &t.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Tokens{}, fmt.Errorf("get tokens: %w", ErrSessionNotFound) // no such user
	}
	if err != nil {
		return auth.Tokens{}, fmt.Errorf("get tokens: %w", err)
	}
	return t, nil
}

// CreateSession records a new session, keyed by its token's hash.
func (s *Store) CreateSession(ctx context.Context, hash auth.SessionTokenHash, userID string, expiresAt time.Time) error {
	id, ok := parseID(userID)
	if !ok {
		return fmt.Errorf("create session: invalid user id %q", userID)
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)", hash[:], id, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSessionUser returns the user for a session token's hash, if it exists
// and has not expired.
func (s *Store) GetSessionUser(ctx context.Context, hash auth.SessionTokenHash) (auth.User, error) {
	var u auth.User
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.github_login, u.name, u.avatar_url
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hash[:]).
		Scan(&id, &u.GitHubLogin, &u.Name, &u.AvatarURL)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, ErrSessionNotFound
	}
	if err != nil {
		return auth.User{}, fmt.Errorf("get session user: %w", err)
	}
	u.ID = strconv.FormatInt(id, 10)
	return u, nil
}

// DeleteSession removes one session (logout). Deleting an unknown or
// already-expired session is not an error.
func (s *Store) DeleteSession(ctx context.Context, hash auth.SessionTokenHash) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = $1", hash[:]); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes every session past its expiry. Call this
// periodically (e.g. at startup, alongside FailInterrupted) to keep the
// table from growing without bound.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= now()")
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted sessions: %w", err)
	}
	return int(n), nil
}
