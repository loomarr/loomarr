package main

import (
	"strings"
	"testing"
)

func TestNASASearchIsBoundedToVideo(t *testing.T) {
	raw, err := nasaSearchURL(defaultSearchBase, "mission trailer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "media_type=video") || !strings.Contains(raw, "page_size=50") || !strings.Contains(raw, "q=mission+trailer") {
		t.Fatalf("URL = %s", raw)
	}
}

func TestSelectMP4PrefersTrustedMediumDerivative(t *testing.T) {
	raw := []byte(`[
      "http://images-assets.nasa.gov/video/id/id~large.mp4",
      "https://example.com/video/id/id~medium.mp4",
      "http://images-assets.nasa.gov/video/id/id~medium.mp4"
    ]`)
	got, err := selectMP4(raw, defaultAssetHost)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://images-assets.nasa.gov/video/id/id~medium.mp4" {
		t.Fatalf("media = %s", got)
	}
}

func TestDecodeSearchRejectsPartialResultPage(t *testing.T) {
	raw := []byte(`{"collection":{"metadata":{"total_hits":2},"items":[{}]}}`)
	if _, err := decodeSearch(raw); err == nil || !strings.Contains(err.Error(), "returned 1 of 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestTrustedURLRejectsCredentialsAndForeignHosts(t *testing.T) {
	for _, raw := range []string{"https://user@images-assets.nasa.gov/video.mp4", "https://example.com/video.mp4", "ftp://images-assets.nasa.gov/video.mp4"} {
		if got, ok := trustedURL(raw, defaultAssetHost); ok {
			t.Errorf("%s => %s", raw, got)
		}
	}
}

func TestJoinedNonEmptyDoesNotInventCreatorText(t *testing.T) {
	if got := joinedNonEmpty("", " "); got != "" {
		t.Fatalf("creator = %q", got)
	}
	if got := joinedNonEmpty("NASA", "JPL"); got != "NASA; JPL" {
		t.Fatalf("creator = %q", got)
	}
}
