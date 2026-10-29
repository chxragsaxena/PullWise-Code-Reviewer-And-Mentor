package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func (a *API) streamEvents(w http.ResponseWriter, r *http.Request) {
	ctx, _ := a.viewerContext(r)
	stream, err := a.svc.Watch(ctx, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, err) // still a normal JSON error: nothing streamed yet
		return
	}
	defer stream.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // ask nginx-style proxies not to buffer
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	send := func(event review.Event) bool {
		data, err := json.Marshal(toEventDTO(event))
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	if !send(stream.Current) || stream.Current.Status.Terminal() {
		return
	}
	last := stream.Current.Status

	keepAlive := time.NewTicker(a.heartbeat)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return // the client went away
		case <-a.done:
			return // the server is shutting down; the client will reconnect
		case event, open := <-stream.Events:
			if !open {
				return // this subscriber fell behind; reconnecting resumes cleanly
			}
			if event.Status == last {
				continue // subscribing before reading can report a status twice
			}
			last = event.Status
			if !send(event) || event.Status.Terminal() {
				return
			}
		case <-keepAlive.C:
			// A comment line: ignored by EventSource, but keeps the connection warm.
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
