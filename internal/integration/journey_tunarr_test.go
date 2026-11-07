package integration_test

import (
	"net/http"
	"testing"
)

// TestJourney_TunarrConnect drives the full-automation onboarding step (§6): saving the
// media-server token wires the media server as *Tunarr's* media source using Loomarr's
// OWN admin token (no Emby user login) and enables + scans the movie/show libraries, so
// the tunarr_library setup check is ready — closing the silent dead-air gap the FE
// would otherwise hit (a wizard all-green while channels play nothing). The explicit
// Connect is the retry and changes nothing once wired. The real MediaSourceConnector
// runs against the injected Tunarr double.
func TestJourney_TunarrConnect(t *testing.T) {
	h := newHarness(t) // library + tunarr configured; the token save auto-wired the source
	admin := h.asAdmin()

	if !h.setupCheckOK(admin, "tunarr_library") {
		t.Fatal("tunarr_library not ready after the media-server token was saved")
	}
	if h.tun.Scans != 2 {
		t.Fatalf("token save scanned %d libraries, want 2 (movies+shows)", h.tun.Scans)
	}

	// Connect: ensure the source + enable/scan the movie + show libraries.
	var tc struct {
		SourceID         string `json:"sourceId"`
		LibrariesEnabled int    `json:"librariesEnabled"`
	}
	decodeBody(t, h.do(http.MethodPost, "/v1/setup/tunarr-connect", "", admin), &tc)
	if tc.SourceID == "" || tc.LibrariesEnabled != 2 {
		t.Fatalf("tunarr-connect: source=%q enabled=%d, want a source + 2 libraries (movies+shows)", tc.SourceID, tc.LibrariesEnabled)
	}

	// It used Loomarr's admin token — no separate Emby user login/credential.
	if h.tun.MediaSourceToken != h.ms.AdminToken {
		t.Errorf("Tunarr media source token = %q, want the media server's admin token", h.tun.MediaSourceToken)
	}

	// Idempotent: connecting a wired source enables/scans nothing new.
	if h.tun.Scans != 2 {
		t.Errorf("connect re-scanned (scans=%d), want 2 — not idempotent", h.tun.Scans)
	}
	if !h.setupCheckOK(admin, "tunarr_library") {
		t.Error("tunarr_library not ready after tunarr-connect")
	}
}
