package store

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestRememberedReposComeBackMostRecentFirst(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))

	for _, repo := range []string{"go-chi/chi", "pallets/flask", "go-chi/chi", "acme/shop"} {
		if err := s.RememberRepo(ctx, alice.ID, repo); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.RecentRepos(ctx, alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	// chi was used again after flask, so it moves back to the front and is stored once.
	if want := []string{"acme/shop", "go-chi/chi", "pallets/flask"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent = %v, want %v", got, want)
	}

	var uses int
	_ = s.db.QueryRowContext(ctx, "SELECT uses FROM user_repos WHERE repo = 'go-chi/chi'").Scan(&uses)
	if uses != 2 {
		t.Errorf("uses = %d, want 2", uses)
	}
	if limited, _ := s.RecentRepos(ctx, alice.ID, 2); len(limited) != 2 {
		t.Errorf("limit 2 returned %d", len(limited))
	}
}

func TestRememberedReposBelongToTheirUser(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))
	bob, _ := s.UpsertGitHubUser(ctx, testProfile(2, "bob"))

	if err := s.RememberRepo(ctx, alice.ID, "alice/secret-project"); err != nil {
		t.Fatal(err)
	}

	if got, _ := s.RecentRepos(ctx, bob.ID, 10); len(got) != 0 {
		t.Errorf("bob sees alice's repositories: %v", got)
	}
	if got, _ := s.RecentRepos(ctx, "not-a-number", 10); len(got) != 0 {
		t.Errorf("a malformed user id returned %v", got)
	}
}

func TestOnlyTheLatestFiftyReposAreKept(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))

	for i := 0; i < maxRememberedRepos+5; i++ {
		if err := s.RememberRepo(ctx, alice.ID, fmt.Sprintf("org/repo%03d", i)); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := s.RecentRepos(ctx, alice.ID, 100)
	if len(got) != maxRememberedRepos {
		t.Fatalf("kept %d repositories, want %d", len(got), maxRememberedRepos)
	}
	if got[0] != fmt.Sprintf("org/repo%03d", maxRememberedRepos+4) {
		t.Errorf("newest = %q", got[0])
	}
	for _, repo := range got {
		if repo == "org/repo000" {
			t.Error("the oldest repository was not dropped")
		}
	}
}

func TestRememberRepoRejectsNamesThatAreNotRepositories(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))

	for _, bad := range []string{"", "no-slash", "Upper/Case", "a/b/c", "a/", "/b", "a b/c", "x'; DROP TABLE users; --/y"} {
		if err := s.RememberRepo(ctx, alice.ID, bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if err := s.RememberRepo(ctx, "nope", "a/b"); err == nil {
		t.Error("a malformed user id was accepted")
	}
	if got, _ := s.RecentRepos(ctx, alice.ID, 10); len(got) != 0 {
		t.Errorf("rejected names were stored: %v", got)
	}
}

func TestDeletingAUserForgetsTheirRepos(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, _ := s.UpsertGitHubUser(ctx, testProfile(1, "alice"))
	_ = s.RememberRepo(ctx, alice.ID, "a/b")

	if _, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM user_repos").Scan(&n)
	if n != 0 {
		t.Errorf("%d rows left after the user was deleted", n)
	}
}
