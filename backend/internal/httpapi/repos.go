package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// maxRecentRepos is how many suggestions the endpoint returns.
const maxRecentRepos = 20

// RepoHistory remembers the repositories each signed-in user has used.
type RepoHistory interface {
	RememberRepo(ctx context.Context, userID, repo string) error
	RecentRepos(ctx context.Context, userID string, limit int) ([]string, error)
}

// WithRepoHistory enables suggestions of repositories a user has used before.
func WithRepoHistory(h RepoHistory) Option { return func(a *API) { a.repos = h } }

// listRecentRepos returns the signed-in user's repositories; anonymous visitors have none.
func (a *API) listRecentRepos(w http.ResponseWriter, r *http.Request) {
	repos := []string{}
	if _, user := a.viewerContext(r); user != nil {
		got, err := a.repos.RecentRepos(r.Context(), user.ID, maxRecentRepos)
		if err != nil {
			slog.Error("recent repos request failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong.")
			return
		}
		repos = append(repos, got...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"repos": repos})
}

// rememberRepo notes that the signed-in user used a repository. Suggestions are a convenience, so a
// failure is logged and never fails the request.
func (a *API) rememberRepo(ctx context.Context, repo string) {
	userID := review.ViewerFrom(ctx)
	if a.repos == nil || userID == "" || repo == "" {
		return
	}
	if err := a.repos.RememberRepo(ctx, userID, strings.ToLower(repo)); err != nil {
		slog.Warn("could not remember a repository for suggestions", "user", userID, "error", err)
	}
}
