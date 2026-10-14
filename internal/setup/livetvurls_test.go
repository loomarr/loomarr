package setup_test

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/setup"
)

// THE GAP THIS CLOSES: the Live TV wiring built Tunarr's URLs unconditionally, while
// `playout.backend` defaults to `internal` — so the media server was pointed at a backend that
// was not serving those channels. Reported symptom: channels appear in Emby's guide and refuse
// to play, and a `livetv-reconnect` "fixes" it by re-registering the same wrong URLs.
// design.md §9.1 item 3 called for this and the code never did it.
func TestInternalPlayoutURLs_PointsAtLoomarr(t *testing.T) {
	got := setup.InternalPlayoutURLs("http://loomarr:8080", "tok123")

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

// ⚠ No public URL ⇒ NO URLs, never a relative path. The media server resolves the URL from its
// OWN host, so registering a relative or empty base silently points it at itself — which looks
// wired and never plays. The caller treats empty as "not wireable".
func TestInternalPlayoutURLs_NoPublicURLYieldsNothing(t *testing.T) {
	got := setup.InternalPlayoutURLs("", "tok")
	if got.M3U != "" || got.XMLTV != "" {
		t.Fatalf("a blank public URL must yield no URLs; got %q / %q", got.M3U, got.XMLTV)
	}
	got = setup.InternalPlayoutURLs("   ", "tok")
	if got.M3U != "" || got.XMLTV != "" {
		t.Fatalf("a whitespace public URL must yield no URLs; got %q / %q", got.M3U, got.XMLTV)
	}
}

// A trailing slash on the public URL must not produce a double slash in the path — the media
// server treats the two as different URLs, which would defeat the idempotent
// "already registered?" check and re-add a duplicate tuner on every connect.
func TestInternalPlayoutURLs_TrailingSlashIsNormalised(t *testing.T) {
	got := setup.InternalPlayoutURLs("http://loomarr:8080/", "tok")
	if strings.Contains(got.M3U, "//playout") {
		t.Errorf("double slash in %q", got.M3U)
	}
}
