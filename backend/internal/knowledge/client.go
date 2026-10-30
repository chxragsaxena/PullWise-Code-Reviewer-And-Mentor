// Package knowledge is the client for the separate pullwise-knowledge repo-context service.
package knowledge

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tokenTTL      = time.Minute
	indexTimeout  = 6 * time.Minute // a large repo may be paced by the embedding rate limit
	searchTimeout = 20 * time.Second
	minSecretLen  = 32
	maxResponse   = 8 << 20
)

// File is one source file to index.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// wireFile is a File as sent: base64, because web firewalls in front of the service misread plain
// source and docs (shell commands, HTML-like placeholders) as attacks and block the whole request.
type wireFile struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
}

func encodeFiles(files []File) []wireFile {
	out := make([]wireFile, len(files))
	for i, f := range files {
		out[i] = wireFile{Path: f.Path, ContentBase64: base64.StdEncoding.EncodeToString([]byte(f.Content))}
	}
	return out
}

// Snippet is a piece of related code the service found.
type Snippet struct {
	Path      string  `json:"path"`
	StartLine int     `json:"startLine"`
	EndLine   int     `json:"endLine"`
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
}

// IndexResult says what an index call did.
type IndexResult struct {
	Status string `json:"status"`
	Files  int    `json:"files"`
	Chunks int    `json:"chunks"`
}

// APIError is an error response from the service.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("knowledge service: %d %s: %s", e.Status, e.Code, e.Message)
}

// Client calls the knowledge service.
type Client struct {
	baseURL  string
	secret   []byte
	issuer   string
	audience string
	http     *http.Client
	now      func() time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithTLS sets the TLS configuration, e.g.
func WithTLS(cfg *tls.Config) Option {
	return func(c *Client) { c.http = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}} }
}

// WithHTTPClient replaces the HTTP client (used by tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New builds a Client for the service at baseURL.
func New(baseURL string, secret []byte, issuer, audience string, opts ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("knowledge URL %q is not a valid http(s) URL", baseURL)
	}
	if len(secret) < minSecretLen {
		return nil, fmt.Errorf("knowledge JWT secret must be at least %d bytes", minSecretLen)
	}
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"), secret: secret, issuer: issuer, audience: audience,
		http: &http.Client{}, now: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Tenant names whose index a call may touch: a signed-in user's own, or the shared one for anonymous callers.
func Tenant(viewerID string) string {
	if viewerID == "" {
		return "public"
	}
	return "u" + viewerID
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// Index replaces the tenant's index of a repository with these files at ref.
func (c *Client) Index(ctx context.Context, tenant, repo, ref string, files []File) (IndexResult, error) {
	ctx, cancel := context.WithTimeout(ctx, indexTimeout)
	defer cancel()
	var out IndexResult
	body := map[string]any{"repo": repo, "ref": ref, "files": encodeFiles(files)}
	err := c.do(ctx, http.MethodPost, "/v1/index", tenant, []string{"index"}, body, &out)
	return out, err
}

// Search finds the chunks closest to query; the service also uses the query's own words for keyword search.
func (c *Client) Search(ctx context.Context, tenant, repo, query string, topK int, excludePaths []string) ([]Snippet, error) {
	return c.search(ctx, tenant, repo, map[string]any{"query": query}, topK, excludePaths)
}

// SearchHybrid is Search with its own words for the keyword half; an empty keywords means vectors only.
func (c *Client) SearchHybrid(ctx context.Context, tenant, repo, query, keywords string, topK int, excludePaths []string) ([]Snippet, error) {
	return c.search(ctx, tenant, repo, map[string]any{"query": query, "keywords": keywords}, topK, excludePaths)
}

func (c *Client) search(ctx context.Context, tenant, repo string, body map[string]any, topK int, excludePaths []string) ([]Snippet, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	var out struct {
		Results []Snippet `json:"results"`
	}
	body["repo"], body["topK"], body["excludePaths"] = repo, topK, excludePaths
	if err := c.do(ctx, http.MethodPost, "/v1/search", tenant, []string{"search"}, body, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

func (c *Client) IndexedRef(ctx context.Context, tenant, repo string) (string, bool, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return "", false, fmt.Errorf("repo %q must look like owner/name", repo)
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	var out struct {
		Ref string `json:"ref"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), tenant, []string{"search"}, nil, &out)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return out.Ref, true, nil
}

func (c *Client) do(ctx context.Context, method, path, tenant string, scopes []string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	token, err := c.token(tenant, scopes)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if id, _ := ctx.Value(requestIDKey{}).(string); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call knowledge service: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		_ = json.Unmarshal(respBody, &e)
		return &APIError{Status: resp.StatusCode, Code: e.Error.Code, Message: e.Error.Message}
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) token(tenant string, scopes []string) (string, error) {
	if tenant == "" {
		return "", errors.New("a tenant is required")
	}
	now := c.now()
	claims := map[string]any{
		"iss": c.issuer, "aud": c.audience,
		"iat": now.Unix(), "exp": now.Add(tokenTTL).Unix(),
		"tenant": tenant, "scopes": scopes,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc.EncodeToString(payload)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(signingInput))
	return signingInput + "." + enc.EncodeToString(mac.Sum(nil)), nil
}
