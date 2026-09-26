package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

// fakeService lets each test script only the calls it cares about.
type fakeService struct {
	create     func(ctx context.Context, prURL string) (review.Review, error)
	get        func(ctx context.Context, id string) (review.Review, error)
	list       func(ctx context.Context, limit int) ([]review.Review, error)
	getFinding func(ctx context.Context, reviewID, findingID string) (service.FindingDetail, error)
	watch      func(ctx context.Context, id string) (*service.Stream, error)
}

func (f fakeService) Create(ctx context.Context, u string) (review.Review, error) {
	return f.create(ctx, u)
}
func (f fakeService) Get(ctx context.Context, id string) (review.Review, error) {
	return f.get(ctx, id)
}
func (f fakeService) List(ctx context.Context, n int) ([]review.Review, error) { return f.list(ctx, n) }
func (f fakeService) GetFinding(ctx context.Context, r, fd string) (service.FindingDetail, error) {
	return f.getFinding(ctx, r, fd)
}
func (f fakeService) Watch(ctx context.Context, id string) (*service.Stream, error) {
	return f.watch(ctx, id)
}

var t0 = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func sampleReview() review.Review {
	return review.Review{
		ID: "7", Status: review.StatusQueued, StatusMessage: "Waiting to start", StatusAt: t0, CreatedAt: t0,
		PullRequest: review.PullRequest{Owner: "o", Repo: "r", Number: 3},
	}
}

// newAPI builds an API with rate limiting off unless a test turns it on.
func newAPI(svc ReviewService, opts ...Option) http.Handler {
	return New(svc, append([]Option{WithCreateRateLimit(0, 0)}, opts...)...).Handler()
}

func do(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body errorBody
	decode(t, rec, &body)
	if body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("error = %+v, want code %q with a message", body.Error, code)
	}
}

func TestHealthAndReadiness(t *testing.T) {
	h := newAPI(fakeService{})
	if rec := do(h, http.MethodGet, "/healthz", "", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("/healthz = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, http.MethodGet, "/readyz", "", ""); rec.Code != 200 {
		t.Fatalf("/readyz without a check = %d, want 200", rec.Code)
	}

	down := newAPI(fakeService{}, WithReadiness(func(context.Context) error { return errors.New("db down") }))
	rec := do(down, http.MethodGet, "/readyz", "", "")
	wantError(t, rec, http.StatusServiceUnavailable, "not_ready")
	if strings.Contains(rec.Body.String(), "db down") {
		t.Error("readiness failure leaked its internal reason")
	}
}

func TestRoutingErrorsUseTheStandardJSONShape(t *testing.T) {
	h := newAPI(fakeService{})

	wantError(t, do(h, http.MethodGet, "/nope", "", ""), http.StatusNotFound, "not_found")
	wantError(t, do(h, http.MethodGet, "/api/nope", "", ""), http.StatusNotFound, "not_found")

	rec := do(h, http.MethodDelete, "/api/reviews", "", "")
	wantError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if allow := rec.Header().Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow = %q, want %q", allow, "GET, POST")
	}
	wantError(t, do(h, http.MethodPost, "/api/reviews/7", "", ""), http.StatusMethodNotAllowed, "method_not_allowed")
	wantError(t, do(h, http.MethodPost, "/healthz", "", ""), http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestResponsesAreNotCacheableAndNotSniffable(t *testing.T) {
	rec := do(newAPI(fakeService{}), http.MethodGet, "/healthz", "", "")
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestCreateReview(t *testing.T) {
	var gotURL string
	h := newAPI(fakeService{create: func(_ context.Context, u string) (review.Review, error) {
		gotURL = u
		return sampleReview(), nil
	}})

	rec := do(h, http.MethodPost, "/api/reviews", `{"prUrl":"https://github.com/o/r/pull/3"}`, "application/json; charset=utf-8")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body %s)", rec.Code, rec.Body.String())
	}
	if gotURL != "https://github.com/o/r/pull/3" {
		t.Errorf("service got %q", gotURL)
	}

	// Exact wire shape from docs/api.md, including nulls and zero values.
	var got map[string]any
	decode(t, rec, &got)
	if got["id"] != "7" || got["status"] != "queued" || got["completedAt"] != nil || got["createdAt"] != "2026-09-26T10:00:00Z" {
		t.Fatalf("summary = %v", got)
	}
	pr := got["pullRequest"].(map[string]any)
	for _, key := range []string{"owner", "repo", "number", "title", "author", "url", "headSha", "filesChanged", "additions", "deletions"} {
		if _, ok := pr[key]; !ok {
			t.Errorf("pullRequest is missing %q", key)
		}
	}
	if counts := got["counts"].(map[string]any); len(counts) != 4 || counts["critical"] != float64(0) {
		t.Errorf("counts = %v", counts)
	}
	if _, present := got["findings"]; present {
		t.Error("a summary must not include findings")
	}
}

func TestCreateReviewRejectsBadRequests(t *testing.T) {
	called := false
	h := newAPI(fakeService{create: func(context.Context, string) (review.Review, error) {
		called = true
		return review.Review{}, nil
	}})

	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
		code        string
	}{
		{"wrong content type", `{"prUrl":"x"}`, "text/plain", 415, "unsupported_media_type"},
		{"no content type", `{"prUrl":"x"}`, "", 415, "unsupported_media_type"},
		{"not json", `prUrl=x`, "application/json", 400, "invalid_request"},
		{"empty body", ``, "application/json", 400, "invalid_request"},
		{"trailing data", `{"prUrl":"x"}{"prUrl":"y"}`, "application/json", 400, "invalid_request"},
		{"too large", `{"prUrl":"` + strings.Repeat("a", maxCreateBody) + `"}`, "application/json", 413, "request_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, do(h, http.MethodPost, "/api/reviews", tt.body, tt.contentType), tt.status, tt.code)
		})
	}
	if called {
		t.Fatal("the service was called for a malformed request")
	}
}

