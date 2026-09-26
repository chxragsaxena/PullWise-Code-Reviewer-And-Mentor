package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// reviewColumns joins a review with its pull request and repository. The head
// commit comes from the review itself, because a pull request row is shared by
// every review of that PR and is overwritten as the PR moves on.
const reviewSelect = `
	SELECT r.id, r.status, r.status_message, r.status_at, r.error_code, r.error_message,
	       r.created_at, r.completed_at, r.head_sha,
	       repo.owner, repo.name, pr.number, pr.title, pr.author, pr.url, pr.state,
	       pr.files_changed, pr.additions, pr.deletions
	FROM reviews r
	JOIN pull_requests pr ON pr.id = r.pull_request_id
	JOIN repositories repo ON repo.id = pr.repository_id`

const findingColumns = `id, skill_id, check_id, source, severity, confidence, file,
	line, end_line, evidence, found, why, improvement, lesson_id`

type rowScanner interface{ Scan(dest ...any) error }

func scanReview(row rowScanner) (review.Review, error) {
	var (
		rv              review.Review
		id              int64
		errCode, errMsg sql.NullString
		completedAt     sql.NullTime
		status          string
	)
	err := row.Scan(&id, &status, &rv.StatusMessage, &rv.StatusAt, &errCode, &errMsg,
		&rv.CreatedAt, &completedAt, &rv.PullRequest.HeadSHA,
		&rv.PullRequest.Owner, &rv.PullRequest.Repo, &rv.PullRequest.Number,
		&rv.PullRequest.Title, &rv.PullRequest.Author, &rv.PullRequest.URL, &rv.PullRequest.State,
		&rv.PullRequest.FilesChanged, &rv.PullRequest.Additions, &rv.PullRequest.Deletions)
	if err != nil {
		return review.Review{}, err
	}

	rv.ID = strconv.FormatInt(id, 10)
	rv.Status = review.Status(status)
	if completedAt.Valid {
		t := completedAt.Time
		rv.CompletedAt = &t
	}
	if errCode.Valid {
		rv.Error = &review.ErrorInfo{Code: errCode.String, Message: errMsg.String}
	}
	return rv, nil
}

func scanFinding(row rowScanner) (review.Finding, error) {
	var (
		f           review.Finding
		id          int64
		source, sev string
		line, end   sql.NullInt32
		lesson      sql.NullString
		confidence  float32
	)
	err := row.Scan(&id, &f.SkillID, &f.CheckID, &source, &sev, &confidence, &f.File,
		&line, &end, &f.Evidence, &f.Found, &f.Why, &f.Improvement, &lesson)
	if err != nil {
		return review.Finding{}, err
	}
	f.ID = strconv.FormatInt(id, 10)
	f.Source = review.Source(source)
	f.Severity = review.Severity(sev)
	f.Confidence = float64(confidence)
	f.Line = int(line.Int32)
	f.EndLine = int(end.Int32)
	f.Lesson = lesson.String
	return f, nil
}

