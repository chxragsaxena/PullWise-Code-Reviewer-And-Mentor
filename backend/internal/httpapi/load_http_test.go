package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// Throughput and latency of the read endpoints over real sockets, with the review store stubbed in memory:
// this measures the HTTP layer (routing, JSON encoding, middleware), not the database.
// Run with:  LOAD=1 go test -run LoadHTTP -v ./internal/httpapi/

func TestLoadHTTPReads(t *testing.T) {
	if os.Getenv("LOAD") == "" {
		t.Skip("set LOAD=1 to run load tests")
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	list := make([]review.Review, 20)
	for i := range list {
		list[i] = sampleReview()
	}
	rv := sampleReview()
	svc := fakeService{
		list: func(context.Context, int) ([]review.Review, error) { return list, nil },
		get:  func(context.Context, string) (review.Review, error) { return rv, nil },
	}
	srv := httptest.NewServer(newAPI(svc))
	defer srv.Close()

	for _, path := range []string{"/healthz", "/api/reviews", "/api/reviews/7"} {
		for _, workers := range []int{16, 64, 256} {
			t.Run(fmt.Sprintf("%s_%d_workers", path, workers), func(t *testing.T) {
				client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: workers, DisableCompression: true}}
				defer client.CloseIdleConnections()
				const window = 4 * time.Second

				var (
					wg     sync.WaitGroup
					mu     sync.Mutex
					lat    []time.Duration
					errors atomic.Int64
				)
				stop := time.Now().Add(window)
				start := time.Now()
				for w := 0; w < workers; w++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						var local []time.Duration
						for time.Now().Before(stop) {
							t0 := time.Now()
							resp, err := client.Get(srv.URL + path)
							if err != nil {
								errors.Add(1)
								continue
							}
							_, _ = io.Copy(io.Discard, resp.Body)
							resp.Body.Close()
							if resp.StatusCode != 200 {
								errors.Add(1)
								continue
							}
							local = append(local, time.Since(t0))
						}
						mu.Lock()
						lat = append(lat, local...)
						mu.Unlock()
					}()
				}
				wg.Wait()
				elapsed := time.Since(start)

				sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
				t.Logf("%s with %d connections: %.0f req/s, p50=%v p95=%v p99=%v, errors=%d of %d",
					path, workers, float64(len(lat))/elapsed.Seconds(), pct(lat, 0.5), pct(lat, 0.95), pct(lat, 0.99), errors.Load(), int64(len(lat))+errors.Load())
			})
		}
	}
}