func TestServiceErrorsMapToDocumentedResponses(t *testing.T) {
	tests := []struct {
		err       error
		status    int
		code      string
		retryHint bool
	}{
		{service.ErrInvalidPRURL, 400, "invalid_pr_url", false},
		{service.ErrBusy, 503, "server_busy", true},
		{service.ErrShuttingDown, 503, "shutting_down", false},
		{errors.New("pq: connection refused at 10.0.0.5"), 500, "internal_error", false},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			h := newAPI(fakeService{create: func(context.Context, string) (review.Review, error) { return review.Review{}, tt.err }})
			rec := do(h, http.MethodPost, "/api/reviews", `{"prUrl":"x"}`, "application/json")
			wantError(t, rec, tt.status, tt.code)
			if tt.retryHint && rec.Header().Get("Retry-After") == "" {
				t.Error("missing Retry-After")
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Error("internal error detail leaked to the client")
			}
		})
	}
}

func TestCreateIsRateLimitedPerClient(t *testing.T) {
	h := newAPI(fakeService{create: func(context.Context, string) (review.Review, error) { return sampleReview(), nil }},
		WithCreateRateLimit(60, 2))

	post := func(remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(`{"prUrl":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 2; i++ {
		if rec := post("198.51.100.1:5000"); rec.Code != http.StatusAccepted {
			t.Fatalf("request %d = %d, want 202 within the burst", i+1, rec.Code)
		}
	}
	rec := post("198.51.100.1:5001") // same address, different port
	wantError(t, rec, http.StatusTooManyRequests, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After")
	}
	if rec := post("203.0.113.9:5000"); rec.Code != http.StatusAccepted {
		t.Fatalf("another client was limited too: %d", rec.Code)
	}

	// A forged forwarding header must not buy a fresh allowance.
	req := httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(`{"prUrl":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	req.RemoteAddr = "198.51.100.1:5002"
	spoofed := httptest.NewRecorder()
	h.ServeHTTP(spoofed, req)
	wantError(t, spoofed, http.StatusTooManyRequests, "rate_limited")
}

func TestListReviews(t *testing.T) {
	var gotLimit int
	reviews := []review.Review{sampleReview()}
	h := newAPI(fakeService{list: func(_ context.Context, n int) ([]review.Review, error) {
		gotLimit = n
		return reviews, nil
	}})

	rec := do(h, http.MethodGet, "/api/reviews?limit=5", "", "")
	var body struct {
		Reviews []map[string]any `json:"reviews"`
	}
	decode(t, rec, &body)
	if rec.Code != 200 || gotLimit != 5 || len(body.Reviews) != 1 || body.Reviews[0]["id"] != "7" {
		t.Fatalf("got %d, limit %d, body %+v", rec.Code, gotLimit, body)
	}

	do(h, http.MethodGet, "/api/reviews", "", "")
	if gotLimit != 0 {
		t.Errorf("limit without a query = %d, want 0 (service default)", gotLimit)
	}

	wantError(t, do(h, http.MethodGet, "/api/reviews?limit=abc", "", ""), 400, "invalid_request")

	// No reviews must serialize as [] and never null.
	reviews = nil
	rec = do(h, http.MethodGet, "/api/reviews", "", "")
	if !strings.Contains(rec.Body.String(), `"reviews":[]`) {
		t.Fatalf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestGetReviewShape(t *testing.T) {
	done := t0.Add(time.Minute)
	rv := sampleReview()
	rv.Status, rv.CompletedAt = review.StatusCompleted, &done
	rv.Counts = review.Counts{High: 1, Suggestion: 1}
	rv.Findings = []review.Finding{
		{ID: "1", SkillID: "go", SkillName: "Go", CheckID: "ignored-error", CheckTitle: "Ignored", Source: review.SourceDeterministic,
			Severity: review.SeverityHigh, Confidence: 1, File: "a.go", Line: 12, EndLine: 14, Evidence: "x", Found: "f", Why: "w", Improvement: "i", Lesson: "errors"},
		{ID: "2", SkillID: "go", CheckID: "big", Source: review.SourceAI, Severity: review.SeveritySuggestion, Confidence: 0.74, File: "b.go"}, // whole file, no lesson
	}
	h := newAPI(fakeService{get: func(_ context.Context, id string) (review.Review, error) {
		if id != "7" {
			return review.Review{}, review.ErrReviewNotFound
		}
		return rv, nil
	}})

	rec := do(h, http.MethodGet, "/api/reviews/7", "", "")
	var got struct {
		CompletedAt *string          `json:"completedAt"`
		Error       any              `json:"error"`
		Findings    []map[string]any `json:"findings"`
	}
	decode(t, rec, &got)
	if rec.Code != 200 || got.CompletedAt == nil || *got.CompletedAt != "2026-09-26T10:01:00Z" || got.Error != nil || len(got.Findings) != 2 {
		t.Fatalf("got %d %+v", rec.Code, got)
	}
	first, second := got.Findings[0], got.Findings[1]
	if first["line"] != float64(12) || first["endLine"] != float64(14) || first["lessonId"] != "errors" || first["confidence"] != float64(1) {
		t.Errorf("first finding = %v", first)
	}
	for _, key := range []string{"line", "endLine", "lessonId"} {
		if v, present := second[key]; !present || v != nil {
			t.Errorf("whole-file finding %q = %v (present %v), want an explicit null", key, v, present)
		}
	}

	wantError(t, do(h, http.MethodGet, "/api/reviews/999", "", ""), 404, "review_not_found")
}

func TestGetFailedReviewCarriesStructuredError(t *testing.T) {
	rv := sampleReview()
	rv.Status = review.StatusFailed
	rv.Error = &review.ErrorInfo{Code: "pr_not_found", Message: "Pull request not found."}
	h := newAPI(fakeService{get: func(context.Context, string) (review.Review, error) { return rv, nil }})

	var got struct {
		Error    map[string]string `json:"error"`
		Findings []any             `json:"findings"`
	}
	decode(t, do(h, http.MethodGet, "/api/reviews/7", "", ""), &got)
	if got.Error["code"] != "pr_not_found" || got.Error["message"] == "" {
		t.Fatalf("error = %v", got.Error)
	}
	if got.Findings == nil {
		t.Fatal("findings must be an empty array, not null")
	}
}

func TestGetFinding(t *testing.T) {
	h := newAPI(fakeService{getFinding: func(_ context.Context, reviewID, findingID string) (service.FindingDetail, error) {
		switch findingID {
		case "1":
			return service.FindingDetail{
				Finding: review.Finding{ID: "1", SkillID: "go", CheckID: "c", Source: review.SourceDeterministic, Severity: review.SeverityHigh, Confidence: 1, File: "a.go", Line: 3, Lesson: "errors"},
				Lesson:  &service.Lesson{ID: "errors", Markdown: "# Errors"},
			}, nil
		case "2":
			return service.FindingDetail{Finding: review.Finding{ID: "2", File: "a.go"}}, nil
		}
		return service.FindingDetail{}, review.ErrFindingNotFound
	}})

	var withLesson struct {
		ID     string `json:"id"`
		Lesson *struct {
			ID       string `json:"id"`
			Markdown string `json:"markdown"`
		} `json:"lesson"`
	}
	decode(t, do(h, http.MethodGet, "/api/reviews/7/findings/1", "", ""), &withLesson)
	if withLesson.ID != "1" || withLesson.Lesson == nil || withLesson.Lesson.Markdown != "# Errors" {
		t.Fatalf("got %+v", withLesson)
	}

	rec := do(h, http.MethodGet, "/api/reviews/7/findings/2", "", "")
	if !strings.Contains(rec.Body.String(), `"lesson":null`) {
		t.Fatalf("body = %s, want an explicit null lesson", rec.Body.String())
	}
	wantError(t, do(h, http.MethodGet, "/api/reviews/7/findings/99", "", ""), 404, "finding_not_found")
}

// --- Server-Sent Events -----------------------------------------------------

type streamHandle struct {
	events chan review.Event
	closed atomic.Bool
}

// eventServer starts a real HTTP server whose watch() hands out the given stream.
func eventServer(t *testing.T, current review.Event, opts ...Option) (*httptest.Server, *streamHandle, *API) {
	t.Helper()
	h := &streamHandle{events: make(chan review.Event, 8)}
	svc := fakeService{watch: func(_ context.Context, id string) (*service.Stream, error) {
		if id != "7" {
			return nil, review.ErrReviewNotFound
		}
		st := &service.Stream{Current: current, Close: func() { h.closed.Store(true) }}
		if !current.Status.Terminal() {
			st.Events = h.events
		}
		return st, nil
	}}
	api := New(svc, append([]Option{WithCreateRateLimit(0, 0)}, opts...)...)
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv, h, api
}

// readLines streams a response body's lines on a channel that closes at EOF.
func readLines(body *bufio.Scanner) <-chan string {
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		for body.Scan() {
			lines <- body.Text()
		}
	}()
	return lines
}

// nextStatus returns the status of the next "data:" line, skipping the rest.
func nextStatus(t *testing.T, lines <-chan string) (string, bool) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return "", false
			}
			if data, found := strings.CutPrefix(line, "data: "); found {
				var e eventDTO
				if err := json.Unmarshal([]byte(data), &e); err != nil {
					t.Fatalf("bad event data %q: %v", data, err)
				}
				return e.Status, true
			}
		case <-timeout:
			t.Fatal("timed out waiting for an event")
		}
	}
}

