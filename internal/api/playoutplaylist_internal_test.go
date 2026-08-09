package api

import "testing"

func TestRewritePlaylistAuth(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-VERSION:6\n#EXTINF:4.0,\nseg-0.ts\n#EXTINF:4.0,\nseg-1.ts\n"
	got := string(rewritePlaylistAuth([]byte(in), "sig=abc123"))

	// Segment URIs get the auth query; tag lines and blanks are untouched.
	want := "#EXTM3U\n#EXT-X-VERSION:6\n#EXTINF:4.0,\nseg-0.ts?sig=abc123\n#EXTINF:4.0,\nseg-1.ts?sig=abc123\n"
	if got != want {
		t.Fatalf("rewrite:\n got %q\nwant %q", got, want)
	}
}

func TestRewritePlaylistAuth_EmptyQueryIsNoop(t *testing.T) {
	in := []byte("#EXTM3U\nseg-0.ts\n")
	if got := rewritePlaylistAuth(in, ""); string(got) != string(in) {
		t.Fatalf("empty query must not modify the playlist, got %q", got)
	}
}

func TestRewritePlaylistAuth_ExistingQueryUsesAmpersand(t *testing.T) {
	// A URI that already carries a query (not the case today, but must not double the `?`).
	got := string(rewritePlaylistAuth([]byte("seg-0.ts?x=1\n"), "sig=abc"))
	if got != "seg-0.ts?x=1&sig=abc\n" {
		t.Fatalf("got %q", got)
	}
}

// ⚠ The fMP4 init segment rides an #EXT-X-MAP TAG, not a bare URI line — but its URI is still a fetch
// that must self-authenticate, or the init segment 404s and an HEVC stream black-screens (§9.1 V48).
// The rewrite must reach INTO the quoted URI while leaving the rest of the tag intact.
func TestRewritePlaylistAuth_RewritesFmp4InitMap(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.0,\nseg-0.m4s\n"
	got := string(rewritePlaylistAuth([]byte(in), "sig=abc123"))
	want := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MAP:URI=\"init.mp4?sig=abc123\"\n#EXTINF:4.0,\nseg-0.m4s?sig=abc123\n"
	if got != want {
		t.Fatalf("fMP4 init-map rewrite:\n got %q\nwant %q", got, want)
	}
}
