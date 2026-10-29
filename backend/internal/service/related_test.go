package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/knowledge"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

type searchCall struct {
	tenant, repo, query string
	topK                int
	exclude             []string
}

// fakeKnowledge is a scriptable Knowledge that records how it was used.
type fakeKnowledge struct {
	mu sync.Mutex

	indexed    map[string]string // tenant|repo -> ref already indexed
	results    []knowledge.Snippet
	refErr     error
	indexErr   error
	searchErr  error
	indexCalls []string // tenant|repo|ref|files
	searches   []searchCall
	requestIDs []string
}

func (f *fakeKnowledge) IndexedRef(ctx context.Context, tenant, repo string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refErr != nil {
		return "", false, f.refErr
	}
	ref, ok := f.indexed[tenant+"|"+repo]
	return ref, ok, nil
}

func (f *fakeKnowledge) Index(ctx context.Context, tenant, repo, ref string, files []knowledge.File) (knowledge.IndexResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.indexCalls = append(f.indexCalls, tenant+"|"+repo+"|"+ref+"|"+string(rune('0'+len(files))))
	if f.indexErr != nil {
		return knowledge.IndexResult{}, f.indexErr
	}
	if f.indexed == nil {
		f.indexed = map[string]string{}
	}
	f.indexed[tenant+"|"+repo] = ref
	return knowledge.IndexResult{Status: "indexed", Files: len(files)}, nil
}

func (f *fakeKnowledge) Search(ctx context.Context, tenant, repo, query string, topK int, exclude []string) ([]knowledge.Snippet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches = append(f.searches, searchCall{tenant, repo, query, topK, exclude})
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.results, nil
}

func relatedHarness(t *testing.T, kn *fakeKnowledge, withAI bool) (*Service, *fakeStore, *fakeSource, *fakeAI) {
	t.Helper()
	store := newFakeStore()
	src := newFakeSource("worker.go", "README.md")
	src.files[0].Patch = "+go func() { for { work() } }()"
	src.pr = func(context.Context) (github.PullRequest, error) {
		return github.PullRequest{Title: "x", HeadSHA: "headsha", BaseSHA: "basesha"}, nil
	}
	src.repoFiles = []github.SourceFile{{Path: "util/retry.go", Content: "package util"}, {Path: "main.go", Content: "package main"}}
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, nil }}

	cfg := Config{Skills: testSkills(t), AIProvider: nil, Knowledge: kn}
	if withAI {
		cfg.AIProvider = provider
	}
	if kn == nil { // keep the interface nil, not a typed nil
		cfg.Knowledge = nil
	}
	svc := New(store, store, src, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	return svc, store, src, provider
}

func runReview(t *testing.T, svc *Service, store *fakeStore, ctx context.Context) review.Review {
	t.Helper()
	rv, err := svc.Create(ctx, prURL)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("review ended %s (%+v), want completed", got.Status, got.Error)
	}
	return got
}

func TestRelatedCodeReachesTheAIPassAndExcludesTheChangedFiles(t *testing.T) {
	kn := &fakeKnowledge{results: []knowledge.Snippet{
		{Path: "util/retry.go", StartLine: 3, EndLine: 12, Content: "func Retry() {}", Score: 0.8},
	}}
	svc, store, src, provider := relatedHarness(t, kn, true)

	runReview(t, svc, store, context.Background())

	if len(provider.calls) == 0 {
		t.Fatal("the AI pass did not run")
	}
	ctxSnippets := provider.calls[0].Context
	if len(ctxSnippets) != 1 || ctxSnippets[0].Path != "util/retry.go" || ctxSnippets[0].StartLine != 3 {
		t.Fatalf("AI saw context %+v, want the retrieved snippet", ctxSnippets)
	}

	// Indexed once, at the pull request's BASE commit, for the anonymous tenant.
	if len(kn.indexCalls) != 1 || !strings.HasPrefix(kn.indexCalls[0], "public|o/r|basesha|") {
		t.Fatalf("index calls = %v, want one at the base commit for tenant public", kn.indexCalls)
	}
	if len(src.archiveShas) != 1 || src.archiveShas[0] != "basesha" {
		t.Fatalf("archive downloaded at %v, want the base commit", src.archiveShas)
	}

	if len(kn.searches) == 0 {
		t.Fatal("no search was made")
	}
	s := kn.searches[0]
	if s.tenant != "public" || s.repo != "o/r" || !strings.Contains(s.query, "go func()") {
		t.Errorf("search = %+v, want a query built from the diff", s)
	}
	if len(s.exclude) == 0 || s.exclude[0] != "worker.go" {
		t.Errorf("excluded paths = %v; the changed files are already shown in full", s.exclude)
	}
}