func waitTrue(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestEventStreamDeliversChangesThenEndsAtTerminalStatus(t *testing.T) {
	srv, handle, _ := eventServer(t, review.Event{Status: review.StatusQueued, Message: "Waiting", At: t0})

	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("status %d, headers %v", resp.StatusCode, resp.Header)
	}
	lines := readLines(bufio.NewScanner(resp.Body))

	if s, _ := nextStatus(t, lines); s != "queued" {
		t.Fatalf("first event = %q, want the current status", s)
	}
	handle.events <- review.Event{Status: review.StatusAnalyzing, Message: "a", At: t0}
	handle.events <- review.Event{Status: review.StatusAnalyzing, Message: "dup", At: t0} // must be skipped
	handle.events <- review.Event{Status: review.StatusCompleted, Message: "done", At: t0}

	if s, _ := nextStatus(t, lines); s != "analyzing" {
		t.Fatalf("second event = %q", s)
	}
	if s, _ := nextStatus(t, lines); s != "completed" {
		t.Fatalf("third event = %q; the duplicate should have been skipped", s)
	}
	if _, more := nextStatus(t, lines); more {
		t.Fatal("stream should end after the terminal status")
	}
	waitTrue(t, "the subscription to be closed", handle.closed.Load)
}

func TestEventStreamWireFormat(t *testing.T) {
	srv, _, _ := eventServer(t, review.Event{Status: review.StatusFailed, Message: "It failed", At: t0})
	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var raw strings.Builder
	for line := range readLines(bufio.NewScanner(resp.Body)) {
		raw.WriteString(line + "\n")
	}
	want := "event: status\ndata: {\"status\":\"failed\",\"message\":\"It failed\",\"at\":\"2026-09-26T10:00:00Z\"}\n\n"
	if raw.String() != want {
		t.Fatalf("stream = %q\nwant     %q", raw.String(), want)
	}
}

