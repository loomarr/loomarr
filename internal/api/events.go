package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mantonx/loomarr/internal/events"
)

// EventSource is the subscribe surface the SSE handler needs (implemented by
// events.Bus). Nil ⇒ /v1/events is not mounted.
type EventSource interface {
	Subscribe() (<-chan events.Event, func())
}

// eventsHandler streams provisioning + channel + job state changes as SSE
// (§7 /v1/events, §8). Authenticated via the same cookie/token as /v1 (an
// EventSource sends cookies same-origin). Per §8, this is a latency optimization:
// a client that misses events re-reads the GET endpoints (source of truth), so
// the handler makes no delivery guarantees.
func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate like any /v1 route (the Huma middleware doesn't wrap this plain
	// handler, so check here). Anonymous → 401.
	if s.auth != nil && s.auth.Authorize(r) == RoleAnonymous {
		s.writeProblem(w, r, http.StatusUnauthorized, "Not signed in", "You need to sign in to receive live updates.")
		return
	}
	if s.events == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Live updates unavailable", "The live-events stream isn't running right now.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeProblem(w, r, http.StatusInternalServerError, "Live updates unavailable", "Your connection doesn't support streaming updates.")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.events.Subscribe()
	defer unsubscribe()

	// Initial comment so proxies flush headers and the client knows it's live.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case ev, ok := <-ch:
			if !ok {
				return // bus closed
			}
			payload, err := json.Marshal(ev.Payload)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
			flusher.Flush()
		}
	}
}