func TestASignedInUsersReviewUsesTheirOwnIndex(t *testing.T) {
	kn := &fakeKnowledge{}
	svc, store, _, _ := relatedHarness(t, kn, true)

	runReview(t, svc, store, review.WithViewer(context.Background(), "42"))

	if len(kn.indexCalls) != 1 || !strings.HasPrefix(kn.indexCalls[0], "u42|o/r|") {
		t.Fatalf("index calls = %v, want tenant u42", kn.indexCalls)
	}
	for _, s := range kn.searches {
		if s.tenant != "u42" {
			t.Errorf("search used tenant %q, want u42", s.tenant)
		}
	}
}

func TestAnAlreadyIndexedCommitIsNotDownloadedOrSentAgain(t *testing.T) {
	kn := &fakeKnowledge{indexed: map[string]string{"public|o/r": "basesha"}}
	svc, store, src, provider := relatedHarness(t, kn, true)

	runReview(t, svc, store, context.Background())

	if len(src.archiveShas) != 0 || len(kn.indexCalls) != 0 {
		t.Fatalf("downloaded %v and indexed %v although the commit was already indexed", src.archiveShas, kn.indexCalls)
	}
	if len(kn.searches) == 0 || len(provider.calls) == 0 {
		t.Fatal("it should still search the existing index")
	}
}

func TestAnythingGoingWrongWithTheKnowledgeBaseNeverFailsTheReview(t *testing.T) {
	boom := errors.New("knowledge service is down")
	tests := []struct {
		name  string
		kn    *fakeKnowledge
		setup func(*fakeSource)
	}{
		{"status check fails", &fakeKnowledge{refErr: boom}, nil},
		{"indexing fails", &fakeKnowledge{indexErr: boom}, nil},
		{"search fails", &fakeKnowledge{searchErr: boom}, nil},
		{"downloading the repository fails", &fakeKnowledge{}, func(s *fakeSource) { s.repoErr = github.ErrRateLimited }},
		{"the repository has no source files", &fakeKnowledge{}, func(s *fakeSource) { s.repoFiles = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, src, provider := relatedHarness(t, tt.kn, true)
			if tt.setup != nil {
				tt.setup(src)
			}

			runReview(t, svc, store, context.Background())

			if len(provider.calls) == 0 {
				t.Fatal("the AI pass should still run without context")
			}
			if n := len(provider.calls[0].Context); n != 0 {
				t.Fatalf("AI got %d context snippets after a failure, want none", n)
			}
		})
	}
}

func TestKnowledgeIsNotUsedWithoutAnAIPassOrWhenNotConfigured(t *testing.T) {
	kn := &fakeKnowledge{}
	svc, store, src, _ := relatedHarness(t, kn, false) // no AI provider
	runReview(t, svc, store, context.Background())
	if len(kn.searches)+len(kn.indexCalls)+len(src.archiveShas) != 0 {
		t.Fatal("the knowledge base was used although nothing would read its results")
	}

	svc, store, src, provider := relatedHarness(t, nil, true) // no knowledge base
	runReview(t, svc, store, context.Background())
	if len(src.archiveShas) != 0 {
		t.Fatal("the repository was downloaded although no knowledge base is configured")
	}
	if len(provider.calls) == 0 || len(provider.calls[0].Context) != 0 {
		t.Fatal("the AI pass should run with no context")
	}
}

func TestPickSnippetsKeepsTheBestDistinctOnesUpToTheCap(t *testing.T) {
	var found []knowledge.Snippet
	for i, p := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		found = append(found, knowledge.Snippet{Path: p + ".go", StartLine: 1, Score: float64(i) / 10})
	}
	// The same chunk found by a second query, with a lower score.
	found = append(found, knowledge.Snippet{Path: "h.go", StartLine: 1, Score: 0.01})

	got := pickSnippets(found)

	if len(got) != maxSnippets {
		t.Fatalf("got %d snippets, want the cap of %d", len(got), maxSnippets)
	}
	if got[0].Path != "h.go" || got[1].Path != "g.go" {
		t.Fatalf("order = %v, want best score first", got)
	}
	seen := map[string]bool{}
	for _, s := range got {
		if seen[s.Path] {
			t.Fatalf("%s appears twice", s.Path)
		}
		seen[s.Path] = true
	}
}

func TestTenantFor(t *testing.T) {
	if tenantFor("") != "public" || tenantFor("42") != "u42" {
		t.Fatalf("tenantFor = %q, %q", tenantFor(""), tenantFor("42"))
	}
}
