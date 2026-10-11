package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fakeGitHubOAuth(t *testing.T, tokenResp func(r *http.Request) (int, string), userResp func(r *http.Request) (int, string)) *OAuth {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var status int
		var body string
		switch r.URL.Path {
		case "/token":
			status, body = tokenResp(r)
		case "/user":
			status, body = userResp(r)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return NewOAuth("client-id", "client-secret", "http://localhost:8080/auth/github/callback",
		WithEndpoints(srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/user"))
}

func TestNewStateIsRandomAndURLSafe(t *testing.T) {
	a, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewState()
	if a == b {
		t.Fatal("two calls produced the same state")
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("state %q is not URL-safe (raw base64url should have no +, / or =)", a)
	}
}

func TestAuthorizeURL(t *testing.T) {
	o := NewOAuth("my-client-id", "secret", "http://localhost:8080/auth/github/callback",
		WithEndpoints("https://x/authorize", "https://x/token", "https://x/user"))
	got := o.AuthorizeURL("state123")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "my-client-id" || q.Get("state") != "state123" || q.Get("scope") != "repo" {
		t.Fatalf("query = %v", q)
	}
	if q.Get("redirect_uri") != "http://localhost:8080/auth/github/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
}

func TestExchangeSendsCorrectFormAndParsesToken(t *testing.T) {
	var gotForm url.Values
	o := fakeGitHubOAuth(t, func(r *http.Request) (int, string) {
		r.ParseForm()
		gotForm = r.PostForm
		return 200, `{"access_token":"tok_abc","refresh_token":"ref_xyz","expires_in":28800,"token_type":"bearer"}`
	}, nil)

	tok, err := o.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "tok_abc" || tok.RefreshToken != "ref_xyz" {
		t.Fatalf("token = %+v", tok)
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("expected a non-zero expiry for an 8h token")
	}
	if gotForm.Get("code") != "the-code" || gotForm.Get("client_id") != "client-id" ||
		gotForm.Get("client_secret") != "client-secret" || gotForm.Get("redirect_uri") != "http://localhost:8080/auth/github/callback" {
		t.Fatalf("form sent to GitHub = %v", gotForm)
	}
}

func TestExchangeWithoutExpiryLeavesExpiresAtZero(t *testing.T) {
	o := fakeGitHubOAuth(t, func(r *http.Request) (int, string) {
		return 200, `{"access_token":"tok_abc","token_type":"bearer"}`
	}, nil)
	tok, err := o.Exchange(context.Background(), "code")
	if err != nil || !tok.ExpiresAt.IsZero() {
		t.Fatalf("tok = %+v, err = %v; want a zero ExpiresAt for a non-expiring token", tok, err)
	}
}

func TestExchangeSurfacesGitHubError(t *testing.T) {
	o := fakeGitHubOAuth(t, func(r *http.Request) (int, string) {
		return 200, `{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`
	}, nil)
	_, err := o.Exchange(context.Background(), "stale-code")
	if err == nil || !strings.Contains(err.Error(), "bad_verification_code") {
		t.Fatalf("error = %v, want it to mention bad_verification_code", err)
	}
}

func TestFetchProfile(t *testing.T) {
	var gotAuth string
	o := fakeGitHubOAuth(t, nil, func(r *http.Request) (int, string) {
		gotAuth = r.Header.Get("Authorization")
		return 200, `{"id":12345,"login":"chxragsaxena","name":"Chirag","email":"c@example.com","avatar_url":"https://example.com/a.png"}`
	})
	p, err := o.FetchProfile(context.Background(), "tok_abc")
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if p.ID != 12345 || p.Login != "chxragsaxena" || p.Name != "Chirag" || p.Email != "c@example.com" {
		t.Fatalf("profile = %+v", p)
	}
	if gotAuth != "Bearer tok_abc" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestFetchProfileNonOKStatus(t *testing.T) {
	o := fakeGitHubOAuth(t, nil, func(r *http.Request) (int, string) { return 401, `{"message":"Bad credentials"}` })
	if _, err := o.FetchProfile(context.Background(), "bad-token"); err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}
