package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
)

func testProfile(githubID int64, login string) auth.Profile {
	return auth.Profile{ID: githubID, Login: login, Name: "Test User", Email: "t@example.com", AvatarURL: "https://example.com/a.png"}
}

func TestUpsertGitHubUserCreatesThenUpdates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	created, err := s.UpsertGitHubUser(ctx, testProfile(555, "chirag"))
	if err != nil {
		t.Fatalf("UpsertGitHubUser: %v", err)
	}
	if created.GitHubLogin != "chirag" || created.Name != "Test User" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}

	// Same github_id, changed profile fields (e.g. renamed on GitHub): same
	// row, updated fields.
	updated := testProfile(555, "chirag-renamed")
	updated.Name = "New Name"
	again, err := s.UpsertGitHubUser(ctx, updated)
	if err != nil {
		t.Fatalf("second UpsertGitHubUser: %v", err)
	}
	if again.ID != created.ID {
		t.Fatalf("id changed: %s -> %s, want the same row", created.ID, again.ID)
	}
	if again.GitHubLogin != "chirag-renamed" || again.Name != "New Name" {
		t.Fatalf("profile fields did not refresh: %+v", again)
	}

	var count int
	s.db.QueryRow("SELECT count(*) FROM users WHERE github_id = 555").Scan(&count)
	if count != 1 {
		t.Fatalf("users rows for github_id 555 = %d, want 1", count)
	}
}

func TestSaveAndGetTokens(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, _ := s.UpsertGitHubUser(ctx, testProfile(1, "a"))

	expires := time.Now().Add(8 * time.Hour).Truncate(time.Second)
	if err := s.SaveTokens(ctx, u.ID, []byte("access-ciphertext"), []byte("refresh-ciphertext"), &expires); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	got, err := s.GetTokens(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetTokens: %v", err)
	}
	if string(got.AccessEnc) != "access-ciphertext" || string(got.RefreshEnc) != "refresh-ciphertext" {
		t.Fatalf("tokens = %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("expiresAt = %v, want %v", got.ExpiresAt, expires)
	}
}

func TestSaveTokensWithoutExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, _ := s.UpsertGitHubUser(ctx, testProfile(2, "b"))

	if err := s.SaveTokens(ctx, u.ID, []byte("access"), nil, nil); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	got, err := s.GetTokens(ctx, u.ID)
	if err != nil || got.RefreshEnc != nil || got.ExpiresAt != nil {
		t.Fatalf("tokens = %+v, err = %v; want no refresh token or expiry", got, err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, _ := s.UpsertGitHubUser(ctx, testProfile(3, "c"))

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, hash, u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSessionUser(ctx, auth.HashSessionToken(token))
	if err != nil || got.ID != u.ID || got.GitHubLogin != "c" {
		t.Fatalf("GetSessionUser = %+v, %v", got, err)
	}

	if err := s.DeleteSession(ctx, hash); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSessionUser(ctx, hash); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("after delete, error = %v, want ErrSessionNotFound", err)
	}
}

func TestExpiredSessionIsNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, _ := s.UpsertGitHubUser(ctx, testProfile(4, "d"))

	_, hash, _ := auth.NewSessionToken()
	if err := s.CreateSession(ctx, hash, u.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSessionUser(ctx, hash); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound for an expired session", err)
	}
}

func TestUnknownSessionIsNotFound(t *testing.T) {
	s := testStore(t)
	_, hash, _ := auth.NewSessionToken()
	if _, err := s.GetSessionUser(context.Background(), hash); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	u, _ := s.UpsertGitHubUser(ctx, testProfile(5, "e"))

	_, expiredHash, _ := auth.NewSessionToken()
	s.CreateSession(ctx, expiredHash, u.ID, time.Now().Add(-time.Hour))
	_, liveHash, _ := auth.NewSessionToken()
	s.CreateSession(ctx, liveHash, u.ID, time.Now().Add(time.Hour))

	n, err := s.DeleteExpiredSessions(ctx)
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpiredSessions = %d, %v; want 1 deleted", n, err)
	}
	if _, err := s.GetSessionUser(ctx, liveHash); err != nil {
		t.Fatalf("the live session should survive: %v", err)
	}
}
