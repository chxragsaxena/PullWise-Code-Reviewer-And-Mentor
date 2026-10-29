-- NULL owner = anonymous review of a public PR; CASCADE so deleting a user never makes reviews public.
ALTER TABLE reviews ADD COLUMN owner_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX reviews_owner_idx ON reviews (owner_id, created_at DESC);
