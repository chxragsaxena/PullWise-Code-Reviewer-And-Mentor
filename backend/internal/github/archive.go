package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
)

const (
	maxArchiveBytes   = 100 << 20 // compressed download
	maxRepoFiles      = 400
	maxRepoFileBytes  = 100_000
	maxRepoTotalBytes = 8 << 20
	archiveTimeout    = 2 * time.Minute
)

var skippedDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, "dist": true, "build": true,
	"third_party": true, "testdata": true, "__pycache__": true, ".venv": true, "venv": true,
}

// SourceFile is one source file of a repository at a commit.
type SourceFile struct {
	Path    string
	Content string
}

func (c *Client) RepositoryFiles(ctx context.Context, ref PullRequestRef, sha string) ([]SourceFile, error) {
	// The archive can take longer than an API call, so use a longer timeout.
	slow := *c
	slow.http = &http.Client{Timeout: archiveTimeout, Transport: c.http.Transport}

	apiPath := fmt.Sprintf("/repos/%s/%s/tarball/%s", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), url.PathEscape(sha))
	resp, err := slow.get(ctx, apiPath, nil, "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("download archive of %s/%s at %s: %w", ref.Owner, ref.Repo, shortSHA(sha), err)
	}
	defer resp.Body.Close()

	files, err := readArchive(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return nil, fmt.Errorf("read archive of %s/%s: %w", ref.Owner, ref.Repo, err)
	}
	return files, nil
}

func readArchive(r io.Reader) ([]SourceFile, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var files []SourceFile
	total := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Every entry sits under one top-level folder named for the repo and commit.
		_, name, ok := strings.Cut(hdr.Name, "/")
		if !ok || !wanted(name, hdr.Size) {
			continue
		}
		if len(files) >= maxRepoFiles || total+int(hdr.Size) > maxRepoTotalBytes {
			break // out of budget: keep what we have
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxRepoFileBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxRepoFileBytes || bytes.IndexByte(data, 0) >= 0 {
			continue // too large after all, or binary
		}
		files = append(files, SourceFile{Path: name, Content: string(data)})
		total += len(data)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// wanted reports whether a file at name (relative to the repo root) is worth indexing.
func wanted(name string, size int64) bool {
	if size == 0 || size > maxRepoFileBytes || lang.Detect(name) == "" {
		return false
	}
	for _, dir := range strings.Split(path.Dir(name), "/") {
		if skippedDirs[dir] {
			return false
		}
	}
	base := path.Base(name)
	switch {
	case strings.HasSuffix(base, ".min.js"), strings.HasSuffix(base, ".pb.go"),
		strings.HasSuffix(base, "_generated.go"), strings.HasSuffix(base, ".d.ts"),
		strings.HasPrefix(base, "zz_generated"):
		return false
	}
	return true
}
