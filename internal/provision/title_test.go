package provision_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestTitleKey(t *testing.T) {
	tests := []struct {
		name  string
		title provision.Title
		want  provision.Key
		err   bool
	}{
		{"movie by tmdb", provision.Title{MediaType: provision.Movie, TMDBID: 91111867}, "movie:tmdb:91111867", false},
		{"series prefers tvdb", provision.Title{MediaType: provision.Series, TVDBID: 90012471, TMDBID: 90000999}, "series:tvdb:90012471", false},
		{"series falls back to tmdb", provision.Title{MediaType: provision.Series, TMDBID: 90000555}, "series:tmdb:90000555", false},
		{"movie without id errors", provision.Title{MediaType: provision.Movie}, "", true},
		{"invalid media type errors", provision.Title{MediaType: "album", TMDBID: 90000001}, "", true},
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
		{"movie:tmdb:90000603", provision.Movie, "tmdb", 90000603, true},
		{"series:tvdb:90071663", provision.Series, "tvdb", 90071663, true},
		{"series:tmdb:90004229", provision.Series, "tmdb", 90004229, true},
		{"movie:imdb:tt1", "", "", 0, false},      // unknown provider
		{"album:tmdb:90000001", "", "", 0, false}, // invalid media type
		{"movie:tmdb:0", "", "", 0, false},        // non-positive id
		{"movie:tmdb:x", "", "", 0, false},        // non-numeric id
		{"movie:tmdb", "", "", 0, false},          // too few parts
		{"", "", "", 0, false},                    // empty
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
		{MediaType: provision.Movie, TMDBID: 90000603},
		{MediaType: provision.Series, TVDBID: 90071663},
		{MediaType: provision.Series, TMDBID: 90004229},
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
