package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	mu       sync.Mutex
	nextID   int
	byGithub map[int64]User
	tokens   map[string]Tokens
	sessions map[SessionTokenHash]struct {
		userID    string
		expiresAt time.Time
	}
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byGithub: map[int64]User{},
		tokens:   map[string]Tokens{},
		sessions: map[SessionTokenHash]struct {
			userID    string
			expiresAt time.Time
		}{},
	}
}

func (f *fakeStore) UpsertGitHubUser(_ context.Context, p Profile) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byGithub[p.ID]; ok {
		u.GitHubLogin, u.Name, u.AvatarURL = p.Login, p.Name, p.AvatarURL
		f.byGithub[p.ID] = u
		return u, nil
	}
	f.nextID++
	u := User{ID: strconv.Itoa(f.nextID), GitHubLogin: p.Login, Name: p.Name, AvatarURL: p.AvatarURL}
	f.byGithub[p.ID] = u
	return u, nil
}

func (f *fakeStore) SaveTokens(_ context.Context, userID string, accessEnc, refreshEnc []byte, expiresAt *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[userID] = Tokens{AccessEnc: accessEnc, RefreshEnc: refreshEnc, ExpiresAt: expiresAt}
	return nil
}

func (f *fakeStore) GetTokens(_ context.Context, userID string) (Tokens, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[userID]
	if !ok {
		return Tokens{}, errors.New("no such user")
	}
	return t, nil
}

func (f *fakeStore) CreateSession(_ context.Context, hash SessionTokenHash, userID string, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[hash] = struct {
		userID    string
		expiresAt time.Time
	}{userID, expiresAt}
	return nil
}

func (f *fakeStore) GetSessionUser(_ context.Context, hash SessionTokenHash) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[hash]
	if !ok || time.Now().After(sess.expiresAt) {
		return User{}, errors.New("session not found")
	}
	for _, u := range f.byGithub {
		if u.ID == sess.userID {
			return u, nil
		}
	}
	return User{}, errors.New("session not found")
}

func (f *fakeStore) DeleteSession(_ context.Context, hash SessionTokenHash) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, hash)
	return nil
}

