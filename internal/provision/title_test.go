package provision_test

import (
	"encoding/json"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestTitleKey(t *testing.T) {
	tests := []struct {
		name  string
		title provision.Title
		want  provision.Key
		err   bool
	}{
		{"movie by tmdb", provision.Title{MediaType: provision.Movie, TMDBID: 1111867}, "movie:tmdb:1111867", false},
		{"series prefers tvdb", provision.Title{MediaType: provision.Series, TVDBID: 12471, TMDBID: 999}, "series:tvdb:12471", false},
		{"series falls back to tmdb", provision.Title{MediaType: provision.Series, TMDBID: 555}, "series:tmdb:555", false},
		{"movie without id errors", provision.Title{MediaType: provision.Movie}, "", true},
		{"invalid media type errors", provision.Title{MediaType: "album", TMDBID: 1}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.title.Key()
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got key %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Key() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ParseKey is the inverse of Title.Key — it recovers the ids a rating lookup needs
// from an approved lineup entry that stores only the Key (§389). Round-trips must hold.
func TestParseKey(t *testing.T) {
	tests := []struct {
		key      provision.Key
		mt       provision.MediaType
		provider string
		id       int
		ok       bool
	}{
		{"movie:tmdb:603", provision.Movie, "tmdb", 603, true},
		{"series:tvdb:71663", provision.Series, "tvdb", 71663, true},
		{"series:tmdb:4229", provision.Series, "tmdb", 4229, true},
		{"movie:imdb:tt1", "", "", 0, false}, // unknown provider
		{"album:tmdb:1", "", "", 0, false},   // invalid media type
		{"movie:tmdb:0", "", "", 0, false},   // non-positive id
		{"movie:tmdb:x", "", "", 0, false},   // non-numeric id
		{"movie:tmdb", "", "", 0, false},     // too few parts
		{"", "", "", 0, false},               // empty
	}
	for _, tc := range tests {
		mt, provider, id, ok := provision.ParseKey(tc.key)
		if ok != tc.ok || mt != tc.mt || provider != tc.provider || id != tc.id {
			t.Errorf("ParseKey(%q) = (%q,%q,%d,%v), want (%q,%q,%d,%v)",
				tc.key, mt, provider, id, ok, tc.mt, tc.provider, tc.id, tc.ok)
		}
	}
	// Round-trip: every Key the domain builds must parse back to its ids.
	for _, ti := range []provision.Title{
		{MediaType: provision.Movie, TMDBID: 603},
		{MediaType: provision.Series, TVDBID: 71663},
		{MediaType: provision.Series, TMDBID: 4229},
	} {
		k, err := ti.Key()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, ok := provision.ParseKey(k); !ok {
			t.Errorf("Key %q from %+v did not parse back", k, ti)
		}
	}
}

// TestWebhookKeyParity is the §21 phase-2 gate test: a key derived from a Title
// must equal the key derived from the identity a real webhook carries. Uses the
// pinned Phase-0 Radarr import fixture (remoteMovie.tmdbId).
func TestWebhookKeyParity(t *testing.T) {
	// The request side: a Title we would have created for this movie.
	titleKey, err := provision.Title{MediaType: provision.Movie, TMDBID: 1111867, Name: "In Flames"}.Key()
	if err != nil {
		t.Fatal(err)
	}

	// The webhook side: parse the real Radarr import payload the way Phase 6 will.
	var wh struct {
		RemoteMovie struct {
			TMDBID int `json:"tmdbId"`
		} `json:"remoteMovie"`
	}
	if err := json.Unmarshal(testkit.Fixture(t, "radarr/import_webhook.json"), &wh); err != nil {
		t.Fatal(err)
	}
	if wh.RemoteMovie.TMDBID == 0 {
		t.Fatal("fixture missing remoteMovie.tmdbId — webhook correlation would break")
	}

	webhookKey, err := provision.KeyFromWebhook(provision.Movie, wh.RemoteMovie.TMDBID, 0)
	if err != nil {
		t.Fatal(err)
	}

	if titleKey != webhookKey {
		t.Errorf("key parity broken: title=%q webhook=%q", titleKey, webhookKey)
	}
}
