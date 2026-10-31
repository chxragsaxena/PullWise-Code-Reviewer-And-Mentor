package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// maxRememberedRepos is how many repositories are kept per user; the least recently used go first.
const maxRememberedRepos = 50

var repoPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,100}/[a-z0-9_.-]{1,100}$`)

// RememberRepo records that the user used a repository (lowercase owner/name), moving it to the front.
func (s *Store) RememberRepo(ctx context.Context, userID, repo string) error {
	uid, ok := parseID(userID)
	if !ok {
		return errors.New("remember repo: invalid user id")
	}
	if !repoPattern.MatchString(repo) {
		return fmt.Errorf("remember repo: %q is not a lowercase owner/name", repo)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remember repo: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_repos (user_id, repo) VALUES ($1, $2)
		ON CONFLICT (user_id, repo) DO UPDATE SET uses = user_repos.uses + 1, last_used_at = now()`, uid, repo); err != nil {
		return fmt.Errorf("remember repo: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM user_repos WHERE user_id = $1 AND repo NOT IN (
			SELECT repo FROM user_repos WHERE user_id = $1 ORDER BY last_used_at DESC, repo LIMIT $2)`,
		uid, maxRememberedRepos); err != nil {
		return fmt.Errorf("remember repo: trim old entries: %w", err)
	}
	return tx.Commit()
}

// RecentRepos returns the user's repositories, most recently used first.
func (s *Store) RecentRepos(ctx context.Context, userID string, limit int) ([]string, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT repo FROM user_repos WHERE user_id = $1 ORDER BY last_used_at DESC, repo LIMIT $2", uid, limit)
	if err != nil {
		return nil, fmt.Errorf("recent repos: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err != nil {
			return nil, fmt.Errorf("scan recent repo: %w", err)
		}
		out = append(out, repo)
	}
	return out, rows.Err()
}
