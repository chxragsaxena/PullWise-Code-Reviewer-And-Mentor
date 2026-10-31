package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

// Real sockets, the real SSE handler and the real event broker; only the review pipeline is a stub.
// Run with:  LOAD=1 go test -run Load -v ./internal/httpapi/

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(float64(len(sorted)-1)*p)]
}

func TestLoadSSEConnections(t *testing.T) {
	if os.Getenv("LOAD") == "" {
		t.Skip("set LOAD=1 to run load tests")
	}
	// One log line per request would dominate the measurement.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for _, clients := range []int{1000, 5000} {
		t.Run(fmt.Sprintf("%d_clients", clients), func(t *testing.T) {
			broker := service.NewBroker()
			svc := fakeService{watch: func(_ context.Context, id string) (*service.Stream, error) {
				events, cancel := broker.Subscribe(id)
				return &service.Stream{
					Current: review.Event{Status: review.StatusAnalyzing, Message: "Running static analysis", At: time.Now()},
					Events:  events,
					Close:   cancel,
				}, nil
			}}
			srv := httptest.NewServer(newAPI(svc))
			defer srv.Close()

			client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: clients, MaxConnsPerHost: 0, DisableCompression: true}}
			var (
				wg        sync.WaitGroup
				connected sync.WaitGroup
				mu        sync.Mutex
				lat       []time.Duration
				failed    atomic.Int64
				firstErr  atomic.Value
				connTime  = make([]time.Duration, clients)
				publishAt time.Time
			)
			connected.Add(clients)
			wg.Add(clients)
			for i := 0; i < clients; i++ {
				go func() {
					defer wg.Done()
					time.Sleep(time.Duration(i/50) * 80 * time.Millisecond) // visitors arrive over time: about 600 a second
					t0 := time.Now()
					resp, err := client.Get(srv.URL + "/api/reviews/7/events")
					if err != nil {
						firstErr.CompareAndSwap(nil, err.Error())
						failed.Add(1)
						connected.Done()
						return
					}
					defer resp.Body.Close()
					sc := bufio.NewScanner(resp.Body)
					gotFirst := false
					for sc.Scan() {
						line := sc.Text()
						if !gotFirst && strings.HasPrefix(line, "data:") { // the current status arrives on connect
							gotFirst = true
							connTime[i] = time.Since(t0)
							connected.Done()
							continue
						}
						if strings.HasPrefix(line, "data:") && strings.Contains(line, `"completed"`) {
							d := time.Since(publishAt)
							mu.Lock()
							lat = append(lat, d)
							mu.Unlock()
							return
						}
					}
					if !gotFirst {
						connected.Done()
					}
					failed.Add(1) // the stream ended without a final status
				}()
			}
			connected.Wait()

			publishAt = time.Now()
			broker.Publish("7", review.Event{Status: review.StatusCompleted, Message: "Review complete", At: time.Now()})
			wg.Wait()

			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			sort.Slice(connTime, func(i, j int) bool { return connTime[i] < connTime[j] })
			t.Logf("clients=%d delivered=%d failed=%d  connect p50=%v p95=%v  final-status latency p50=%v p95=%v p99=%v max=%v",
				clients, len(lat), failed.Load(), pct(connTime, 0.5), pct(connTime, 0.95),
				pct(lat, 0.5), pct(lat, 0.95), pct(lat, 0.99), pct(lat, 1))
			if failed.Load() != 0 || len(lat) != clients {
				t.Fatalf("delivered %d of %d clients, %d failed; first error: %v", len(lat), clients, failed.Load(), firstErr.Load())
			}
		})
	}
}
