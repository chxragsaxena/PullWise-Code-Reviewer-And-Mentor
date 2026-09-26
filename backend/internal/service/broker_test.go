package service

import (
	"sync"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func event(s review.Status) review.Event {
	return review.Event{Status: s, Message: string(s), At: time.Now()}
}

func TestBrokerDeliversToTheRightSubscribers(t *testing.T) {
	b := NewBroker()
	a1, cancelA1 := b.Subscribe("a")
	a2, cancelA2 := b.Subscribe("a")
	other, cancelOther := b.Subscribe("b")
	defer cancelA1()
	defer cancelA2()
	defer cancelOther()

	b.Publish("a", event(review.StatusAnalyzing))

	for name, ch := range map[string]<-chan review.Event{"a1": a1, "a2": a2} {
		select {
		case e := <-ch:
			if e.Status != review.StatusAnalyzing {
				t.Errorf("%s got %v", name, e.Status)
			}
		default:
			t.Errorf("%s received nothing", name)
		}
	}
	select {
	case e := <-other:
		t.Fatalf("review b's subscriber received an event for review a: %v", e)
	default:
	}
}

func TestBrokerCancelClosesChannelAndIsIdempotent(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe("a")
	cancel()
	cancel() // must not panic

	if _, open := <-ch; open {
		t.Fatal("channel should be closed after cancel")
	}
	b.Publish("a", event(review.StatusCompleted)) // no subscribers left; must not panic

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subs) != 0 {
		t.Fatalf("broker still tracks %d reviews after the last subscriber left", len(b.subs))
	}
}

func TestBrokerDropsASlowSubscriberInsteadOfBlocking(t *testing.T) {
	b := NewBroker()
	slow, cancel := b.Subscribe("a")
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer+5; i++ { // nobody reads slow
			b.Publish("a", event(review.StatusAnalyzing))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that is not reading")
	}

	// The buffered events are still readable, then the channel reports closed.
	n := 0
	for range slow {
		n++
	}
	if n != subscriberBuffer {
		t.Fatalf("read %d buffered events, want %d", n, subscriberBuffer)
	}
}

func TestBrokerIsSafeUnderConcurrency(t *testing.T) {
	b := NewBroker()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ch, cancel := b.Subscribe("a")
			defer cancel()
			for range 3 {
				select {
				case <-ch:
				case <-time.After(10 * time.Millisecond):
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 50 {
				b.Publish("a", event(review.StatusAnalyzing))
			}
		}()
	}
	wg.Wait() // the race detector (go test -race) is the real assertion here
}
