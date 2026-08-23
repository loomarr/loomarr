package provision_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// FuzzKeyFromWebhook exercises the §3 key-derivation identity model over random
// (mediaType, tmdbID, tvdbID) triples. It asserts the invariants that keep
// webhook correlation sound (title.go:52 Title.Key, title.go:70 KeyFromWebhook):
//
//   - PARITY: Title{same ids}.Key() == KeyFromWebhook(same ids) whenever both
//     succeed (the §3 webhook-correlation guarantee — a Grab/Import event must
//     resolve to the Record its Title-based request created).
//   - series-with-TVDB always yields the "series:tvdb:<id>" form.
//   - neither derivation ever panics on any int triple.
//   - an under-identified title (no usable id) errors deterministically rather
//     than returning a partial/ambiguous key ("" is never a "valid" key).
func FuzzKeyFromWebhook(f *testing.F) {
	// Seed with realistic triples mirroring the pinned fixtures / existing
	// table cases: a Radarr movie, a Sonarr series (tvdb preferred), a
	// series that must fall back to tmdb, an under-identified movie, and an
	// invalid media type. mediaType is fuzzed as a byte selector so the
	// engine explores movie/series/garbage uniformly.
	f.Add(uint8(0), 1111867, 0) // movie by tmdb (Radarr "In Flames")
	f.Add(uint8(1), 999, 12471) // series prefers tvdb over tmdb
	f.Add(uint8(1), 555, 0)     // series falls back to tmdb
	f.Add(uint8(0), 0, 0)       // movie, no id -> errors
	f.Add(uint8(1), 0, 0)       // series, no id -> errors
	f.Add(uint8(2), 1, 1)       // garbage media type -> errors
	f.Add(uint8(1), -5, -7)     // negative ids (not > 0) -> errors
	f.Add(uint8(0), 2147483647, 0)

	f.Fuzz(func(t *testing.T, mtSel uint8, tmdbID, tvdbID int) {
		mt := selectMediaType(mtSel)

		title := provision.Title{MediaType: mt, TMDBID: tmdbID, TVDBID: tvdbID}

		// Never panics. Both derivations run over the same identity.
		titleKey, titleErr := title.Key()
		webhookKey, webhookErr := provision.KeyFromWebhook(mt, tmdbID, tvdbID)

		// Determinism: the two paths must agree on whether the title is
		// identifiable at all. KeyFromWebhook is defined as Title.Key over the
		// same ids, so success/failure must be identical, not just the key.
		if (titleErr == nil) != (webhookErr == nil) {
			t.Fatalf("success mismatch for mt=%q tmdb=%d tvdb=%d: title err=%v webhook err=%v",
				mt, tmdbID, tvdbID, titleErr, webhookErr)
		}

		if titleErr != nil {
			// Under-identified / invalid: must error deterministically AND
			// never leak a partial key. An error path returning a non-empty
			// key would silently break correlation.
			if titleKey != "" {
				t.Fatalf("errored derivation returned non-empty key %q (mt=%q tmdb=%d tvdb=%d)",
					titleKey, mt, tmdbID, tvdbID)
			}
			if webhookKey != "" {
				t.Fatalf("errored webhook derivation returned non-empty key %q (mt=%q tmdb=%d tvdb=%d)",
					webhookKey, mt, tmdbID, tvdbID)
			}
			return
		}

		// PARITY: both succeeded — the §3 guarantee.
		if titleKey != webhookKey {
			t.Fatalf("key parity broken for mt=%q tmdb=%d tvdb=%d: title=%q webhook=%q",
				mt, tmdbID, tvdbID, titleKey, webhookKey)
		}

		// A successful key is never empty.
		if titleKey == "" {
			t.Fatalf("nil-error derivation produced empty key (mt=%q tmdb=%d tvdb=%d)", mt, tmdbID, tvdbID)
		}

		// series-with-TVDB must always take the tvdb branch.
		if mt == provision.Series && tvdbID > 0 {
			want := provision.Key("series:tvdb:" + strconv.Itoa(tvdbID))
			if titleKey != want {
				t.Fatalf("series+tvdb did not yield tvdb key: got %q want %q (tmdb=%d tvdb=%d)",
					titleKey, want, tmdbID, tvdbID)
			}
			if !strings.HasPrefix(string(titleKey), "series:tvdb:") {
				t.Fatalf("series+tvdb key missing prefix: %q", titleKey)
			}
			// IsSeries must agree with the media type.
			if !titleKey.IsSeries() {
				t.Fatalf("series key not reported as series: %q", titleKey)
			}
		}

		// Structural cross-check: a successful key always has the documented
		// "<a>:<b>:<id>" shape and the trailing id round-trips to a positive int.
		parts := strings.Split(string(titleKey), ":")
		if len(parts) != 3 {
			t.Fatalf("key %q is not <type>:<source>:<id>", titleKey)
		}
		if parts[1] != "tmdb" && parts[1] != "tvdb" {
			t.Fatalf("key %q has unexpected source segment %q", titleKey, parts[1])
		}
		id, err := strconv.Atoi(parts[2])
		if err != nil || id <= 0 {
			t.Fatalf("key %q has non-positive/non-numeric id segment %q", titleKey, parts[2])
		}

		// IsSeries must be exactly the media-type prefix, never a false positive
		// for movies.
		if mt == provision.Movie && titleKey.IsSeries() {
			t.Fatalf("movie key reported as series: %q", titleKey)
		}
	})
}

// selectMediaType maps a fuzzed byte to a media type, biasing toward the two
// valid types while still exercising the invalid-type error path.
func selectMediaType(sel uint8) provision.MediaType {
	switch sel % 3 {
	case 0:
		return provision.Movie
	case 1:
		return provision.Series
	default:
		return provision.MediaType("album") // invalid: must error
	}
}
