package integration_test

import (
	"net/http"
	"testing"
)

// TestJourney_TunarrConnect drives the full-automation onboarding step (§6): Loomarr
// wires the media server as *Tunarr's* media source using its OWN admin token (no
// Emby user login), enables + scans the movie/show libraries, and the
// tunarr_library setup check flips ready — closing the silent dead-air gap the FE
// would otherwise hit (a wizard all-green while channels play nothing). The real
// MediaSourceConnector runs against the injected Tunarr double.
func TestJourney_TunarrConnect(t *testing.T) {
	h := newHarness(t) // library + tunarr configured
	admin := h.asAdmin()

	// Before connect: the tunarr_library check reports not-ready.
	if h.setupCheckOK(admin, "tunarr_library") {
		t.Fatal("tunarr_library reported ready before connect")
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
	if h.tun.MediaSourceToken == "" {
		t.Error("Tunarr media source wired without a token")
	}

	// After connect: the check passes — Tunarr can see the library, no dead-air.
	if !h.setupCheckOK(admin, "tunarr_library") {
		t.Error("tunarr_library still not-ready after tunarr-connect")
	}

	// Idempotent: a second connect enables/scans nothing new.
	decodeBody(t, h.do(http.MethodPost, "/v1/setup/tunarr-connect", "", admin), &tc)
	if h.tun.Scans != 2 {
		t.Errorf("re-connect re-scanned (scans=%d), want 2 — not idempotent", h.tun.Scans)
	}
}
