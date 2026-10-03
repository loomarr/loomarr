package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/tunarr/tunarrtest"
)

// liveTVURLsFor replaced the deleted setup.LiveTVURLsFor free function (Refs #1564 PR 2); these
// pin the exact byte-for-byte behaviour it inherited: `internal` reads Loomarr's own endpoints,
// anything else (including an unrecognised value) reads the adapter's own Tunarr URLs. The
// behaviour-change flip for an unrecognised value is a later PR's call-site consolidation, not
// this one.
func TestLiveTVURLsFor_InternalReadsLoomarrsOwnURLs(t *testing.T) {
	t.Parallel()
	got := liveTVURLsFor(tunarrtest.NewTunarr(), "internal", "http://loomarr:8080", "tok")
	want := setup.InternalPlayoutURLs("http://loomarr:8080", "tok")
	if got != want {
		t.Errorf("liveTVURLsFor(internal) = %+v, want %+v", got, want)
	}
}

func TestLiveTVURLsFor_NonInternalReadsTheAdapter(t *testing.T) {
	t.Parallel()
	prog := fakeLiveTVAdapter{
		Tunarr: tunarrtest.NewTunarr(),
		urls:   setup.LiveTVURLs{M3U: "http://tunarr/api/channels.m3u", XMLTV: "http://tunarr/api/xmltv.xml"},
	}
	// "tunarr" is the declared value; an empty or unrecognised one falls back to it too — the
	// pre-§9.1 behaviour, unchanged by this PR.
	for _, backend := range []string{"tunarr", "", "something-else"} {
		got := liveTVURLsFor(prog, backend, "http://loomarr:8080", "tok")
		if got != prog.urls {
			t.Errorf("backend=%q: liveTVURLsFor = %+v, want the adapter's URLs %+v", backend, got, prog.urls)
		}
	}
}

// fakeLiveTVAdapter overrides the promoted LiveTVURLs so the test can assert the ADAPTER's
// answer (never the live `tunarr.url` setting) is what a non-internal backend reads.
type fakeLiveTVAdapter struct {
	*tunarrtest.Tunarr
	urls setup.LiveTVURLs
}

func (f fakeLiveTVAdapter) LiveTVURLs() setup.LiveTVURLs { return f.urls }
