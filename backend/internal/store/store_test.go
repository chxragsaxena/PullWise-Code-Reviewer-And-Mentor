package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/migrations"
)

// testStore returns a Store backed by a freshly migrated database. It needs a
// throwaway PostgreSQL database named *_test in TEST_DATABASE_URL, and skips
// otherwise. The schema is wiped, so it refuses to touch any other database.
func testStore(t *testing.T) *Store {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(u.Path, "/"), "_test") {
		t.Fatalf("TEST_DATABASE_URL must point at a database whose name ends in _test (it is wiped)")
	}

	ctx := context.Background()
	db, err := Open(ctx, raw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	s := New(db)
	if err := s.SyncSkills(ctx, []skills.Skill{{ID: "go", Name: "Go", Version: 1, Languages: []string{"go"}}}); err != nil {
		t.Fatalf("sync skills: %v", err)
	}
	return s
}

func finding(check string, sev review.Severity, line int) review.Finding {
	return review.Finding{
		SkillID: "go", CheckID: check, Source: review.SourceDeterministic, Severity: sev, Confidence: 1,
		File: "main.go", Line: line, Evidence: "x := f()", Found: "found", Why: "why", Improvement: "fix", Lesson: "error-handling",
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := testStore(t)
	if err := Migrate(context.Background(), s.db, migrations.Files); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 1 {
		t.Fatalf("schema_migrations rows = %d, err = %v; want exactly 1", n, err)
	}
}

func TestCreateReviewStartsQueuedAndSharesRepositoryRows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	first, err := s.CreateReview(ctx, "Acme", "Shop", 7)
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if first.Status != review.StatusQueued || first.PullRequest.Owner != "Acme" || first.PullRequest.Number != 7 || first.ID == "" {
		t.Fatalf("unexpected review: %+v", first)
	}
	if first.Error != nil || first.CompletedAt != nil || len(first.Findings) != 0 {
		t.Fatalf("a new review should have no error, completion time or findings: %+v", first)
	}

	// Same PR, different letter case: a second review, but the same repository row.
	second, err := s.CreateReview(ctx, "acme", "shop", 7)
	if err != nil {
		t.Fatalf("second CreateReview: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("expected a distinct review id")
	}
	var repos, prs int
	s.db.QueryRow("SELECT count(*) FROM repositories").Scan(&repos)
	s.db.QueryRow("SELECT count(*) FROM pull_requests").Scan(&prs)
	if repos != 1 || prs != 1 {
		t.Fatalf("repositories = %d, pull_requests = %d; want 1 each", repos, prs)
	}
}

func TestReviewLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.CreateReview(ctx, "o", "r", 3)
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID

	if err := s.SetStatus(ctx, id, review.StatusFetchingPR, "Fetching"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	err = s.SavePullRequest(ctx, id, review.PullRequest{
		Title: "Add caching", Author: "chirag", URL: "https://github.com/o/r/pull/3", State: "open",
		HeadSHA: "abc123", FilesChanged: 4, Additions: 30, Deletions: 2,
	})
	if err != nil {
		t.Fatalf("SavePullRequest: %v", err)
	}

	whole := finding("high-complexity", review.SeverityMedium, 0) // whole-file: no line
	findings := []review.Finding{finding("ignored-error", review.SeverityHigh, 42), whole}
	findings[0].EndLine = 44
	if err := s.CompleteReview(ctx, id, findings, "Review complete"); err != nil {
		t.Fatalf("CompleteReview: %v", err)
	}

	got, err := s.GetReview(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != review.StatusCompleted || got.CompletedAt == nil || got.Error != nil {
		t.Fatalf("status/completion wrong: %+v", got)
	}
	if got.PullRequest.Title != "Add caching" || got.PullRequest.HeadSHA != "abc123" || got.PullRequest.Additions != 30 {
		t.Fatalf("pull request not saved: %+v", got.PullRequest)
	}
	if got.Counts != (review.Counts{High: 1, Medium: 1}) || len(got.Findings) != 2 {
		t.Fatalf("counts = %+v, findings = %d", got.Counts, len(got.Findings))
	}
	first, second := got.Findings[0], got.Findings[1]
	if first.Line != 42 || first.EndLine != 44 || first.Lesson != "error-handling" || first.Confidence != 1 {
		t.Fatalf("first finding wrong: %+v", first)
	}
	if second.Line != 0 || second.EndLine != 0 {
		t.Fatalf("whole-file finding should have no line: %+v", second)
	}

	one, err := s.GetFinding(ctx, id, first.ID)
	if err != nil || one.CheckID != "ignored-error" {
		t.Fatalf("GetFinding = %+v, %v", one, err)
	}

	list, err := s.ListReviews(ctx, 10)
	if err != nil || len(list) != 1 || list[0].Counts != got.Counts || len(list[0].Findings) != 0 {
		t.Fatalf("ListReviews = %+v, %v; want one review with counts and no findings", list, err)
	}
}

func TestCompleteReviewIsAtomic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	created, _ := s.CreateReview(ctx, "o", "r", 1)

	bad := finding("x", review.SeverityHigh, 1)
	bad.SkillID = "no-such-skill" // violates the foreign key on the second insert
	err := s.CompleteReview(ctx, created.ID, []review.Finding{finding("ok", review.SeverityHigh, 1), bad}, "done")
	if err == nil {
		t.Fatal("expected the foreign key violation to fail the whole completion")
	}

	got, _ := s.GetReview(ctx, created.ID)
	if got.Status != review.StatusQueued || len(got.Findings) != 0 {
		t.Fatalf("nothing should be saved on failure, got status %s with %d findings", got.Status, len(got.Findings))
	}
}

func TestFinishedReviewsRefuseFurtherUpdates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	done, _ := s.CreateReview(ctx, "o", "r", 1)
	if err := s.CompleteReview(ctx, done.ID, nil, "done"); err != nil {
		t.Fatal(err)
	}
	failed, _ := s.CreateReview(ctx, "o", "r", 2)
	if err := s.FailReview(ctx, failed.ID, "pr_not_found", "gone"); err != nil {
		t.Fatal(err)
	}

	for name, err := range map[string]error{
		"SetStatus on completed":      s.SetStatus(ctx, done.ID, review.StatusAnalyzing, "x"),
		"CompleteReview on completed": s.CompleteReview(ctx, done.ID, nil, "again"),
		"FailReview on completed":     s.FailReview(ctx, done.ID, "c", "m"),
		"SetStatus on failed":         s.SetStatus(ctx, failed.ID, review.StatusAnalyzing, "x"),
	} {
		if !errors.Is(err, review.ErrReviewFinished) {
			t.Errorf("%s: error = %v, want ErrReviewFinished", name, err)
		}
	}

	got, _ := s.GetReview(ctx, failed.ID)
	if got.Status != review.StatusFailed || got.Error == nil || got.Error.Code != "pr_not_found" || got.CompletedAt == nil {
		t.Fatalf("failed review wrong: %+v", got)
	}
}

func TestNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, id := range []string{"999999", "0", "-1", "abc", ""} {
		if _, err := s.GetReview(ctx, id); !errors.Is(err, review.ErrReviewNotFound) {
			t.Errorf("GetReview(%q) error = %v, want ErrReviewNotFound", id, err)
		}
	}
	if err := s.SetStatus(ctx, "999999", review.StatusAnalyzing, "x"); !errors.Is(err, review.ErrReviewNotFound) {
		t.Errorf("SetStatus on unknown review = %v, want ErrReviewNotFound", err)
	}

	created, _ := s.CreateReview(ctx, "o", "r", 1)
	if _, err := s.GetFinding(ctx, created.ID, "12345"); !errors.Is(err, review.ErrFindingNotFound) {
		t.Errorf("GetFinding = %v, want ErrFindingNotFound", err)
	}
}

func TestFailInterruptedOnlyTouchesActiveReviews(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	active, _ := s.CreateReview(ctx, "o", "r", 1)
	running, _ := s.CreateReview(ctx, "o", "r", 2)
	_ = s.SetStatus(ctx, running.ID, review.StatusAnalyzing, "busy")
	done, _ := s.CreateReview(ctx, "o", "r", 3)
	_ = s.CompleteReview(ctx, done.ID, nil, "done")

	n, err := s.FailInterrupted(ctx)
	if err != nil || n != 2 {
		t.Fatalf("FailInterrupted = %d, %v; want 2", n, err)
	}
	for _, id := range []string{active.ID, running.ID} {
		got, _ := s.GetReview(ctx, id)
		if got.Status != review.StatusFailed || got.Error == nil || got.Error.Code != "interrupted" {
			t.Errorf("review %s = %+v, want failed/interrupted", id, got)
		}
	}
	if got, _ := s.GetReview(ctx, done.ID); got.Status != review.StatusCompleted {
		t.Errorf("completed review changed to %s", got.Status)
	}
}

func TestListReviewsNewestFirstWithLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var ids []string
	for n := 1; n <= 3; n++ {
		r, _ := s.CreateReview(ctx, "o", "r", n)
		ids = append(ids, r.ID)
	}

	got, err := s.ListReviews(ctx, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListReviews = %d reviews, %v; want 2", len(got), err)
	}
	if got[0].ID != ids[2] || got[1].ID != ids[1] {
		t.Fatalf("order = %s,%s; want newest first %s,%s", got[0].ID, got[1].ID, ids[2], ids[1])
	}
}