// CreateReview records a new queued review for a pull request, creating the
// repository and pull request rows on first sight.
func (s *Store) CreateReview(ctx context.Context, owner, repo string, number int) (review.Review, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return review.Review{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	var repoID, prID, reviewID int64
	// The no-op update makes RETURNING yield the existing row on conflict.
	err = tx.QueryRowContext(ctx, `
		INSERT INTO repositories (owner, name) VALUES ($1, $2)
		ON CONFLICT (lower(owner), lower(name)) DO UPDATE SET owner = repositories.owner
		RETURNING id`, owner, repo).Scan(&repoID)
	if err != nil {
		return review.Review{}, fmt.Errorf("upsert repository: %w", err)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO pull_requests (repository_id, number) VALUES ($1, $2)
		ON CONFLICT (repository_id, number) DO UPDATE SET updated_at = now()
		RETURNING id`, repoID, number).Scan(&prID)
	if err != nil {
		return review.Review{}, fmt.Errorf("upsert pull request: %w", err)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO reviews (pull_request_id, status_message) VALUES ($1, 'Waiting to start')
		RETURNING id`, prID).Scan(&reviewID)
	if err != nil {
		return review.Review{}, fmt.Errorf("insert review: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return review.Review{}, fmt.Errorf("commit: %w", err)
	}
	return s.GetReview(ctx, strconv.FormatInt(reviewID, 10))
}

// GetReview returns a review with its findings.
func (s *Store) GetReview(ctx context.Context, id string) (review.Review, error) {
	n, ok := parseID(id)
	if !ok {
		return review.Review{}, review.ErrReviewNotFound
	}

	rv, err := scanReview(s.db.QueryRowContext(ctx, reviewSelect+" WHERE r.id = $1", n))
	if errors.Is(err, sql.ErrNoRows) {
		return review.Review{}, review.ErrReviewNotFound
	}
	if err != nil {
		return review.Review{}, fmt.Errorf("get review %s: %w", id, err)
	}

	rows, err := s.db.QueryContext(ctx, "SELECT "+findingColumns+" FROM findings WHERE review_id = $1 ORDER BY id", n)
	if err != nil {
		return review.Review{}, fmt.Errorf("get findings of review %s: %w", id, err)
	}
	defer rows.Close()

	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return review.Review{}, fmt.Errorf("scan finding: %w", err)
		}
		rv.Findings = append(rv.Findings, f)
	}
	if err := rows.Err(); err != nil {
		return review.Review{}, fmt.Errorf("read findings: %w", err)
	}
	rv.Counts = review.CountFindings(rv.Findings)
	return rv, nil
}

// ListReviews returns the most recent reviews, newest first, without their
// findings but with per-severity counts.
func (s *Store) ListReviews(ctx context.Context, limit int) ([]review.Review, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.status, r.status_message, r.status_at, r.error_code, r.error_message,
		       r.created_at, r.completed_at, r.head_sha,
		       repo.owner, repo.name, pr.number, pr.title, pr.author, pr.url, pr.state,
		       pr.files_changed, pr.additions, pr.deletions,
		       c.critical, c.high, c.medium, c.suggestion
		FROM reviews r
		JOIN pull_requests pr ON pr.id = r.pull_request_id
		JOIN repositories repo ON repo.id = pr.repository_id
		CROSS JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE f.severity = 'critical')   AS critical,
			       COUNT(*) FILTER (WHERE f.severity = 'high')       AS high,
			       COUNT(*) FILTER (WHERE f.severity = 'medium')     AS medium,
			       COUNT(*) FILTER (WHERE f.severity = 'suggestion') AS suggestion
			FROM findings f WHERE f.review_id = r.id
		) c
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	var out []review.Review
	for rows.Next() {
		var counts review.Counts
		rv, err := scanReview(&withCounts{rows: rows, counts: &counts})
		if err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		rv.Counts = counts
		out = append(out, rv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read reviews: %w", err)
	}
	return out, nil
}

// withCounts lets scanReview read the 19 review columns while capturing the 4
// trailing count columns of the list query.
type withCounts struct {
	rows   *sql.Rows
	counts *review.Counts
}

func (w *withCounts) Scan(dest ...any) error {
	all := append(dest, &w.counts.Critical, &w.counts.High, &w.counts.Medium, &w.counts.Suggestion)
	return w.rows.Scan(all...)
}

// GetFinding returns one finding of a review.
func (s *Store) GetFinding(ctx context.Context, reviewID, findingID string) (review.Finding, error) {
	rid, ok1 := parseID(reviewID)
	fid, ok2 := parseID(findingID)
	if !ok1 || !ok2 {
		return review.Finding{}, review.ErrFindingNotFound
	}

	f, err := scanFinding(s.db.QueryRowContext(ctx,
		"SELECT "+findingColumns+" FROM findings WHERE id = $1 AND review_id = $2", fid, rid))
	if errors.Is(err, sql.ErrNoRows) {
		return review.Finding{}, review.ErrFindingNotFound
	}
	if err != nil {
		return review.Finding{}, fmt.Errorf("get finding %s: %w", findingID, err)
	}
	return f, nil
}

