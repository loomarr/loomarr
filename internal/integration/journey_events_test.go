package integration_test

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestJourney_EventsSSE proves the FE's live-update channel actually DELIVERS: an
// authenticated subscriber to /v1/events receives a frame when the backend
// publishes one. The prior coverage only asserted the anonymous-401 case — but the
// model-download bar and live title-state updates ride entirely on this socket, so
// we drive it end to end through the real composition (bus → SSE handler).
func TestJourney_EventsSSE(t *testing.T) {
	h := newHarness(t)
	admin := h.asAdmin()

	// Subscribe FIRST — the bus only delivers to currently-connected subscribers.
	// Bound the whole read with a context deadline so a miss fails fast, never hangs.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.URL+"/v1/events", nil)
	req.AddCookie(admin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open /v1/events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/events → %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("/v1/events Content-Type = %q, want text/event-stream", ct)
	}

	// Now publish: a model pull streams llm_pull frames onto the bus (the download bar).
	go func() { _ = h.status(http.MethodPost, "/v1/system/llm/pull", `{"model":"qwen3:8b"}`, admin) }()

	// Read frames until an llm_pull event arrives or the deadline cancels the read.
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "llm_pull") {
			return // delivered end to end — the FE's live channel works
		}
	}
	t.Fatal("no llm_pull frame arrived on /v1/events before the deadline")
}
