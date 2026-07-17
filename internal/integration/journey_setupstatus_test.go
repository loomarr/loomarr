package integration_test

import (
	"testing"
)

// TestSetupStatus_FullChecklist pins the completed §13 checklist shape the wizard
// codes against: setup/status returns the connection probes — each with a
// Troubleshooting deep-link (docHref) — AND the webhook handshake check (waiting
// until a Sonarr/Radarr test lands). Before 13.0 it returned only livetv +
// tunarr_library, so a wizard rendering the checklist had nothing for most steps —
// exactly the FE surprise 13.0 closes.
func TestSetupStatus_FullChecklist(t *testing.T) {
	h := newHarness(t)
	admin := h.asAdmin()

	var out struct {
		Checks []struct {
			Name         string            `json:"name"`
			OK           bool              `json:"ok"`
			DocHref      string            `json:"docHref"`
			LastReceived map[string]string `json:"lastReceived"`
		} `json:"checks"`
	}
	h.getJSON("/v1/setup/status", admin, &out)

	type check struct {
		ok      bool
		docHref string
		last    map[string]string
	}
	byName := map[string]check{}
	for _, c := range out.Checks {
		byName[c.Name] = check{c.OK, c.DocHref, c.LastReceived}
	}

	// The connection probes are present, each with a Troubleshooting deep-link so a
	// red check is one click from its fix (§13 "failures never blame").
	for _, name := range []string{"media_server", "tunarr", "llm", "tmdb"} {
		c, ok := byName[name]
		if !ok {
			t.Errorf("checklist missing connection check %q", name)
			continue
		}
		if c.docHref == "" {
			t.Errorf("connection check %q has no docHref (§13 deep-link)", name)
		}
	}

	// The webhook handshake check is present and — with no webhook yet delivered —
	// waiting: not ok, no per-app timestamps. (The wizard shows "listening…".)
	wh, ok := byName["webhook"]
	if !ok {
		t.Fatal("checklist missing the webhook handshake check")
	}
	if wh.ok {
		t.Error("webhook check ok=true before any webhook was received")
	}
	if len(wh.last) != 0 {
		t.Errorf("webhook lastReceived should be empty before first receipt, got %v", wh.last)
	}
}
