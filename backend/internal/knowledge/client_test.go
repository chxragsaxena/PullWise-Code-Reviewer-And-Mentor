package knowledge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, secret, "pullwise", "pullwise-knowledge")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func decodeToken(t *testing.T, header string) map[string]any {
	t.Helper()
	raw, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		t.Fatalf("Authorization = %q, want a bearer token", header)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatal("the token's signature does not verify with the shared secret")
	}
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(hdr) != `{"alg":"HS256","typ":"JWT"}` {
		t.Fatalf("header = %s", hdr)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestIndexSendsASignedTokenForTheTenantAndTheFiles(t *testing.T) {
	var claims map[string]any
	var body struct {
		Repo, Ref string
		Files     []File
	}
	var requestID string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/index" || r.Method != http.MethodPost {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		claims = decodeToken(t, r.Header.Get("Authorization"))
		requestID = r.Header.Get("X-Request-ID")
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"status":"indexed","files":1,"chunks":2}`)
	})

	ctx := WithRequestID(context.Background(), "review-7")
	got, err := c.Index(ctx, "u42", "o/r", "abc", []File{{Path: "a.go", Content: "package a"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "indexed" || got.Chunks != 2 {
		t.Fatalf("result = %+v", got)
	}

	if claims["iss"] != "pullwise" || claims["aud"] != "pullwise-knowledge" || claims["tenant"] != "u42" {
		t.Errorf("claims = %v", claims)
	}
	scopes, _ := claims["scopes"].([]any)
	if len(scopes) != 1 || scopes[0] != "index" {
		t.Errorf("scopes = %v, want only index (least privilege)", scopes)
	}
	life := claims["exp"].(float64) - claims["iat"].(float64)
	if life <= 0 || life > 120 {
		t.Errorf("token lifetime = %v seconds, want about a minute", life)
	}
	if requestID != "review-7" {
		t.Errorf("X-Request-ID = %q", requestID)
	}
	if body.Repo != "o/r" || body.Ref != "abc" || len(body.Files) != 1 || body.Files[0].Path != "a.go" {
		t.Errorf("body = %+v", body)
	}
}

func TestSearchAsksOnlyForTheSearchScopeAndParsesResults(t *testing.T) {
	var claims map[string]any
	var body map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		claims = decodeToken(t, r.Header.Get("Authorization"))
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"results":[{"path":"retry.go","startLine":3,"endLine":9,"content":"func Retry()","score":0.8}]}`)
	})

	got, err := c.Search(context.Background(), "public", "o/r", "retry a call", 3, []string{"a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "retry.go" || got[0].StartLine != 3 || got[0].Score != 0.8 {
		t.Fatalf("results = %+v", got)
	}
	scopes, _ := claims["scopes"].([]any)
	if claims["tenant"] != "public" || len(scopes) != 1 || scopes[0] != "search" {
		t.Errorf("claims = %v", claims)
	}
	if body["query"] != "retry a call" || body["topK"] != float64(3) {
		t.Errorf("body = %v", body)
	}
	if _, hasTenant := body["tenant"]; hasTenant {
		t.Error("the tenant must travel only in the token, never in the body (the service refuses it)")
	}
}

func TestIndexedRefReportsTheRefOrThatThereIsNone(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/repos/acme/shop" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "" {
			t.Error("a GET should not claim to have a body")
		}
		scopes, _ := decodeToken(t, r.Header.Get("Authorization"))["scopes"].([]any)
		if len(scopes) != 1 || scopes[0] != "search" {
			t.Errorf("scopes = %v", scopes)
		}
		if decodeToken(t, r.Header.Get("Authorization"))["tenant"] == "nobody" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"code":"not_found","message":"not indexed"}}`)
			return
		}
		fmt.Fprint(w, `{"repo":"acme/shop","ref":"abc123"}`)
	})

	if ref, ok, err := c.IndexedRef(context.Background(), "u1", "acme/shop"); err != nil || !ok || ref != "abc123" {
		t.Fatalf("IndexedRef = %q, %v, %v", ref, ok, err)
	}
	if ref, ok, err := c.IndexedRef(context.Background(), "nobody", "acme/shop"); err != nil || ok || ref != "" {
		t.Fatalf("an unindexed repo = %q, %v, %v; want not-found to be a normal answer, not an error", ref, ok, err)
	}
	if _, _, err := c.IndexedRef(context.Background(), "u1", "no-slash"); err == nil {
		t.Fatal("a malformed repo name must be rejected")
	}
}

func TestErrorsFromTheServiceKeepTheirStatusAndCode(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"error":{"code":"embedding_unavailable","message":"provider down"}}`)
	})
	_, err := c.Search(context.Background(), "u1", "o/r", "q", 3, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 502 || apiErr.Code != "embedding_unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestAnUnreachableServiceIsAnError(t *testing.T) {
	c, _ := New("http://127.0.0.1:1", secret, "pullwise", "pullwise-knowledge")
	if _, err := c.Search(context.Background(), "u1", "o/r", "q", 3, nil); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	for name, tt := range map[string]struct {
		url    string
		secret []byte
	}{
		"empty url":    {"", secret},
		"no scheme":    {"localhost:8081", secret},
		"ftp scheme":   {"ftp://x", secret},
		"short secret": {"http://x", []byte("short")},
	} {
		if _, err := New(tt.url, tt.secret, "i", "a"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestTokenRequiresATenant(t *testing.T) {
	c, _ := New("http://x", secret, "i", "a")
	c.now = func() time.Time { return time.Unix(1_000_000, 0) }
	if _, err := c.token("", []string{"search"}); err == nil {
		t.Fatal("a token without a tenant must not be issued")
	}
}