func newTestService(t *testing.T, tokenResp, userResp func(r *http.Request) (int, string)) (*Service, *fakeStore) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var status int
		var body string
		if r.URL.Path == "/token" {
			status, body = tokenResp(r)
		} else {
			status, body = userResp(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	oauth := NewOAuth("id", "secret", "http://localhost:8080/auth/github/callback",
		WithEndpoints(srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/user"))
	key := testKey(t)
	crypt, err := NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore()
	return NewService(oauth, crypt, store), store
}

func defaultResponses() (func(*http.Request) (int, string), func(*http.Request) (int, string)) {
	tokenResp := func(*http.Request) (int, string) {
		return 200, `{"access_token":"tok_abc","refresh_token":"ref_xyz","expires_in":28800}`
	}
	userResp := func(*http.Request) (int, string) {
		return 200, `{"id":42,"login":"chirag","name":"Chirag","email":"c@example.com","avatar_url":"https://x/a.png"}`
	}
	return tokenResp, userResp
}

func TestLoginReturnsAWorkingURLAndState(t *testing.T) {
	svc, _ := newTestService(t, nil, nil)
	redirectURL, state, err := svc.Login()
	if err != nil {
		t.Fatal(err)
	}
	if state == "" || redirectURL == "" {
		t.Fatalf("redirectURL=%q state=%q", redirectURL, state)
	}
}

func TestCallbackHappyPath(t *testing.T) {
	tokenResp, userResp := defaultResponses()
	svc, store := newTestService(t, tokenResp, userResp)

	token, expiresAt, err := svc.Callback(context.Background(), "code", "state1", "state1")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if token == "" || expiresAt.Before(time.Now()) {
		t.Fatalf("token=%q expiresAt=%v", token, expiresAt)
	}

	user, err := svc.CurrentUser(context.Background(), token)
	if err != nil || user.GitHubLogin != "chirag" || user.Name != "Chirag" {
		t.Fatalf("CurrentUser = %+v, %v", user, err)
	}

	// Tokens were encrypted, not stored as plaintext.
	stored := store.tokens[user.ID]
	if string(stored.AccessEnc) == "tok_abc" || len(stored.AccessEnc) == 0 {
		t.Fatalf("access token was not encrypted: %q", stored.AccessEnc)
	}
	if stored.RefreshEnc == nil || stored.ExpiresAt == nil {
		t.Fatal("expected a refresh token and expiry to be stored for an 8h grant")
	}

	// And UserGitHubToken decrypts it back correctly.
	got, err := svc.UserGitHubToken(context.Background(), user.ID)
	if err != nil || got != "tok_abc" {
		t.Fatalf("UserGitHubToken = %q, %v", got, err)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	tokenResp, userResp := defaultResponses()
	called := false
	svc, _ := newTestService(t, func(r *http.Request) (int, string) { called = true; return tokenResp(r) }, userResp)

	_, _, err := svc.Callback(context.Background(), "code", "attacker-state", "real-state")
	if !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("error = %v, want ErrStateMismatch", err)
	}
	if called {
		t.Fatal("the code must not be exchanged when state does not match")
	}
}

func TestCallbackRejectsEmptyExpectedState(t *testing.T) {
	// Guards against a missing/expired state cookie being silently treated
	// as "no check needed".
	svc, _ := newTestService(t, nil, nil)
	_, _, err := svc.Callback(context.Background(), "code", "", "")
	if !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("error = %v, want ErrStateMismatch", err)
	}
}

func TestCallbackWithoutRefreshToken(t *testing.T) {
	tokenResp := func(*http.Request) (int, string) { return 200, `{"access_token":"tok_abc"}` }
	_, userResp := defaultResponses()
	svc, store := newTestService(t, tokenResp, userResp)

	token, _, err := svc.Callback(context.Background(), "code", "s", "s")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := svc.CurrentUser(context.Background(), token)
	stored := store.tokens[user.ID]
	if stored.RefreshEnc != nil || stored.ExpiresAt != nil {
		t.Fatalf("a non-expiring grant should store no refresh token or expiry: %+v", stored)
	}
}

func TestSecondLoginUpdatesTheSameUser(t *testing.T) {
	tokenResp, _ := defaultResponses()
	renamed := func(*http.Request) (int, string) {
		return 200, `{"id":42,"login":"chirag-new","name":"Chirag New","email":"c@example.com","avatar_url":"https://x/b.png"}`
	}
	svc, _ := newTestService(t, tokenResp, renamed)

	token1, _, _ := svc.Callback(context.Background(), "code1", "s1", "s1")
	user1, _ := svc.CurrentUser(context.Background(), token1)

	token2, _, _ := svc.Callback(context.Background(), "code2", "s2", "s2")
	user2, _ := svc.CurrentUser(context.Background(), token2)

	if user1.ID != user2.ID {
		t.Fatalf("logging in again created a new user: %s vs %s", user1.ID, user2.ID)
	}
	if user2.GitHubLogin != "chirag-new" {
		t.Fatalf("profile did not refresh: %+v", user2)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	tokenResp, userResp := defaultResponses()
	svc, _ := newTestService(t, tokenResp, userResp)
	token, _, _ := svc.Callback(context.Background(), "code", "s", "s")

	if _, err := svc.CurrentUser(context.Background(), token); err != nil {
		t.Fatalf("expected a valid session before logout: %v", err)
	}
	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CurrentUser(context.Background(), token); err == nil {
		t.Fatal("expected the session to be gone after logout")
	}
}

func TestCurrentUserWithUnknownTokenFails(t *testing.T) {
	svc, _ := newTestService(t, nil, nil)
	if _, err := svc.CurrentUser(context.Background(), "nonexistent"); err == nil {
		t.Fatal("expected an error for an unknown session token")
	}
}

func TestUserGitHubTokenWithNoStoredTokenFails(t *testing.T) {
	svc, store := newTestService(t, nil, nil)
	store.tokens["1"] = Tokens{} // no AccessEnc
	if _, err := svc.UserGitHubToken(context.Background(), "1"); err == nil {
		t.Fatal("expected an error when no access token is stored")
	}
}
