package tunarr_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

func TestLiveTVURLsAt(t *testing.T) {
	t.Parallel()
	urls := tunarr.LiveTVURLsAt("http://tunarr:8000/")
	if urls.M3U != "http://tunarr:8000/api/channels.m3u" {
		t.Errorf("M3U = %q", urls.M3U)
	}
	if urls.XMLTV != "http://tunarr:8000/api/xmltv.xml" {
		t.Errorf("XMLTV = %q", urls.XMLTV)
	}
}

// The adapter's pair follows the live-configured base URL, not the one it was built with.
func TestLiveTVURLsFollowsConfiguredBaseURL(t *testing.T) {
	t.Parallel()
	base := "http://old-tunarr:8000"
	tn := tunarr.NewDynamic(func() tunarr.Config { return tunarr.Config{BaseURL: base} })
	base = "http://new-tunarr:8000/"
	if got, want := tn.LiveTVURLs(), tunarr.LiveTVURLsAt("http://new-tunarr:8000"); got != want {
		t.Errorf("LiveTVURLs() = %+v, want %+v", got, want)
	}
}