func TestEventStreamForFinishedReviewSendsOneEventAndCloses(t *testing.T) {
	srv, handle, _ := eventServer(t, review.Event{Status: review.StatusCompleted, Message: "done", At: t0})
	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := readLines(bufio.NewScanner(resp.Body))
	if s, _ := nextStatus(t, lines); s != "completed" {
		t.Fatalf("status = %q", s)
	}
	if _, more := nextStatus(t, lines); more {
		t.Fatal("expected the stream to end")
	}
	waitTrue(t, "the subscription to be closed", handle.closed.Load)
}

func TestEventStreamUnknownReviewIsAnOrdinaryJSONError(t *testing.T) {
	srv, _, _ := eventServer(t, review.Event{Status: review.StatusQueued})
	resp, err := http.Get(srv.URL + "/api/reviews/404/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestEventStreamSendsKeepAlivesWhileIdle(t *testing.T) {
	srv, _, _ := eventServer(t, review.Event{Status: review.StatusQueued, At: t0}, WithHeartbeat(10*time.Millisecond))
	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := readLines(bufio.NewScanner(resp.Body))
	timeout := time.After(3 * time.Second)
	for {
		select {
		case line := <-lines:
			if line == ": keepalive" {
				return
			}
		case <-timeout:
			t.Fatal("no keep-alive comment arrived")
		}
	}
}

func TestEventStreamStopsWhenTheClientDisconnects(t *testing.T) {
	srv, handle, _ := eventServer(t, review.Event{Status: review.StatusQueued, At: t0})
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/reviews/7/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := readLines(bufio.NewScanner(resp.Body))
	nextStatus(t, lines)
	cancel()
	waitTrue(t, "the handler to release its subscription after the client left", handle.closed.Load)
}

func TestEventStreamEndsWhenTheSubscriberIsDropped(t *testing.T) {
	srv, handle, _ := eventServer(t, review.Event{Status: review.StatusQueued, At: t0})
	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := readLines(bufio.NewScanner(resp.Body))
	nextStatus(t, lines)
	close(handle.events) // the broker dropped us for being too slow
	if _, more := nextStatus(t, lines); more {
		t.Fatal("expected the stream to end so the browser reconnects")
	}
}

func TestBeginShutdownEndsOpenStreams(t *testing.T) {
	srv, handle, api := eventServer(t, review.Event{Status: review.StatusQueued, At: t0})
	resp, err := http.Get(srv.URL + "/api/reviews/7/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := readLines(bufio.NewScanner(resp.Body))
	nextStatus(t, lines)
	api.BeginShutdown()
	api.BeginShutdown() // idempotent
	if _, more := nextStatus(t, lines); more {
		t.Fatal("stream should end on shutdown")
	}
	waitTrue(t, "the subscription to be closed", handle.closed.Load)
}

// --- Rate limiter -----------------------------------------------------------

func TestLimiterRefillsOverTimeAndSweepsIdleKeys(t *testing.T) {
	now := t0
	l := newLimiter(60, 2, func() time.Time { return now }) // 1 token per second, burst 2

	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("request %d denied within the burst", i+1)
		}
	}
	ok, wait := l.allow("a")
	if ok || wait <= 0 || wait > time.Second+time.Millisecond {
		t.Fatalf("allow = %v, wait %v; want denied with a wait of about 1s", ok, wait)
	}

	now = now.Add(1100 * time.Millisecond)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("a token should have refilled after ~1s")
	}
	if ok, _ := l.allow("a"); ok {
		t.Fatal("only one token should have refilled")
	}

	// Refill never exceeds the burst, however long the key was idle.
	now = now.Add(time.Hour)
	l.allow("b")
	l.allow("b")
	if ok, _ := l.allow("b"); ok {
		t.Fatal("burst was exceeded after a long idle period")
	}

}

func TestLimiterForgetsIdleKeysSoMemoryStaysBounded(t *testing.T) {
	now := t0
	l := newLimiter(60, 2, func() time.Time { return now })
	size := func() int {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.buckets)
	}

	l.allow("x")
	l.allow("y")
	if size() != 2 {
		t.Fatalf("tracked keys = %d, want 2", size())
	}

	now = now.Add(time.Hour) // both are long idle, so their buckets are full again
	l.allow("z")             // the next request sweeps them
	if size() != 1 {
		t.Fatalf("tracked keys after the sweep = %d, want only z", size())
	}
}
