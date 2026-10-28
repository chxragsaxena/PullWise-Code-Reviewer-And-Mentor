package store

import (
	"context"
	"errors"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func ids(rs []review.Review) map[string]bool {
	out := map[string]bool{}
	for _, r := range rs {
		out[r.ID] = true
	}
	return out
}

func TestReviewsMadeWhileSignedInArePrivateToTheirOwner(t *testing.T) {
	s := testStore(t)
	bg := context.Background()
	alice, _ := s.UpsertGitHubUser(bg, testProfile(1, "alice"))
	bob, _ := s.UpsertGitHubUser(bg, testProfile(2, "bob"))
	asAlice := review.WithViewer(bg, alice.ID)
	asBob := review.WithViewer(bg, bob.ID)

	public, err := s.CreateReview(bg, "o", "public", 1)
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.CreateReview(asAlice, "o", "private", 2)
	if err != nil {
		t.Fatalf("an owner must be able to read back the review they just made: %v", err)
	}
	if err := s.CompleteReview(bg, private.ID, []review.Finding{finding("ignored-error", review.SeverityHigh, 1)}, "done"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetReview(asAlice, private.ID)
	findingID := got.Findings[0].ID

	tests := []struct {
		name        string
		ctx         context.Context
		wantPrivate bool // can this viewer see Alice's private review?
	}{
		{"owner", asAlice, true},
		{"another signed-in user", asBob, false},
		{"anonymous", bg, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.GetReview(tt.ctx, private.ID)
			if tt.wantPrivate != (err == nil) {
				t.Errorf("GetReview err = %v, visible = %v", err, tt.wantPrivate)
			}
			if !tt.wantPrivate && !errors.Is(err, review.ErrReviewNotFound) {
				t.Errorf("a hidden review should look exactly like a missing one, got %v", err)
			}

			_, err = s.GetFinding(tt.ctx, private.ID, findingID)
			if tt.wantPrivate != (err == nil) {
				t.Errorf("GetFinding err = %v, visible = %v", err, tt.wantPrivate)
			}
			if !tt.wantPrivate && !errors.Is(err, review.ErrFindingNotFound) {
				t.Errorf("a hidden finding should look like a missing one, got %v", err)
			}

			listed, err := s.ListReviews(tt.ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(listed)[private.ID]; got != tt.wantPrivate {
				t.Errorf("private review in list = %v, want %v", got, tt.wantPrivate)
			}
			if !ids(listed)[public.ID] {
				t.Error("an anonymous review must stay visible to everyone")
			}
		})
	}
}

func TestDeletingAUserDeletesTheirPrivateReviewsInsteadOfPublishingThem(t *testing.T) {
	s := testStore(t)
	bg := context.Background()
	alice, _ := s.UpsertGitHubUser(bg, testProfile(1, "alice"))
	private, err := s.CreateReview(review.WithViewer(bg, alice.ID), "o", "r", 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.ExecContext(bg, "DELETE FROM users WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetReview(bg, private.ID); !errors.Is(err, review.ErrReviewNotFound) {
		t.Fatalf("after the owner is deleted the review should be gone, got %v", err)
	}
}
