package httpapi

import (
	"sync"
	"time"
)

// limiter is a per-key token bucket: each key may burst up to `burst` requests,
// then is limited to `rate` requests per second. It is safe for concurrent use.
type limiter struct {
	rate  float64
	burst float64
	now   func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(perMinute, burst int, now func() time.Time) *limiter {
	return &limiter{
		rate:    float64(perMinute) / 60,
		burst:   float64(burst),
		now:     now,
		buckets: make(map[string]*bucket),
	}
}

// allow reports whether the key may make a request now. When it may not, it
// also returns how long to wait before a request would succeed.
func (l *limiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

// sweep forgets keys that have been idle long enough to have a full bucket
// again, so the map cannot grow without bound. The caller holds l.mu.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	idle := time.Duration(l.burst / l.rate * float64(time.Second))
	for key, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, key)
		}
	}
}
