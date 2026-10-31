-- Repositories a signed-in user has reviewed or asked about, newest first, to suggest them next time.
CREATE TABLE user_repos (
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo         TEXT NOT NULL,          -- lowercase owner/name
    uses         INTEGER NOT NULL DEFAULT 1,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, repo)
);
CREATE INDEX user_repos_recent_idx ON user_repos (user_id, last_used_at DESC);

-- Start from the reviews each user already owns.
INSERT INTO user_repos (user_id, repo, uses, last_used_at)
SELECT rv.owner_id, lower(r.owner || '/' || r.name), count(*), max(rv.created_at)
FROM reviews rv
JOIN pull_requests pr ON pr.id = rv.pull_request_id
JOIN repositories r ON r.id = pr.repository_id
WHERE rv.owner_id IS NOT NULL
GROUP BY rv.owner_id, lower(r.owner || '/' || r.name);
