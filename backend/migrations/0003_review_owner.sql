-- Reviews made while signed in belong to that user and are visible only to them.
-- NULL owner = an anonymous review of a public PR, visible to everyone (as in V1).
-- CASCADE, not SET NULL: deleting a user must delete their private reviews, never
-- turn them public.
ALTER TABLE reviews ADD COLUMN owner_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX reviews_owner_idx ON reviews (owner_id, created_at DESC);
