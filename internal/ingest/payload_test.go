package ingest

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
)

// The Radarr import fixture parses to a movie key via remoteMovie.tmdbId, and
// its eventType is the "Download" naming quirk (§6).
func TestParseRadarrImport(t *testing.T) {
	p, err := parse(testkit.Fixture(t, "radarr/import_webhook.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.eventType != evDownload {
		t.Errorf("radarr import eventType = %q, want Download (§6 quirk)", p.eventType)
	}
	if !p.isTracked || p.key != provision.Key("movie:tmdb:1111867") {
		t.Errorf("radarr import key = %q tracked=%v, want movie:tmdb:1111867", p.key, p.isTracked)
	}
	if p.downloadID == "" {
		t.Error("radarr import missing downloadId (grab↔import correlation, §6)")
	}
}

// The Sonarr grab fixture parses to a series key via series.tvdbId (Sonarr's
// Grab has no top-level remoteSeries — the id is under series; pinned Phase 0).
func TestParseSonarrGrab(t *testing.T) {
	p, err := parse(testkit.Fixture(t, "sonarr/grab_webhook.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.eventType != evGrab {
		t.Errorf("sonarr grab eventType = %q, want Grab", p.eventType)
	}
	if !p.isTracked || p.key != provision.Key("series:tvdb:360893") {
		t.Errorf("sonarr grab key = %q tracked=%v, want series:tvdb:360893", p.key, p.isTracked)
	}
}

// Test payloads (both apps) are classified as Test — never resolved to a title.
func TestParseTestWebhooks(t *testing.T) {
	for _, f := range []string{"radarr/test_webhook.json", "sonarr/test_webhook.json"} {
		p, err := parse(testkit.Fixture(t, f))
		if err != nil {
			t.Fatal(err)
		}
		if !p.isTest || p.isTracked {
			t.Errorf("%s: isTest=%v isTracked=%v, want test/not-tracked", f, p.isTest, p.isTracked)
		}
		if p.app == "" {
			t.Errorf("%s: Test payload should carry instanceName for the §13 handshake", f)
		}
	}
}
