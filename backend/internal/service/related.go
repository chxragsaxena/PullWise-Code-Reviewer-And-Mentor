package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/knowledge"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// Knowledge is the repository knowledge base: it indexes a repository and finds related code.
type Knowledge interface {
	IndexedRef(ctx context.Context, tenant, repo string) (string, bool, error)
	Index(ctx context.Context, tenant, repo, ref string, files []knowledge.File) (knowledge.IndexResult, error)
	Search(ctx context.Context, tenant, repo, query string, topK int, excludePaths []string) ([]knowledge.Snippet, error)
}

// RepositorySource is an optional PullRequestSource capability: read a repository at a commit.
type RepositorySource interface {
	RepositoryFiles(ctx context.Context, ref github.PullRequestRef, sha string) ([]github.SourceFile, error)
}

const (
	maxQueryFiles    = 5    // changed files used as search queries
	queryChars       = 1500 // how much of a file's diff is used as its query
	snippetsPerQuery = 3
	maxSnippets      = 6
)

// relatedCode finds code elsewhere in the repository related to the PR; it never fails the review.
func (s *Service) relatedCode(ctx context.Context, reviewID string, ref github.PullRequestRef, pr github.PullRequest, changed []review.ChangedFile) []ai.ContextSnippet {
	repoSource, ok := s.source.(RepositorySource)
	if s.knowledge == nil || s.aiProvider == nil || !ok {
		return nil
	}

	tenant := knowledge.Tenant(review.ViewerFrom(ctx))
	repo := strings.ToLower(ref.Owner + "/" + ref.Repo)
	sha := pr.BaseSHA
	if sha == "" {
		sha = pr.HeadSHA
	}
	ctx = knowledge.WithRequestID(ctx, "review-"+reviewID)

	snippets, err := s.findRelated(ctx, repoSource, tenant, repo, ref, sha, changed)
	if err != nil {
		slog.Warn("could not get repository context; reviewing without it", "review", reviewID, "repo", repo, "error", err)
		return nil
	}
	return snippets
}

func (s *Service) findRelated(ctx context.Context, repoSource RepositorySource, tenant, repo string, ref github.PullRequestRef, sha string, changed []review.ChangedFile) ([]ai.ContextSnippet, error) {
	// Index only if this commit isn't indexed yet, to skip the archive download and embeddings.
	if have, ok, err := s.knowledge.IndexedRef(ctx, tenant, repo); err != nil {
		return nil, err
	} else if !ok || have != sha {
		files, err := repoSource.RepositoryFiles(ctx, ref, sha)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, nil
		}
		send := make([]knowledge.File, len(files))
		for i, f := range files {
			send[i] = knowledge.File{Path: f.Path, Content: f.Content}
		}
		if _, err := s.knowledge.Index(ctx, tenant, repo, sha, send); err != nil {
			return nil, err
		}
	}

	exclude := make([]string, 0, len(changed))
	for _, f := range changed {
		exclude = append(exclude, f.Path)
	}

	var found []knowledge.Snippet
	queries := 0
	for _, f := range changed {
		if f.Patch == "" || queries >= maxQueryFiles {
			continue
		}
		queries++
		query := f.Patch
		if len(query) > queryChars {
			query = query[:queryChars]
		}
		results, err := s.knowledge.Search(ctx, tenant, repo, query, snippetsPerQuery, exclude)
		if err != nil {
			return nil, err
		}
		found = append(found, results...)
	}
	return pickSnippets(found), nil
}

// pickSnippets keeps the best distinct snippets across all the searches.
func pickSnippets(found []knowledge.Snippet) []ai.ContextSnippet {
	sort.SliceStable(found, func(i, j int) bool { return found[i].Score > found[j].Score })

	type key struct {
		path  string
		start int
	}
	seen := map[key]bool{}
	var out []ai.ContextSnippet
	for _, sn := range found {
		k := key{sn.Path, sn.StartLine}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, ai.ContextSnippet{Path: sn.Path, StartLine: sn.StartLine, EndLine: sn.EndLine, Content: sn.Content})
		if len(out) == maxSnippets {
			break
		}
	}
	return out
}