// SetStatus moves an active review to a new status and records the progress line.
func (s *Store) SetStatus(ctx context.Context, id string, status review.Status, message string) error {
	n, ok := parseID(id)
	if !ok {
		return review.ErrReviewNotFound
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE reviews SET status = $2, status_message = $3, status_at = now()
		WHERE id = $1 AND status NOT IN ('completed', 'failed')`, n, string(status), message)
	if err != nil {
		return fmt.Errorf("set status of review %s: %w", id, err)
	}
	return s.requireChange(ctx, res, n)
}

// SavePullRequest stores what GitHub told us about the pull request.
func (s *Store) SavePullRequest(ctx context.Context, id string, pr review.PullRequest) error {
	n, ok := parseID(id)
	if !ok {
		return review.ErrReviewNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	var prID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE reviews SET head_sha = $2 WHERE id = $1 RETURNING pull_request_id`, n, pr.HeadSHA).Scan(&prID)
	if errors.Is(err, sql.ErrNoRows) {
		return review.ErrReviewNotFound
	}
	if err != nil {
		return fmt.Errorf("set head sha: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE pull_requests
		SET title = $2, author = $3, url = $4, state = $5, files_changed = $6,
		    additions = $7, deletions = $8, updated_at = now()
		WHERE id = $1`, prID, pr.Title, pr.Author, pr.URL, pr.State, pr.FilesChanged, pr.Additions, pr.Deletions)
	if err != nil {
		return fmt.Errorf("update pull request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// CompleteReview saves the findings and marks the review completed in one
// transaction, so a reader never sees a completed review with missing findings.
func (s *Store) CompleteReview(ctx context.Context, id string, findings []review.Finding, message string) error {
	n, ok := parseID(id)
	if !ok {
		return review.ErrReviewNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	res, err := tx.ExecContext(ctx, `
		UPDATE reviews
		SET status = 'completed', status_message = $2, status_at = now(), completed_at = now()
		WHERE id = $1 AND status NOT IN ('completed', 'failed')`, n, message)
	if err != nil {
		return fmt.Errorf("mark review completed: %w", err)
	}
	if err := s.requireChange(ctx, res, n); err != nil {
		return err
	}

	for _, f := range findings {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO findings (review_id, skill_id, check_id, source, severity, confidence,
			                      file, line, end_line, evidence, found, why, improvement, lesson_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			n, f.SkillID, f.CheckID, string(f.Source), string(f.Severity), f.Confidence,
			f.File, nullableInt(f.Line), nullableInt(f.EndLine), f.Evidence, f.Found, f.Why, f.Improvement,
			sql.NullString{String: f.Lesson, Valid: f.Lesson != ""})
		if err != nil {
			return fmt.Errorf("insert finding %s/%s: %w", f.SkillID, f.CheckID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// FailReview marks an active review failed.
func (s *Store) FailReview(ctx context.Context, id, code, message string) error {
	n, ok := parseID(id)
	if !ok {
		return review.ErrReviewNotFound
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE reviews
		SET status = 'failed', status_message = $3, status_at = now(), completed_at = now(),
		    error_code = $2, error_message = $3
		WHERE id = $1 AND status NOT IN ('completed', 'failed')`, n, code, message)
	if err != nil {
		return fmt.Errorf("fail review %s: %w", id, err)
	}
	return s.requireChange(ctx, res, n)
}

// FailInterrupted marks every review still in progress as failed. Call it at
// startup: a review that was running when the previous process died will never finish.
func (s *Store) FailInterrupted(ctx context.Context) (int, error) {
	const message = "The server restarted while this review was running. Please start it again."
	res, err := s.db.ExecContext(ctx, `
		UPDATE reviews
		SET status = 'failed', status_message = $1, status_at = now(), completed_at = now(),
		    error_code = 'interrupted', error_message = $1
		WHERE status NOT IN ('completed', 'failed')`, message)
	if err != nil {
		return 0, fmt.Errorf("fail interrupted reviews: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count interrupted reviews: %w", err)
	}
	return int(n), nil
}

// requireChange turns "no row updated" into the right error: either the review
// does not exist or it had already finished.
func (s *Store) requireChange(ctx context.Context, res sql.Result, id int64) error {
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update: %w", err)
	}
	if changed > 0 {
		return nil
	}

	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM reviews WHERE id = $1)", id).Scan(&exists); err != nil {
		return fmt.Errorf("check review exists: %w", err)
	}
	if !exists {
		return review.ErrReviewNotFound
	}
	return review.ErrReviewFinished
}
