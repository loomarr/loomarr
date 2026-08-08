package setup_test

import (
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/setup"
)

// THE GAP THIS CLOSES: the Live TV wiring built Tunarr's URLs unconditionally, while
// `playout.backend` defaults to `internal` — so the media server was pointed at a backend that
// was not serving those channels. Reported symptom: channels appear in Emby's guide and refuse
// to play, and a `livetv-reconnect` "fixes" it by re-registering the same wrong URLs.
// design.md §9.1 item 3 called for this and the code never did it.
func TestLiveTVURLsFor_InternalPointsAtLoomarr(t *testing.T) {
	got := setup.LiveTVURLsFor("internal", "http://tunarr:8000", "http://loomarr:8080", "tok123")

	if !strings.HasPrefix(got.M3U, "http://loomarr:8080/v1/playout/tuner.m3u") {
		t.Errorf("M3U = %q, want Loomarr's own tuner endpoint", got.M3U)
	}
	if !strings.HasPrefix(got.XMLTV, "http://loomarr:8080/v1/playout/guide.xml") {
		t.Errorf("XMLTV = %q, want Loomarr's own guide endpoint", got.XMLTV)
	}
	// The media server fetches these unauthenticated from a background job, so the device
	// token must ride the URL (§11 — it authenticates a DEVICE, not a person).
	if !strings.Contains(got.M3U, "token=tok123") || !strings.Contains(got.XMLTV, "token=tok123") {
		t.Errorf("both URLs must carry the device token: %q / %q", got.M3U, got.XMLTV)
	}
}

// `tunarr` keeps the pre-§9.1 behaviour exactly — an install streaming through Tunarr must be
// unaffected by this change.
func TestLiveTVURLsFor_TunarrIsUnchanged(t *testing.T) {
	got := setup.LiveTVURLsFor("tunarr", "http://tunarr:8000", "http://loomarr:8080", "tok123")

	if got.M3U != "http://tunarr:8000/api/channels.m3u" {
		t.Errorf("M3U = %q, want Tunarr's playlist", got.M3U)
	}
	if got.XMLTV != "http://tunarr:8000/api/xmltv.xml" {
		t.Errorf("XMLTV = %q, want Tunarr's guide", got.XMLTV)
	}
}

// An unrecognised backend falls back to TUNARR, not to internal: that is the pre-§9.1
// behaviour, so a typo or a future value degrades to what installs already run rather than
// silently retargeting a working media server.
func TestLiveTVURLsFor_UnknownBackendFallsBackToTunarr(t *testing.T) {
	got := setup.LiveTVURLsFor("", "http://tunarr:8000", "http://loomarr:8080", "tok")
	if got.M3U != "http://tunarr:8000/api/channels.m3u" {
		t.Errorf("empty backend should fall back to Tunarr; got %q", got.M3U)
	}
	got = setup.LiveTVURLsFor("something-new", "http://tunarr:8000", "http://loomarr:8080", "tok")
	if got.M3U != "http://tunarr:8000/api/channels.m3u" {
		t.Errorf("unknown backend should fall back to Tunarr; got %q", got.M3U)
	}
}

// ⚠ No public URL ⇒ NO URLs, never a relative path. The media server resolves the URL from its
// OWN host, so registering a relative or empty base silently points it at itself — which looks
// wired and never plays. The caller treats empty as "not wireable".
func TestInternalPlayoutURLs_NoPublicURLYieldsNothing(t *testing.T) {
	got := setup.LiveTVURLsFor("internal", "http://tunarr:8000", "", "tok")
	if got.M3U != "" || got.XMLTV != "" {
		t.Fatalf("a blank public URL must yield no URLs; got %q / %q", got.M3U, got.XMLTV)
	}
	got = setup.LiveTVURLsFor("internal", "http://tunarr:8000", "   ", "tok")
	if got.M3U != "" || got.XMLTV != "" {
		t.Fatalf("a whitespace public URL must yield no URLs; got %q / %q", got.M3U, got.XMLTV)
	}
}

// A trailing slash on the public URL must not produce a double slash in the path — the media
// server treats the two as different URLs, which would defeat the idempotent
// "already registered?" check and re-add a duplicate tuner on every connect.
func TestInternalPlayoutURLs_TrailingSlashIsNormalised(t *testing.T) {
	got := setup.LiveTVURLsFor("internal", "", "http://loomarr:8080/", "tok")
	if strings.Contains(got.M3U, "//playout") {
		t.Errorf("double slash in %q", got.M3U)
	}
}
