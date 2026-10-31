package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// Load tests use the real service, worker pool and event broker with stubbed GitHub and AI, so they measure
// this code and not the network. They run only with LOAD=1:  LOAD=1 go test -run Load -v ./internal/service/

func needLoad(t *testing.T) {
	t.Helper()
	if os.Getenv("LOAD") == "" {
		t.Skip("set LOAD=1 to run load tests")
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p)
	return sorted[i]
}

func sortDurations(d []time.Duration) { sort.Slice(d, func(i, j int) bool { return d[i] < d[j] }) }

// gatedSource holds every review at "fetching" until release is closed.
func gatedSource(release <-chan struct{}) *fakeSource {
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return github.PullRequest{}, ctx.Err()
		}
		return github.PullRequest{Title: "t", HeadSHA: "abc", URL: "https://github.com/o/r/pull/1", State: "open"}, nil
	}
	return src
}

// Many browsers watching one review's progress: every watcher must see the final status, none dropped.
func TestLoadProgressFanOut(t *testing.T) {
	needLoad(t)
	for _, watchers := range []int{1000, 5000, 20000} {
		t.Run(fmt.Sprintf("%d_watchers", watchers), func(t *testing.T) {
			release := make(chan struct{})
			store := newFakeStore()
			svc := newService(t, store, gatedSource(release), Config{})
			rv, err := svc.Create(context.Background(), prURL)
			if err != nil {
				t.Fatal(err)
			}

			var (
				wg         sync.WaitGroup
				mu         sync.Mutex
				lat        = make([]time.Duration, 0, watchers)
				dropped    atomic.Int64
				ready      sync.WaitGroup
				subTime    = make([]time.Duration, watchers)
				releasedAt time.Time // set before the review is let go; read only after events arrive
			)
			ready.Add(watchers)
			wg.Add(watchers)
			for i := 0; i < watchers; i++ {
				go func() {
					defer wg.Done()
					t0 := time.Now()
					stream, err := svc.Watch(context.Background(), rv.ID)
					subTime[i] = time.Since(t0)
					ready.Done()
					if err != nil {
						dropped.Add(1)
						return
					}
					defer stream.Close()
					for ev := range stream.Events {
						if ev.Status == review.StatusCompleted || ev.Status == review.StatusFailed {
							d := time.Since(releasedAt) // monotonic: the wall clock ticks too coarsely on Windows
							mu.Lock()
							lat = append(lat, d)
							mu.Unlock()
							return
						}
					}
					dropped.Add(1) // the channel closed before a final status arrived
				}()
			}
			ready.Wait()

			releasedAt = time.Now()
			close(release)
			wg.Wait()

			sortDurations(lat)
			sortDurations(subTime)
			t.Logf("watchers=%d delivered=%d dropped=%d  subscribe p95=%v  final-status latency p50=%v p95=%v p99=%v max=%v",
				watchers, len(lat), dropped.Load(), percentile(subTime, 0.95),
				percentile(lat, 0.50), percentile(lat, 0.95), percentile(lat, 0.99), percentile(lat, 1))
			if dropped.Load() != 0 || len(lat) != watchers {
				t.Fatalf("delivered %d of %d, dropped %d", len(lat), watchers, dropped.Load())
			}
		})
	}
}

// A burst of review requests larger than the queue: the excess is refused quickly, the rest all finish.
func TestLoadSheddingUnderABurst(t *testing.T) {
	needLoad(t)
	const burst = 500
	release := make(chan struct{})
	store := newFakeStore()
	svc := newService(t, store, gatedSource(release), Config{MaxConcurrent: 2, MaxPending: 50})

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		accepted  []string
		refused   int
		createLat []time.Duration
		other     int
	)
	t0 := time.Now()
	wg.Add(burst)
	for i := 0; i < burst; i++ {
		go func() {
			defer wg.Done()
			s := time.Now()
			rv, err := svc.Create(context.Background(), prURL)
			d := time.Since(s)
			mu.Lock()
			defer mu.Unlock()
			createLat = append(createLat, d)
			switch {
			case err == nil:
				accepted = append(accepted, rv.ID)
			case err == ErrBusy:
				refused++
			default:
				other++
			}
		}()
	}
	wg.Wait()
	burstTime := time.Since(t0)
	close(release)
	for _, id := range accepted {
		waitTerminalLong(t, store, id)
	}

	sortDurations(createLat)
	t.Logf("burst=%d in %v: accepted=%d refused(ErrBusy)=%d other=%d  Create latency p50=%v p95=%v max=%v  all accepted reviews finished=%v",
		burst, burstTime, len(accepted), refused, other, percentile(createLat, 0.5), percentile(createLat, 0.95), percentile(createLat, 1), true)
	if other != 0 || len(accepted)+refused != burst {
		t.Fatalf("accepted %d + refused %d + other %d != %d", len(accepted), refused, other, burst)
	}
}

func waitTerminalLong(t *testing.T, store *fakeStore, id string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		rv, ok := store.reviews[id]
		done := ok && (rv.Status == review.StatusCompleted || rv.Status == review.StatusFailed)
		store.mu.Unlock()
		if done {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("review %s did not finish", id)
}
