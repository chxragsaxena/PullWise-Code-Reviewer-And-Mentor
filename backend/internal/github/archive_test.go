package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// tarball builds a .tar.gz the way GitHub does: everything under one top folder.
func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// GitHub's archives start with the top folder itself and a pax header entry.
	_ = tw.WriteHeader(&tar.Header{Name: "owner-repo-abc123/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "owner-repo-abc123/" + name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func archiveClient(t *testing.T, archive []byte, gotAuth, gotPath *string) *Client {
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		w.Write(archive)
	}), WithToken("tok"))
}

func TestRepositoryFilesKeepsOnlyProjectSourceAndStripsTheTopFolder(t *testing.T) {
	archive := tarball(t, map[string]string{
		"main.go":                      "package main",
		"internal/util/retry.go":       "package util",
		"web/App.tsx":                  "export const A = 1",
		"scripts/tool.py":              "print(1)",
		"README.md":                    "# docs",
		"config.json":                  "{}",
		"node_modules/lib/index.js":    "x",
		"vendor/dep/dep.go":            "package dep",
		"dist/bundle.js":               "x",
		"api/api.pb.go":                "package api",
		"static/app.min.js":            "x",
		"types/global.d.ts":            "declare const x: 1",
		"internal/testdata/sample.go":  "package x",
		"empty.go":                     "",
		"blob.go":                      "package x\x00binary",
		".git/hooks/pre-commit.py":     "x",
		"deep/nested/dir/handler.java": "class A {}",
	})
	var gotAuth, gotPath string
	c := archiveClient(t, archive, &gotAuth, &gotPath)

	files, err := c.RepositoryFiles(context.Background(), PullRequestRef{Owner: "owner", Repo: "repo"}, "abc123")
	if err != nil {
		t.Fatalf("RepositoryFiles: %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	want := []string{"deep/nested/dir/handler.java", "internal/util/retry.go", "main.go", "scripts/tool.py", "web/App.tsx"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v\nwant    %v", got, want)
	}
	if files[2].Content != "package main" {
		t.Errorf("content of main.go = %q", files[2].Content)
	}
	if gotPath != "/repos/owner/repo/tarball/abc123" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the token sent", gotAuth)
	}
}

func TestRepositoryFilesHonoursTheFileCountLimit(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < maxRepoFiles+50; i++ {
		files[fmt.Sprintf("pkg/f%04d.go", i)] = "package pkg"
	}
	c := archiveClient(t, tarball(t, files), nil, nil)

	got, err := c.RepositoryFiles(context.Background(), PullRequestRef{Owner: "o", Repo: "r"}, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxRepoFiles {
		t.Fatalf("got %d files, want the cap of %d", len(got), maxRepoFiles)
	}
}

func TestRepositoryFilesSkipsOversizedFilesButKeepsTheRest(t *testing.T) {
	c := archiveClient(t, tarball(t, map[string]string{
		"big.go":   "package x // " + strings.Repeat("a", maxRepoFileBytes),
		"small.go": "package x",
	}), nil, nil)

	got, err := c.RepositoryFiles(context.Background(), PullRequestRef{Owner: "o", Repo: "r"}, "abc123")
	if err != nil || len(got) != 1 || got[0].Path != "small.go" {
		t.Fatalf("got %+v, %v; want only small.go", got, err)
	}
}

func TestRepositoryFilesMapsGitHubErrors(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	_, err := c.RepositoryFiles(context.Background(), PullRequestRef{Owner: "o", Repo: "r"}, "abc123")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryFilesRejectsANonArchive(t *testing.T) {
	c := archiveClient(t, []byte("this is not a gzip file"), nil, nil)
	if _, err := c.RepositoryFiles(context.Background(), PullRequestRef{Owner: "o", Repo: "r"}, "abc123"); err == nil {
		t.Fatal("expected an error for a response that is not an archive")
	}
}

func TestDefaultBranchSHAReadsABareCommitID(t *testing.T) {
	const sha = "9f8adb1aabaa8a91180a821111d3f8e668d1c0bf"
	var gotPath, gotAccept string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept = r.URL.Path, r.Header.Get("Accept")
		fmt.Fprint(w, sha+"\n")
	}))

	got, err := c.DefaultBranchSHA(context.Background(), PullRequestRef{Owner: "o", Repo: "r"})
	if err != nil || got != sha {
		t.Fatalf("got %q, %v", got, err)
	}
	if gotPath != "/repos/o/r/commits/HEAD" || gotAccept != "application/vnd.github.sha" {
		t.Errorf("path = %q, accept = %q", gotPath, gotAccept)
	}
}

func TestDefaultBranchSHAErrors(t *testing.T) {
	missing := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	if _, err := missing.DefaultBranchSHA(context.Background(), testRef); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing repo: %v, want ErrNotFound", err)
	}

	odd := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>not a sha</html>")
	}))
	if _, err := odd.DefaultBranchSHA(context.Background(), testRef); err == nil {
		t.Error("a response that is not a commit id must be an error")
	}
}

func TestFetchPullRequestReadsTheBaseCommit(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"title":"t","head":{"sha":"headsha"},"base":{"sha":"basesha"}}`)
	}))
	pr, err := c.FetchPullRequest(context.Background(), testRef)
	if err != nil || pr.HeadSHA != "headsha" || pr.BaseSHA != "basesha" {
		t.Fatalf("pr = %+v, %v", pr, err)
	}
}
