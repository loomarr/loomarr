package library

import (
	"strings"
	"testing"
)

func TestParsePathMap(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want PathMap
	}{
		{"empty", "", nil},
		{"whitespace only", "   \n  ", nil},
		{"single", "/data=>/mnt/media", PathMap{{from: "/data", to: "/mnt/media"}}},
		{
			"comma-separated", "/data=>/mnt, /movies=>/nas/movies",
			PathMap{{from: "/data", to: "/mnt"}, {from: "/movies", to: "/nas/movies"}},
		},
		{
			"newline-separated (textarea)", "/data=>/mnt\n/tv=>/nas/tv",
			PathMap{{from: "/data", to: "/mnt"}, {from: "/tv", to: "/nas/tv"}},
		},
		{"malformed skipped", "/data=>/mnt, garbage, =>/x, /y=>", PathMap{{from: "/data", to: "/mnt"}}},
	}
	for _, c := range cases {
		got := ParsePathMap(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d rules, want %d (%v)", c.name, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: rule %d = %+v, want %+v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

func TestPathMap_Apply(t *testing.T) {
	m := PathMap{{from: "/data", to: "/cifs/fictionalserver"}, {from: "/other", to: "/x"}}

	cases := []struct {
		in     string
		want   string
		wantOK bool
		why    string
	}{
		{"/data/tv/Show/ep.mkv", "/cifs/fictionalserver/tv/Show/ep.mkv", true, "prefix rewrite"},
		{"/data", "/cifs/fictionalserver", true, "exact match"},
		{"/database/x", "", false, "boundary: /data must not match /database"},
		{"/unmapped/y", "", false, "no rule"},
		{"/other/z", "/x/z", true, "second rule"},
	}
	for _, c := range cases {
		got, ok := m.Apply(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("Apply(%q) = (%q,%v), want (%q,%v) — %s", c.in, got, ok, c.want, c.wantOK, c.why)
		}
	}
}

// With no path map, ResolveInput short-circuits to the HTTP stream without even fetching the path.
func TestResolveInput_FallsBackToStreamWithoutMapping(t *testing.T) {
	c := New(Emby, "http://emby:8096", "tok", "dev")
	src := c.ResolveInput(t.Context(), "item1", nil, func(string) bool { return true })
	if src.Kind != InputHTTP {
		t.Fatalf("no path map ⇒ HTTP stream, got kind %v url %q", src.Kind, src.URL)
	}
	if !strings.Contains(src.URL, "/Videos/item1/stream") {
		t.Fatalf("expected the stream URL, got %q", src.URL)
	}
}
