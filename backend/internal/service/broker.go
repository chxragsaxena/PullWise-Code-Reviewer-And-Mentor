package service

import (
	"sync"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// subscriberBuffer is how many undelivered events a subscriber may lag by.
const subscriberBuffer = 16

// Broker fans review status events out to live subscribers, in memory.
//
// It carries no history: the database is the source of truth for the current
// status, and a new subscriber reads that first. A subscriber that falls too far
// behind is disconnected rather than blocking the pipeline; it simply reconnects
// and is told the current status again.
type Broker struct {
	mu   sync.Mutex
	subs map[string]map[*subscription]struct{}
}

type subscription struct {
	ch     chan review.Event
	closed bool
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[string]map[*subscription]struct{})}
}

// Subscribe returns a channel of events for one review and a function that
// stops the subscription. The channel is closed by cancel, or by the broker if
// the subscriber cannot keep up.
func (b *Broker) Subscribe(reviewID string) (<-chan review.Event, func()) {
	sub := &subscription{ch: make(chan review.Event, subscriberBuffer)}

	b.mu.Lock()
	if b.subs[reviewID] == nil {
		b.subs[reviewID] = make(map[*subscription]struct{})
	}
	b.subs[reviewID][sub] = struct{}{}
	b.mu.Unlock()

	return sub.ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.remove(reviewID, sub)
	}
}

// Publish delivers an event to every subscriber of the review without blocking.
func (b *Broker) Publish(reviewID string, event review.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subs[reviewID] {
		select {
		case sub.ch <- event:
		default:
			b.remove(reviewID, sub) // too slow: drop it, the client will reconnect
		}
	}
}

// remove closes and forgets a subscription. The caller holds b.mu.
func (b *Broker) remove(reviewID string, sub *subscription) {
	if sub.closed {
		return
	}
	sub.closed = true
	close(sub.ch)
	delete(b.subs[reviewID], sub)
	if len(b.subs[reviewID]) == 0 {
		delete(b.subs, reviewID)
	}
}
