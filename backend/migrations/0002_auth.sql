-- V2: GitHub OAuth login and encrypted token storage.

-- github_id was already unique/required; login is the @handle, shown in the UI.
-- Tokens are encrypted (AES-256-GCM, key from SESSION_SECRET) before storage,
-- so a database leak alone does not expose usable GitHub credentials.
-- refresh_token is nullable: a classic (non-expiring) OAuth App grant has none.
ALTER TABLE users
    ADD COLUMN github_login      TEXT,
    ADD COLUMN avatar_url        TEXT,
    ADD COLUMN access_token_enc  BYTEA,
    ADD COLUMN refresh_token_enc BYTEA,
    ADD COLUMN token_expires_at  TIMESTAMPTZ;

-- Server-side sessions: the cookie holds a random opaque token; only its
-- SHA-256 hash is stored, so a database leak does not yield a usable session.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
