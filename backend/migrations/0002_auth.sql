-- V2: GitHub OAuth login and encrypted token storage.

-- Tokens are AES-256-GCM encrypted (SESSION_SECRET key); refresh_token is NULL for non-expiring grants.
ALTER TABLE users
    ADD COLUMN github_login      TEXT,
    ADD COLUMN avatar_url        TEXT,
    ADD COLUMN access_token_enc  BYTEA,
    ADD COLUMN refresh_token_enc BYTEA,
    ADD COLUMN token_expires_at  TIMESTAMPTZ;

-- Sessions: the cookie holds an opaque token and only its SHA-256 hash is stored.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
