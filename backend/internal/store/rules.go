package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

const ruleColumns = "id, title, instructions, severity, enabled"

func scanRule(row rowScanner) (review.Rule, error) {
	var r review.Rule
	var id int64
	var severity string
	if err := row.Scan(&id, &r.Title, &r.Instructions, &severity, &r.Enabled); err != nil {
		return review.Rule{}, err
	}
	r.ID, r.Severity = strconv.FormatInt(id, 10), review.Severity(severity)
	return r, nil
}

// ListRules returns the user's rules, oldest first.
func (s *Store) ListRules(ctx context.Context, userID string) ([]review.Rule, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+ruleColumns+" FROM custom_rules WHERE user_id = $1 ORDER BY id", uid)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var out []review.Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRule inserts a rule unless the user already has max of them.
func (s *Store) CreateRule(ctx context.Context, userID string, r review.Rule, max int) (review.Rule, error) {
	uid, ok := parseID(userID)
	if !ok {
		return review.Rule{}, review.ErrRuleNotFound
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO custom_rules (user_id, title, instructions, severity, enabled)
		SELECT $1, $2, $3, $4, $5 WHERE (SELECT count(*) FROM custom_rules WHERE user_id = $1) < $6
		RETURNING `+ruleColumns, uid, r.Title, r.Instructions, string(r.Severity), r.Enabled, max)
	created, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return review.Rule{}, review.ErrRuleLimit
	}
	if err != nil {
		return review.Rule{}, fmt.Errorf("create rule: %w", err)
	}
	return created, nil
}

// UpdateRule replaces one of the user's rules.
func (s *Store) UpdateRule(ctx context.Context, userID, id string, r review.Rule) (review.Rule, error) {
	uid, ok1 := parseID(userID)
	rid, ok2 := parseID(id)
	if !ok1 || !ok2 {
		return review.Rule{}, review.ErrRuleNotFound
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_rules SET title = $3, instructions = $4, severity = $5, enabled = $6, updated_at = now()
		WHERE id = $2 AND user_id = $1 RETURNING `+ruleColumns,
		uid, rid, r.Title, r.Instructions, string(r.Severity), r.Enabled)
	updated, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return review.Rule{}, review.ErrRuleNotFound
	}
	if err != nil {
		return review.Rule{}, fmt.Errorf("update rule: %w", err)
	}
	return updated, nil
}

// DeleteRule removes one of the user's rules.
func (s *Store) DeleteRule(ctx context.Context, userID, id string) error {
	uid, ok1 := parseID(userID)
	rid, ok2 := parseID(id)
	if !ok1 || !ok2 {
		return review.ErrRuleNotFound
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM custom_rules WHERE id = $1 AND user_id = $2", rid, uid)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return review.ErrRuleNotFound
	}
	return nil
}

// ReviewRules returns the rules a review was started with.
func (s *Store) ReviewRules(ctx context.Context, reviewID string) ([]review.Rule, error) {
	rid, ok := parseID(reviewID)
	if !ok {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT rule_id, title, instructions, severity, true FROM review_rules WHERE review_id = $1 ORDER BY rule_id`, rid)
	if err != nil {
		return nil, fmt.Errorf("read review rules: %w", err)
	}
	defer rows.Close()

	var out []review.Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan review rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
