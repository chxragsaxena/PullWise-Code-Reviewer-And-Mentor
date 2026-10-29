package store

import (
	"context"
	"errors"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func rule(title string, enabled bool) review.Rule {
	return review.Rule{Title: title, Instructions: "do " + title, Severity: review.SeverityMedium, Enabled: enabled}
}

func TestRulesBelongToTheirOwner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))
	bob, _ := s.UpsertGitHubUser(ctx, testProfile(2, "bob"))

	mine, err := s.CreateRule(ctx, alice.ID, rule("wrap errors", true), 20)
	if err != nil || mine.ID == "" || mine.Title != "wrap errors" || !mine.Enabled {
		t.Fatalf("CreateRule = %+v, %v", mine, err)
	}

	if got, _ := s.ListRules(ctx, bob.ID); len(got) != 0 {
		t.Errorf("bob sees %d of alice's rules", len(got))
	}
	if _, err := s.UpdateRule(ctx, bob.ID, mine.ID, rule("hijack", true)); !errors.Is(err, review.ErrRuleNotFound) {
		t.Errorf("bob updating alice's rule: %v, want ErrRuleNotFound", err)
	}
	if err := s.DeleteRule(ctx, bob.ID, mine.ID); !errors.Is(err, review.ErrRuleNotFound) {
		t.Errorf("bob deleting alice's rule: %v, want ErrRuleNotFound", err)
	}

	updated, err := s.UpdateRule(ctx, alice.ID, mine.ID, rule("wrap errors v2", false))
	if err != nil || updated.Title != "wrap errors v2" || updated.Enabled {
		t.Fatalf("UpdateRule = %+v, %v", updated, err)
	}
	if err := s.DeleteRule(ctx, alice.ID, mine.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListRules(ctx, alice.ID); len(got) != 0 {
		t.Error("the rule was not deleted")
	}
	if err := s.DeleteRule(ctx, alice.ID, "not-a-number"); !errors.Is(err, review.ErrRuleNotFound) {
		t.Errorf("a malformed id: %v, want ErrRuleNotFound", err)
	}
}

func TestTheRuleLimitIsEnforcedPerUser(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))
	bob, _ := s.UpsertGitHubUser(ctx, testProfile(2, "bob"))

	for _, title := range []string{"a", "b"} {
		if _, err := s.CreateRule(ctx, alice.ID, rule(title, true), 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateRule(ctx, alice.ID, rule("c", true), 2); !errors.Is(err, review.ErrRuleLimit) {
		t.Fatalf("third rule: %v, want ErrRuleLimit", err)
	}
	if _, err := s.CreateRule(ctx, bob.ID, rule("a", true), 2); err != nil {
		t.Fatalf("bob is blocked by alice's rules: %v", err)
	}
}

func TestAReviewKeepsACopyOfTheOwnersEnabledRules(t *testing.T) {
	s := testStore(t)
	bg := context.Background()
	alice, _ := s.UpsertGitHubUser(bg, testProfile(1, "alice"))
	bob, _ := s.UpsertGitHubUser(bg, testProfile(2, "bob"))
	on, _ := s.CreateRule(bg, alice.ID, rule("enabled rule", true), 20)
	_, _ = s.CreateRule(bg, alice.ID, rule("disabled rule", false), 20)
	_, _ = s.CreateRule(bg, bob.ID, rule("bob's rule", true), 20)

	asAlice := review.WithViewer(bg, alice.ID)
	rv, err := s.CreateReview(asAlice, "o", "r", 1)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ReviewRules(bg, rv.ID)
	if err != nil || len(got) != 1 || got[0].Title != "enabled rule" || got[0].ID != on.ID {
		t.Fatalf("snapshot = %+v, %v; want only alice's enabled rule", got, err)
	}

	// Editing or deleting the rule afterwards does not change what the review used.
	_, _ = s.UpdateRule(bg, alice.ID, on.ID, rule("edited later", true))
	_ = s.DeleteRule(bg, alice.ID, on.ID)
	got, _ = s.ReviewRules(bg, rv.ID)
	if len(got) != 1 || got[0].Title != "enabled rule" {
		t.Fatalf("snapshot after edit and delete = %+v, want it unchanged", got)
	}
}

func TestAnonymousReviewsHaveNoRules(t *testing.T) {
	s := testStore(t)
	bg := context.Background()
	alice, _ := s.UpsertGitHubUser(bg, testProfile(1, "alice"))
	_, _ = s.CreateRule(bg, alice.ID, rule("alice's rule", true), 20)

	rv, err := s.CreateReview(bg, "o", "r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ReviewRules(bg, rv.ID); len(got) != 0 {
		t.Fatalf("an anonymous review got rules: %+v", got)
	}
}

func TestDeletingAUserDeletesTheirRules(t *testing.T) {
	s := testStore(t)
	bg := context.Background()
	alice, _ := s.UpsertGitHubUser(bg, testProfile(1, "alice"))
	_, _ = s.CreateRule(bg, alice.ID, rule("a", true), 20)

	if _, err := s.db.ExecContext(bg, "DELETE FROM users WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM custom_rules").Scan(&n)
	if n != 0 {
		t.Fatalf("%d rules left behind", n)
	}
}
