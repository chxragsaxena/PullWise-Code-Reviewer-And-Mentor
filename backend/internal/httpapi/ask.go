package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ask"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// maxAskBody bounds a question plus up to six earlier turns.
const maxAskBody = 64 << 10

// AskService answers questions about a repository.
type AskService interface {
	Ask(ctx context.Context, req ask.Request) (ask.Answer, error)
}

// WithAsk enables POST /api/ask.
func WithAsk(svc AskService) Option { return func(a *API) { a.ask = svc } }

// WithAskRateLimit limits questions per client address; perMinute <= 0 turns the limit off.
func WithAskRateLimit(perMinute, burst int) Option {
	return func(a *API) {
		if perMinute > 0 && burst > 0 {
			a.askLimit = newLimiter(perMinute, burst, time.Now)
		} else {
			a.askLimit = nil
		}
	}
}

type askRequest struct {
	Repo     string `json:"repo"`
	ReviewID string `json:"reviewId"`
	Question string `json:"question"`
	History  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"history"`
}

type sourceDTO struct {
	N         int    `json:"n"`
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet"`
}

type answerDTO struct {
	Answer     string      `json:"answer"`
	Repo       string      `json:"repo"`
	IndexedRef string      `json:"indexedRef"`
	Sources    []sourceDTO `json:"sources"`
}

func (a *API) askQuestion(w http.ResponseWriter, r *http.Request) {
	if a.askLimit != nil {
		if ok, wait := a.askLimit.allow(clientKey(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "You are asking too fast. Please wait a moment.")
			return
		}
	}

	var body askRequest
	if apiErr := decodeJSON(w, r, maxAskBody, &body); apiErr != nil {
		apiErr.write(w)
		return
	}
	req := ask.Request{Repo: body.Repo, ReviewID: body.ReviewID, Question: body.Question}
	for _, t := range body.History {
		req.History = append(req.History, ask.Turn{Role: t.Role, Content: t.Content})
	}

	got, err := a.ask.Ask(a.githubContext(r), req)
	if err != nil {
		writeAskError(w, r, err)
		return
	}

	sources := make([]sourceDTO, 0, len(got.Sources))
	for _, s := range got.Sources {
		sources = append(sources, sourceDTO{N: s.N, Path: s.Path, StartLine: s.StartLine, EndLine: s.EndLine, URL: s.URL, Snippet: s.Snippet})
	}
	writeJSON(w, http.StatusOK, answerDTO{Answer: got.Text, Repo: got.Repo, IndexedRef: got.IndexedRef, Sources: sources})
}

func writeAskError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ask.InvalidError
	var groqErr *ai.GroqAPIError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, ask.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ask_unavailable", "Asking questions is not set up on this server.")
	case errors.Is(err, ask.ErrRepoNotFound):
		writeError(w, http.StatusNotFound, "repo_not_found", "Repository not found. If it is private, sign in with GitHub first.")
	case errors.Is(err, ask.ErrNoSource):
		writeError(w, http.StatusUnprocessableEntity, "no_source", "That repository has no source files in a language we read.")
	case errors.Is(err, review.ErrReviewNotFound):
		writeError(w, http.StatusNotFound, "review_not_found", "That review does not exist.")
	case errors.Is(err, github.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "github_rate_limited", "GitHub's rate limit was reached. Please try again in a few minutes.")
	case errors.As(err, &groqErr) && groqErr.StatusCode == http.StatusTooManyRequests:
		writeError(w, http.StatusTooManyRequests, "ai_busy", "The AI is busy right now. Please try again in a minute.")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "timeout", "Preparing the repository took too long. Please try again.")
	default:
		slog.Error("ask failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusBadGateway, "ask_failed", "Could not answer that right now. Please try again.")
	}
}
