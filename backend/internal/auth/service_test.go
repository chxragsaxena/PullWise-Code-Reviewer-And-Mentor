package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
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

func TestUserGitHubTokenRefusesAnExpiredToken(t *testing.T) {
	svc, store := newTestService(t, nil, nil)
	enc, err := svc.crypt.Encrypt("tok_old")
	if err != nil {
		t.Fatal(err)
	}
	past, future := time.Now().Add(-time.Minute), time.Now().Add(time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		wantErr   error
	}{
		{"expired", &past, ErrTokenExpired},
		{"still valid", &future, nil},
		{"no expiry recorded (token never expires)", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.tokens["1"] = Tokens{AccessEnc: enc, ExpiresAt: tt.expiresAt}
			got, err := svc.UserGitHubToken(context.Background(), "1")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && got != "tok_old" {
				t.Fatalf("token = %q, want tok_old", got)
			}
			if tt.wantErr != nil && got != "" {
				t.Fatal("an expired token must not be returned")
			}
		})
	}
}

// refreshServer answers GitHub's token endpoint for refresh_token grants and
// counts how many times it was asked.
func refreshingService(t *testing.T, respond func(refreshToken string) (int, string)) (*Service, *fakeStore, *int32) {
	t.Helper()
	var calls int32
	tokenResp := func(r *http.Request) (int, string) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("client_secret") != "secret" {
			t.Errorf("unexpected token request: %v", r.PostForm)
		}
		atomic.AddInt32(&calls, 1)
		return respond(r.PostForm.Get("refresh_token"))
	}
	svc, store := newTestService(t, tokenResp, nil)
	return svc, store, &calls
}

func storeTokens(t *testing.T, svc *Service, store *fakeStore, access, refresh string, expiresAt time.Time) {
	t.Helper()
	a, err := svc.crypt.Encrypt(access)
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.crypt.Encrypt(refresh)
	if err != nil {
		t.Fatal(err)
	}
	store.tokens["1"] = Tokens{AccessEnc: a, RefreshEnc: r, ExpiresAt: &expiresAt}
}

func TestUserGitHubTokenRefreshesAnExpiredTokenAndSavesTheRotatedPair(t *testing.T) {
	svc, store, calls := refreshingService(t, func(refresh string) (int, string) {
		if refresh != "ghr_old" {
			t.Errorf("refreshed with %q, want the stored refresh token", refresh)
		}
		return 200, `{"access_token":"ghu_new","refresh_token":"ghr_new","expires_in":28800}`
	})
	storeTokens(t, svc, store, "ghu_old", "ghr_old", time.Now().Add(-time.Hour))

	got, err := svc.UserGitHubToken(context.Background(), "1")
	if err != nil || got != "ghu_new" {
		t.Fatalf("UserGitHubToken = %q, %v; want the refreshed token", got, err)
	}

	saved := store.tokens["1"]
	access, _ := svc.crypt.Decrypt(saved.AccessEnc)
	refresh, _ := svc.crypt.Decrypt(saved.RefreshEnc)
	if access != "ghu_new" || refresh != "ghr_new" {
		t.Fatalf("saved access=%q refresh=%q; the rotated refresh token must be kept", access, refresh)
	}
	if saved.ExpiresAt == nil || !saved.ExpiresAt.After(time.Now().Add(7*time.Hour)) {
		t.Fatalf("saved expiry = %v, want about 8h from now", saved.ExpiresAt)
	}

	// The next call uses the saved token without another refresh.
	if again, err := svc.UserGitHubToken(context.Background(), "1"); err != nil || again != "ghu_new" {
		t.Fatalf("second call = %q, %v", again, err)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("GitHub was asked to refresh %d times, want 1", n)
	}
}

func TestUserGitHubTokenRefreshesJustBeforeExpiry(t *testing.T) {
	svc, store, calls := refreshingService(t, func(string) (int, string) {
		return 200, `{"access_token":"ghu_new","refresh_token":"ghr_new","expires_in":28800}`
	})
	storeTokens(t, svc, store, "ghu_old", "ghr_old", time.Now().Add(20*time.Second)) // inside the skew

	if got, err := svc.UserGitHubToken(context.Background(), "1"); err != nil || got != "ghu_new" {
		t.Fatalf("UserGitHubToken = %q, %v", got, err)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatal("a token about to expire should be refreshed")
	}
}

func TestUserGitHubTokenReportsExpiredWhenGitHubRefusesTheRefresh(t *testing.T) {
	svc, store, _ := refreshingService(t, func(string) (int, string) {
		return 200, `{"error":"bad_refresh_token","error_description":"The refresh token passed is incorrect or expired."}`
	})
	storeTokens(t, svc, store, "ghu_old", "ghr_old", time.Now().Add(-time.Hour))

	got, err := svc.UserGitHubToken(context.Background(), "1")
	if !errors.Is(err, ErrTokenExpired) || got != "" {
		t.Fatalf("got %q, %v; want ErrTokenExpired and no token", got, err)
	}
	// The old tokens are left alone so nothing is lost by a failed attempt.
	if access, _ := svc.crypt.Decrypt(store.tokens["1"].AccessEnc); access != "ghu_old" {
		t.Fatal("a failed refresh must not overwrite the stored tokens")
	}
}

func TestConcurrentRequestsRefreshOnlyOnce(t *testing.T) {
	svc, store, calls := refreshingService(t, func(string) (int, string) {
		time.Sleep(50 * time.Millisecond) // widen the race window
		return 200, `{"access_token":"ghu_new","refresh_token":"ghr_new","expires_in":28800}`
	})
	storeTokens(t, svc, store, "ghu_old", "ghr_old", time.Now().Add(-time.Hour))

	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = svc.UserGitHubToken(context.Background(), "1")
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("GitHub was asked to refresh %d times, want 1 (refresh tokens are single-use)", n)
	}
	for i, got := range results {
		if got != "ghu_new" {
			t.Errorf("request %d got %q, want the refreshed token", i, got)
		}
	}
}

func TestUserGitHubTokenWithNoStoredTokenFails(t *testing.T) {
	svc, store := newTestService(t, nil, nil)
	store.tokens["1"] = Tokens{} // no AccessEnc
	if _, err := svc.UserGitHubToken(context.Background(), "1"); err == nil {
		t.Fatal("expected an error when no access token is stored")
	}
}
