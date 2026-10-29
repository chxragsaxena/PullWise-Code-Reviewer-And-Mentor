package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

const okReview = `{"choices":[{"message":{"role":"assistant","content":"{\"findings\":[]}"},"finish_reason":"stop"}]}`

// limitedServer answers the first `limited` requests with 429, then succeeds.
func limitedServer(t *testing.T, limited int32, header, message string) (*GroqProvider, *atomic.Int32, *[]time.Duration) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= limited {
			if header != "" {
				w.Header().Set("Retry-After", header)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, `{"error":{"message":%q,"type":"tokens"}}`, message)
			return
		}
		fmt.Fprint(w, okReview)
	}))
	t.Cleanup(srv.Close)

	var slept []time.Duration
	p := NewGroqProvider("k", WithGroqBaseURL(srv.URL))
	p.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	return p, &hits, &slept
}

func reviewOnce(p *GroqProvider, t *testing.T) error {
	sk := testSkill(t)
	_, err := p.Review(context.Background(), ReviewInput{
		Skill: sk, Checks: sk.Checks, Files: []review.ChangedFile{{Path: "a.go", Patch: "+x"}},
	})
	return err
}

func TestARateLimitedCallWaitsAndRetries(t *testing.T) {
	p, hits, slept := limitedServer(t, 1, "9", "Rate limit reached")

	if err := reviewOnce(p, t); err != nil {
		t.Fatalf("Review: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("made %d requests, want 2 (one limited, one retry)", hits.Load())
	}
	if len(*slept) != 1 || (*slept)[0] < 9*time.Second || (*slept)[0] > 10*time.Second {
		t.Fatalf("waited %v, want about the 9 seconds Groq asked for", *slept)
	}
}

func TestTheWaitIsReadFromTheMessageWhenThereIsNoHeader(t *testing.T) {
	p, _, slept := limitedServer(t, 1, "", "Rate limit reached. Please try again in 9.6825s. Need more tokens?")
	if err := reviewOnce(p, t); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] < 9*time.Second || (*slept)[0] > 11*time.Second {
		t.Fatalf("waited %v, want about 9.7s from the message", *slept)
	}
}

func TestItGivesUpAfterTwoRetries(t *testing.T) {
	p, hits, _ := limitedServer(t, 100, "1", "Rate limit reached")

	err := reviewOnce(p, t)
	var apiErr *GroqAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Fatalf("error = %v, want the 429 after retries ran out", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("made %d requests, want 3 (the first plus two retries)", hits.Load())
	}
}

func TestItDoesNotWaitLongerThanTheCap(t *testing.T) {
	p, hits, slept := limitedServer(t, 100, "120", "daily limit")

	if err := reviewOnce(p, t); err == nil {
		t.Fatal("expected the 429 to be returned")
	}
	if hits.Load() != 1 || len(*slept) != 0 {
		t.Fatalf("hits=%d slept=%v; a two-minute wait must fail fast, not block the review", hits.Load(), *slept)
	}
}

func TestCancellingDuringTheWaitStopsRetrying(t *testing.T) {
	p, hits, _ := limitedServer(t, 100, "5", "Rate limit reached")
	p.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	if err := reviewOnce(p, t); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("made %d requests after cancellation", hits.Load())
	}
}

func TestOtherErrorsAreNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key","type":"auth"}}`)
	}))
	t.Cleanup(srv.Close)
	p := NewGroqProvider("k", WithGroqBaseURL(srv.URL))
	p.sleep = func(context.Context, time.Duration) error { t.Error("slept for a non-rate-limit error"); return nil }

	if err := reviewOnce(p, t); err == nil || hits.Load() != 1 {
		t.Fatalf("err=%v hits=%d, want one failed request", err, hits.Load())
	}
}
