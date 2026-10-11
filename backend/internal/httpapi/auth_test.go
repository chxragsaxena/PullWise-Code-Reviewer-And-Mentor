package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
)

// fakeAuth is a scriptable AuthService.
type fakeAuth struct {
	loginErr     error
	state        string
	callback     func(code, gotState, wantState string) (string, time.Time, error)
	usersByToken map[string]auth.User
	loggedOut    []string
}

func (f *fakeAuth) Login() (string, string, error) {
	if f.loginErr != nil {
		return "", "", f.loginErr
	}
	return "https://github.com/login/oauth/authorize?state=" + f.state, f.state, nil
}

func (f *fakeAuth) Callback(_ context.Context, code, gotState, wantState string) (string, time.Time, error) {
	return f.callback(code, gotState, wantState)
}

func (f *fakeAuth) CurrentUser(_ context.Context, token string) (auth.User, error) {
	u, ok := f.usersByToken[token]
	if !ok {
		return auth.User{}, errors.New("session not found")
	}
	return u, nil
}

func (f *fakeAuth) Logout(_ context.Context, token string) error {
	f.loggedOut = append(f.loggedOut, token)
	return nil
}

func newAuthAPI(fa *fakeAuth) http.Handler {
	return New(fakeService{}, WithCreateRateLimit(0, 0), WithAuth(fa, "http://localhost:5173", false)).Handler()
}

func TestAuthDisabledByDefault(t *testing.T) {
	h := newAPI(fakeService{}) // no WithAuth

	wantError(t, do(h, http.MethodGet, "/auth/github/login", "", ""), http.StatusServiceUnavailable, "auth_disabled")
	wantError(t, do(h, http.MethodGet, "/auth/github/callback", "", ""), http.StatusNotFound, "not_found")

	var got struct {
		User any `json:"user"`
	}
	decode(t, do(h, http.MethodGet, "/auth/me", "", ""), &got)
	if got.User != nil {
		t.Fatalf("user = %v, want nil when auth is disabled", got.User)
	}
}

func TestLoginRedirectsAndSetsStateCookie(t *testing.T) {
	h := newAuthAPI(&fakeAuth{state: "abc123"})
	req := httptest.NewRequest(http.MethodGet, "/auth/github/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc == "" {
		t.Fatal("missing Location header")
	}
	cookies := rec.Result().Cookies()
	var state *http.Cookie
	for _, c := range cookies {
		if c.Name == stateCookieName {
			state = c
		}
	}
	if state == nil || state.Value != "abc123" || !state.HttpOnly {
		t.Fatalf("state cookie = %+v", state)
	}
}

func TestCallbackHappyPathSetsSessionCookieAndRedirects(t *testing.T) {
	fa := &fakeAuth{
		callback: func(code, gotState, wantState string) (string, time.Time, error) {
			if code != "the-code" || gotState != "s1" || wantState != "s1" {
				t.Fatalf("callback got code=%q gotState=%q wantState=%q", code, gotState, wantState)
			}
			return "session-token-1", time.Now().Add(time.Hour), nil
		},
	}
	h := newAuthAPI(fa)

	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=the-code&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "http://localhost:5173" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	var session, clearedState *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case sessionCookieName:
			session = c
		case stateCookieName:
			clearedState = c
		}
	}
	if session == nil || session.Value != "session-token-1" || !session.HttpOnly {
		t.Fatalf("session cookie = %+v", session)
	}
	if clearedState == nil || clearedState.MaxAge >= 0 {
		t.Fatalf("state cookie should be cleared (MaxAge < 0): %+v", clearedState)
	}
}

func TestCallbackWithoutCodeIsBadRequest(t *testing.T) {
	h := newAuthAPI(&fakeAuth{})
	wantError(t, do(h, http.MethodGet, "/auth/github/callback?state=s", "", ""), http.StatusBadRequest, "invalid_request")
}

func TestCallbackWithGitHubErrorParam(t *testing.T) {
	h := newAuthAPI(&fakeAuth{})
	wantError(t, do(h, http.MethodGet, "/auth/github/callback?error=access_denied", "", ""), http.StatusBadRequest, "oauth_denied")
}

func TestCallbackStateMismatchIsBadRequest(t *testing.T) {
	fa := &fakeAuth{callback: func(string, string, string) (string, time.Time, error) {
		return "", time.Time{}, auth.ErrStateMismatch
	}}
	h := newAuthAPI(fa)
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=wrong", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "right"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	wantError(t, rec, http.StatusBadRequest, "invalid_state")
}

func TestMeReturnsTheLoggedInUser(t *testing.T) {
	fa := &fakeAuth{usersByToken: map[string]auth.User{
		"tok1": {ID: "1", GitHubLogin: "chirag", Name: "Chirag", AvatarURL: "https://x/a.png"},
	}}
	h := newAuthAPI(fa)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "tok1"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var got struct {
		User userDTO `json:"user"`
	}
	decode(t, rec, &got)
	if got.User.Login != "chirag" || got.User.ID != "1" {
		t.Fatalf("user = %+v", got.User)
	}
}

func TestMeWithNoCookieOrUnknownTokenReturnsNullUser(t *testing.T) {
	h := newAuthAPI(&fakeAuth{usersByToken: map[string]auth.User{}})

	var got struct {
		User any `json:"user"`
	}
	decode(t, do(h, http.MethodGet, "/auth/me", "", ""), &got)
	if got.User != nil {
		t.Fatalf("no cookie: user = %v, want nil", got.User)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "unknown"})
	rec := httptest.NewRecorder()
	newAuthAPI(&fakeAuth{usersByToken: map[string]auth.User{}}).ServeHTTP(rec, req)
	decode(t, rec, &got)
	if got.User != nil {
		t.Fatalf("unknown token: user = %v, want nil", got.User)
	}
}

func TestLogoutClearsCookieAndEndsSession(t *testing.T) {
	fa := &fakeAuth{}
	h := newAuthAPI(fa)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "tok1"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("session cookie should be cleared: %+v", cleared)
	}
	if len(fa.loggedOut) != 1 || fa.loggedOut[0] != "tok1" {
		t.Fatalf("loggedOut = %v, want [tok1]", fa.loggedOut)
	}
}

func TestLogoutWithNoCookieStillSucceeds(t *testing.T) {
	fa := &fakeAuth{}
	h := newAuthAPI(fa)
	rec := do(h, http.MethodPost, "/auth/logout", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(fa.loggedOut) != 0 {
		t.Fatalf("loggedOut = %v, want none called with no session", fa.loggedOut)
	}
}
