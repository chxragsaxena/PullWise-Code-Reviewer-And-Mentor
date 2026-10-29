package service

import (
	"context"
	"sync"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
)

// The creating user's GitHub token must reach every GitHub call the background worker makes.
func TestCreatorsGitHubTokenReachesTheBackgroundGitHubCalls(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantToken string
	}{
		{"signed in", "user-token", "user-token"},
		{"anonymous", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]string{} // call -> token it ran with
			record := func(call string, ctx context.Context) {
				mu.Lock()
				defer mu.Unlock()
				seen[call] = github.RequestToken(ctx)
			}

			store := newFakeStore()
			src := newFakeSource("main.go")
			basePR := src.pr
			src.pr = func(ctx context.Context) (github.PullRequest, error) {
				record("pull request", ctx)
				return basePR(ctx)
			}
			baseContent := src.content
			src.content = func(ctx context.Context, path string) ([]byte, error) {
				record("file content", ctx)
				return baseContent(ctx, path)
			}
			svc := newService(t, store, src, Config{})

			ctx := github.WithRequestToken(context.Background(), tt.token)
			rv, err := svc.Create(ctx, prURL)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			waitTerminal(t, store, rv.ID)

			mu.Lock()
			defer mu.Unlock()
			for _, call := range []string{"pull request", "file content"} {
				got, ran := seen[call]
				if !ran {
					t.Fatalf("%s was never fetched", call)
				}
				if got != tt.wantToken {
					t.Errorf("%s ran with token %q, want %q", call, got, tt.wantToken)
				}
			}
		})
	}
}
