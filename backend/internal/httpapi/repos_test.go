package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ask"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

type fakeHistory struct {
	remembered  []string // "user|repo"
	recent      []string
	rememberErr error
	recentErr   error
}

func (f *fakeHistory) RememberRepo(_ context.Context, userID, repo string) error {
	f.remembered = append(f.remembered, userID+"|"+repo)
	return f.rememberErr
}

func (f *fakeHistory) RecentRepos(_ context.Context, userID string, limit int) ([]string, error) {
	if userID != "42" {
		return nil, errors.New("asked for someone else's history")
	}
	return f.recent, f.recentErr
}

func historyAPI(h *fakeHistory, svc fakeService, f *fakeAsk) http.Handler {
	return New(svc, WithCreateRateLimit(0, 0), WithAskRateLimit(0, 0), WithAsk(f), WithRepoHistory(h),
		WithAuth(signedIn(&fakeAuth{githubToken: "gho_user"}), "http://localhost:5173", false)).Handler()
}

func createsReview() fakeService {
	return fakeService{create: func(context.Context, string) (review.Review, error) {
		rv := sampleReview()
		rv.PullRequest.Owner, rv.PullRequest.Repo = "Go-Chi", "Chi"
		return rv, nil
	}}
}

const goodReview = `{"prUrl":"https://github.com/Go-Chi/Chi/pull/3"}`

func TestRecentReposListsTheSignedInUsersRepos(t *testing.T) {
	h := historyAPI(&fakeHistory{recent: []string{"go-chi/chi", "acme/shop"}}, fakeService{}, &fakeAsk{})

	rec := requestAs(h, "GET", "/api/repos/recent", "", true)

	var got struct{ Repos []string }
	decode(t, rec, &got)
	if rec.Code != 200 || strings.Join(got.Repos, ",") != "go-chi/chi,acme/shop" {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRecentReposIsEmptyForAnonymousVisitorsAndNeverNull(t *testing.T) {
	h := historyAPI(&fakeHistory{recent: []string{"someone/elses"}}, fakeService{}, &fakeAsk{})

	for _, signedInUser := range []bool{false, true} {
		h := h
		if signedInUser {
			h = historyAPI(&fakeHistory{}, fakeService{}, &fakeAsk{})
		}
		rec := requestAs(h, "GET", "/api/repos/recent", "", signedInUser)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"repos":[]`) {
			t.Errorf("signed in = %v: %d %s, want an empty array", signedInUser, rec.Code, rec.Body.String())
		}
	}
}

func TestRecentReposFailureIsAnInternalErrorWithoutDetail(t *testing.T) {
	h := historyAPI(&fakeHistory{recentErr: errors.New("connection refused at 10.0.0.5")}, fakeService{}, &fakeAsk{})

	rec := requestAs(h, "GET", "/api/repos/recent", "", true)

	wantError(t, rec, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Error("the response leaked the internal error")
	}
}

func TestRecentReposIsNotRoutedUnlessEnabled(t *testing.T) {
	h := New(fakeService{}, WithCreateRateLimit(0, 0)).Handler()
	wantError(t, requestAs(h, "GET", "/api/repos/recent", "", false), http.StatusNotFound, "not_found")
}

func TestStartingAReviewRemembersTheRepoForASignedInUser(t *testing.T) {
	hist := &fakeHistory{}
	h := historyAPI(hist, createsReview(), &fakeAsk{})

	if rec := requestAs(h, "POST", "/api/reviews", goodReview, true); rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Join(hist.remembered, ",") != "42|go-chi/chi" {
		t.Fatalf("remembered %v, want the lowercase repo for user 42", hist.remembered)
	}
}

func TestAnonymousReviewsAndAsksAreNotRemembered(t *testing.T) {
	hist := &fakeHistory{}
	f := &fakeAsk{answer: ask.Answer{Text: "x", Repo: "acme/shop"}}
	h := historyAPI(hist, createsReview(), f)

	requestAs(h, "POST", "/api/reviews", goodReview, false)
	requestAs(h, "POST", "/api/ask", goodAsk, false)

	if len(hist.remembered) != 0 {
		t.Fatalf("remembered %v for an anonymous visitor", hist.remembered)
	}
}

func TestAskingRemembersTheRepoTheAnswerWasAbout(t *testing.T) {
	hist := &fakeHistory{}
	// The review's repo wins over the one in the request, so the answer says which one it used.
	f := &fakeAsk{answer: ask.Answer{Text: "x", Repo: "acme/orders"}}
	h := historyAPI(hist, fakeService{}, f)

	requestAs(h, "POST", "/api/ask", goodAsk, true)

	if strings.Join(hist.remembered, ",") != "42|acme/orders" {
		t.Fatalf("remembered %v", hist.remembered)
	}
}

func TestAFailedAskIsNotRemembered(t *testing.T) {
	hist := &fakeHistory{}
	h := historyAPI(hist, fakeService{}, &fakeAsk{err: errors.New("boom")})

	requestAs(h, "POST", "/api/ask", goodAsk, true)

	if len(hist.remembered) != 0 {
		t.Fatalf("remembered %v after a failed question", hist.remembered)
	}
}

func TestAFailureToRememberNeverFailsTheRequest(t *testing.T) {
	hist := &fakeHistory{rememberErr: errors.New("db down")}
	f := &fakeAsk{answer: ask.Answer{Text: "fine", Repo: "acme/shop"}}
	h := historyAPI(hist, createsReview(), f)

	if rec := requestAs(h, "POST", "/api/reviews", goodReview, true); rec.Code != http.StatusAccepted {
		t.Errorf("create = %d, want 202", rec.Code)
	}
	if rec := requestAs(h, "POST", "/api/ask", goodAsk, true); rec.Code != http.StatusOK {
		t.Errorf("ask = %d, want 200", rec.Code)
	}
}
