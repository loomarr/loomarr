package tmdb

import "testing"

// A still is built at w300 (#4476), and a still cached at original is moved to w300; any other
// ref, and an image at another size, is left as it is.
func TestStillURLs(t *testing.T) {
	if got, want := StillURL("abc.jpg"), "https://image.tmdb.org/t/p/w300/abc.jpg"; got != want {
		t.Errorf("StillURL = %q, want %q", got, want)
	}
	if got := StillURL(" "); got != "" {
		t.Errorf("StillURL of no path = %q, want none", got)
	}
	for ref, want := range map[string]string{
		"https://image.tmdb.org/t/p/original/abc.jpg": "https://image.tmdb.org/t/p/w300/abc.jpg",
		"https://image.tmdb.org/t/p/w300/abc.jpg":     "https://image.tmdb.org/t/p/w300/abc.jpg",
		"https://image.tmdb.org/t/p/w780/abc.jpg":     "https://image.tmdb.org/t/p/w780/abc.jpg",
		"https://artworks.thetvdb.com/banners/e.jpg":  "https://artworks.thetvdb.com/banners/e.jpg",
		"/media/tv/Show/Show S01E01-thumb.jpg":        "/media/tv/Show/Show S01E01-thumb.jpg",
		"":                                            "",
	} {
		if got := SmallStill(ref); got != want {
			t.Errorf("SmallStill(%q) = %q, want %q", ref, got, want)
		}
	}
}
