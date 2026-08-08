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
