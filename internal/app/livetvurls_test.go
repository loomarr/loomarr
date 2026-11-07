package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/playout/livetvpublish"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
)

// liveTVURLsFor: `tunarr` reads the adapter's own URLs; everything else, including an
// unrecognised value, reads Loomarr's own internal URLs and logs a warning — internal is the
// default backend and Tunarr is optional, so a stale/corrupted setting must not silently serve
// a backend the household doesn't run.
func TestLiveTVURLsFor_TunarrReadsTheAdapter(t *testing.T) {
	t.Parallel()
	prog := fakeLiveTVAdapter{
		Tunarr: tunarrtest.NewTunarr(),
		urls:   tunarr.LiveTVURLs{M3U: "http://tunarr/api/channels.m3u", XMLTV: "http://tunarr/api/xmltv.xml"},
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	got := liveTVURLsFor(prog, "tunarr", "http://loomarr:8080", "tok", log)
	if got != prog.urls {
		t.Errorf("liveTVURLsFor(tunarr) = %+v, want the adapter's URLs %+v", got, prog.urls)
	}
	if buf.Len() != 0 {
		t.Errorf("liveTVURLsFor(tunarr) logged unexpectedly: %s", buf.String())
	}
}

func TestLiveTVURLsFor_UnrecognisedFallsBackToInternalAndWarns(t *testing.T) {
	t.Parallel()
	prog := fakeLiveTVAdapter{
		Tunarr: tunarrtest.NewTunarr(),
		urls:   tunarr.LiveTVURLs{M3U: "http://tunarr/api/channels.m3u", XMLTV: "http://tunarr/api/xmltv.xml"},
	}
	want := livetvpublish.InternalPlayoutURLs("http://loomarr:8080", "tok")
	for _, backend := range []string{"internal", "", "something-else"} {
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		got := liveTVURLsFor(prog, backend, "http://loomarr:8080", "tok", log)
		if got != want {
			t.Errorf("backend=%q: liveTVURLsFor = %+v, want Loomarr's own URLs %+v", backend, got, want)
		}
		if backend == "internal" {
			if buf.Len() != 0 {
				t.Errorf("backend=%q: liveTVURLsFor logged unexpectedly: %s", backend, buf.String())
			}
			continue
		}
		if !strings.Contains(buf.String(), "unrecognised") || !strings.Contains(buf.String(), backend) {
			t.Errorf("backend=%q: liveTVURLsFor did not warn naming the value, got log: %s", backend, buf.String())
		}
	}
}

// fakeLiveTVAdapter overrides the promoted LiveTVURLs so the test can assert the ADAPTER's
// answer (never the live `tunarr.url` setting) is what a non-internal backend reads.
type fakeLiveTVAdapter struct {
	*tunarrtest.Tunarr
	urls tunarr.LiveTVURLs
}

func (f fakeLiveTVAdapter) LiveTVURLs() tunarr.LiveTVURLs { return f.urls }
