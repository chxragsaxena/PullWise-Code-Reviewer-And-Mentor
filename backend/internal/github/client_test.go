package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testRef = PullRequestRef{Owner: "o", Repo: "r", Number: 7}

func newTestClient(t *testing.T, handler http.Handler, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(append([]Option{WithBaseURL(srv.URL)}, opts...)...)
}

func TestFetchPullRequest(t *testing.T) {
	var gotAuth, gotAccept, gotVersion, gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAccept, gotVersion, gotPath = r.Header.Get("Authorization"), r.Header.Get("Accept"), r.Header.Get("X-GitHub-Api-Version"), r.URL.Path
		fmt.Fprint(w, `{"title":"Add caching","html_url":"https://github.com/o/r/pull/7","state":"open",
			"changed_files":3,"additions":40,"deletions":5,"user":{"login":"chirag"},"head":{"sha":"abc123def456"}}`)
	}), WithToken("secret"))

	pr, err := client.FetchPullRequest(context.Background(), testRef)
	if err != nil {
		t.Fatalf("FetchPullRequest: %v", err)
	}

	want := PullRequest{
		Ref: testRef, Title: "Add caching", Author: "chirag", URL: "https://github.com/o/r/pull/7",
		State: "open", HeadSHA: "abc123def456", FilesChanged: 3, Additions: 40, Deletions: 5,
	}
	if pr != want {
		t.Fatalf("got %+v, want %+v", pr, want)
	}
	if gotPath != "/repos/o/r/pulls/7" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want the token sent as a bearer", gotAuth)
	}
	if gotAccept != "application/vnd.github+json" || gotVersion == "" {
		t.Errorf("Accept = %q, version = %q", gotAccept, gotVersion)
	}
}

func TestNoTokenSendsNoAuthorizationHeader(t *testing.T) {
	var gotAuth string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{}`)
	}))
	if _, err := client.FetchPullRequest(context.Background(), testRef); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want none", gotAuth)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		headers      map[string]string
		body         string
		wantNotFound bool
		wantLimited  bool
	}{
		{"not found", 404, nil, `{"message":"Not Found"}`, true, false},
		{"rate limited by header", 403, map[string]string{"X-RateLimit-Remaining": "0"}, `{"message":"forbidden"}`, false, true},
		{"rate limited by message", 403, nil, `{"message":"API rate limit exceeded for 1.2.3.4"}`, false, true},
		{"too many requests", 429, nil, `{"message":"slow down"}`, false, true},
		{"plain forbidden", 403, nil, `{"message":"Resource not accessible"}`, false, false},
		{"server error, non-JSON body", 500, nil, `<html>oops</html>`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))

			_, err := client.FetchPullRequest(context.Background(), testRef)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tt.status {
				t.Fatalf("error = %v, want *APIError with status %d", err, tt.status)
			}
			if got := errors.Is(err, ErrNotFound); got != tt.wantNotFound {
				t.Errorf("Is(ErrNotFound) = %v, want %v", got, tt.wantNotFound)
			}
			if got := errors.Is(err, ErrRateLimited); got != tt.wantLimited {
				t.Errorf("Is(ErrRateLimited) = %v, want %v", got, tt.wantLimited)
			}
		})
	}
}

func TestListFilesFollowsPagination(t *testing.T) {
	// 100 files on page 1, 5 on page 2: the client must fetch both and stop.
	var pages []string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		n := 100
		if page == "2" {
			n = 5
		}
		var items []string
		for i := 0; i < n; i++ {
			items = append(items, fmt.Sprintf(`{"filename":"p%s/f%d.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@"}`, page, i))
		}
		fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	}))

	files, err := client.ListFiles(context.Background(), testRef)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 105 {
		t.Fatalf("got %d files, want 105", len(files))
	}
	if strings.Join(pages, ",") != "1,2" {
		t.Fatalf("pages requested = %v, want 1 then 2", pages)
	}
	if files[0].Path != "p1/f0.go" || files[0].Patch == "" || files[0].Status != "modified" {
		t.Fatalf("first file = %+v", files[0])
	}
}

func TestListFilesEmptyPullRequest(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `[]`) }))
	files, err := client.ListFiles(context.Background(), testRef)
	if err != nil || len(files) != 0 {
		t.Fatalf("got %v, %v; want no files and no error", files, err)
	}
}

func TestFileContent(t *testing.T) {
	var gotPath, gotRef, gotAccept string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRef, gotAccept = r.URL.EscapedPath(), r.URL.Query().Get("ref"), r.Header.Get("Accept")
		fmt.Fprint(w, "package main\n")
	}))

	data, err := client.FileContent(context.Background(), testRef, "internal/my pkg/main.go", "deadbeefcafe")
	if err != nil {
		t.Fatalf("FileContent: %v", err)
	}
	if string(data) != "package main\n" {
		t.Errorf("data = %q", data)
	}
	if gotPath != "/repos/o/r/contents/internal/my%20pkg/main.go" {
		t.Errorf("path = %q, want each segment escaped but slashes kept", gotPath)
	}
	if gotRef != "deadbeefcafe" {
		t.Errorf("ref = %q", gotRef)
	}
	if gotAccept != "application/vnd.github.raw+json" {
		t.Errorf("Accept = %q, want the raw media type", gotAccept)
	}
}

func TestFileContentTooLarge(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(maxContentBytes+10))
		_, _ = w.Write([]byte(strings.Repeat("x", maxContentBytes+10)))
	}))
	_, err := client.FileContent(context.Background(), testRef, "big.go", "abc")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error = %v, want ErrTooLarge", err)
	}
}

func TestContextCancellationStopsTheRequest(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hold the request open until the client gives up
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.FetchPullRequest(ctx, testRef)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}
