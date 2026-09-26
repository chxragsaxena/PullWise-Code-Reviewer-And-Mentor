package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public GitHub REST API.
	DefaultBaseURL = "https://api.github.com"

	filesPerPage = 100
	// maxFiles is GitHub's own limit on files listed for one pull request.
	maxFiles = 3000
	// maxContentBytes caps a single source file we are willing to download.
	maxContentBytes = 2 << 20
)

var (
	// ErrNotFound means the pull request or file does not exist, or is private.
	ErrNotFound = errors.New("github: not found")
	// ErrRateLimited means GitHub's API limit was hit.
	ErrRateLimited = errors.New("github: rate limit exceeded")
	// ErrTooLarge means a file exceeds the size we download.
	ErrTooLarge = errors.New("github: file too large")
)

// APIError is a non-success response from GitHub.
type APIError struct {
	Status      int
	Message     string
	RateLimited bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: %d %s", e.Status, e.Message)
}

// Is lets callers test with errors.Is(err, ErrNotFound) or ErrRateLimited.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrRateLimited:
		return e.RateLimited
	}
	return false
}

// PullRequest is the metadata of a pull request.
type PullRequest struct {
	Ref          PullRequestRef
	Title        string
	Author       string
	URL          string
	State        string
	HeadSHA      string
	FilesChanged int
	Additions    int
	Deletions    int
}

// ChangedFile is one file in a pull request. Patch is empty for binary files and
// for diffs GitHub considers too large.
type ChangedFile struct {
	Path      string
	Status    string
	Additions int
	Deletions int
	Patch     string
}

// Client reads pull requests from the GitHub REST API. It is safe for concurrent use.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithToken authenticates requests, raising the API rate limit.
func WithToken(token string) Option { return func(c *Client) { c.token = token } }

// WithBaseURL points the client at another API host (used by tests).
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient returns a Client for the public GitHub API unless overridden.
func NewClient(opts ...Option) *Client {
	c := &Client{baseURL: DefaultBaseURL, http: &http.Client{Timeout: 30 * time.Second}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchPullRequest returns the metadata of a pull request.
func (c *Client) FetchPullRequest(ctx context.Context, ref PullRequestRef) (PullRequest, error) {
	var body struct {
		Title        string `json:"title"`
		HTMLURL      string `json:"html_url"`
		State        string `json:"state"`
		ChangedFiles int    `json:"changed_files"`
		Additions    int    `json:"additions"`
		Deletions    int    `json:"deletions"`
		User         struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := c.getJSON(ctx, pullPath(ref), nil, &body); err != nil {
		return PullRequest{}, fmt.Errorf("fetch pull request %s: %w", ref, err)
	}

	return PullRequest{
		Ref:          ref,
		Title:        body.Title,
		Author:       body.User.Login,
		URL:          body.HTMLURL,
		State:        body.State,
		HeadSHA:      body.Head.SHA,
		FilesChanged: body.ChangedFiles,
		Additions:    body.Additions,
		Deletions:    body.Deletions,
	}, nil
}

// ListFiles returns every file changed by the pull request, following pagination
// up to GitHub's limit of 3000 files.
func (c *Client) ListFiles(ctx context.Context, ref PullRequestRef) ([]ChangedFile, error) {
	var files []ChangedFile
	for page := 1; len(files) < maxFiles; page++ {
		query := url.Values{"per_page": {strconv.Itoa(filesPerPage)}, "page": {strconv.Itoa(page)}}

		var batch []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
			Patch     string `json:"patch"`
		}
		if err := c.getJSON(ctx, pullPath(ref)+"/files", query, &batch); err != nil {
			return nil, fmt.Errorf("list files of %s: %w", ref, err)
		}

		for _, f := range batch {
			files = append(files, ChangedFile{
				Path: f.Filename, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions, Patch: f.Patch,
			})
		}
		if len(batch) < filesPerPage {
			break
		}
	}
	return files, nil
}

// FileContent returns a file's full contents at a given commit.
func (c *Client) FileContent(ctx context.Context, ref PullRequestRef, path, sha string) ([]byte, error) {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), strings.Join(segments, "/"))

	resp, err := c.get(ctx, apiPath, url.Values{"ref": {sha}}, "application/vnd.github.raw+json")
	if err != nil {
		return nil, fmt.Errorf("fetch %s at %s: %w", path, shortSHA(sha), err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxContentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxContentBytes {
		return nil, fmt.Errorf("fetch %s: %w", path, ErrTooLarge)
	}
	return data, nil
}

func pullPath(ref PullRequestRef) string {
	return fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), ref.Number)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.get(ctx, path, query, "application/vnd.github+json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// get performs a GET and returns the response only for 2xx statuses.
// The caller closes the body.
func (c *Client) get(ctx context.Context, path string, query url.Values, accept string) (*http.Response, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "pullwise")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request github: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, newAPIError(resp)
	}
	return resp, nil
}

func newAPIError(resp *http.Response) *APIError {
	var body struct {
		Message string `json:"message"`
	}
	// The body is best-effort detail; the status code is what matters.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)

	limited := resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden &&
			(resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(body.Message), "rate limit")))

	return &APIError{Status: resp.StatusCode, Message: body.Message, RateLimited: limited}
}
