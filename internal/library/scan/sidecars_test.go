package scan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// An external track's name says its language and flags; a file that only shares the stem's
// prefix, and a sidecar that is not a track, are not tracks.
func TestExternalTracksReadTheirNames(t *testing.T) {
	f := File{Path: "/m/Film (2001)/Film (2001).mkv", Sidecars: map[string]Stat{
		"/m/Film (2001)/Film (2001).en.forced.srt":       {},
		"/m/Film (2001)/Film (2001).eng.sdh.default.ass": {},
		"/m/Film (2001)/Film (2001).fr.commentary.mka":   {},
		"/m/Film (2001)/Film (2001).srt":                 {},
		"/m/Film (2001)/Film (2001).nfo":                 {},
	}}
	got := f.externalTracks()
	want := []externalTrack{
		{Ref: "/m/Film (2001)/Film (2001).en.forced.srt", Kind: "subtitle", Codec: "subrip", Language: "en", Forced: true},
		{Ref: "/m/Film (2001)/Film (2001).eng.sdh.default.ass", Kind: "subtitle", Codec: "ass", Language: "en", Default: true, HearingImpaired: true},
		{Ref: "/m/Film (2001)/Film (2001).fr.commentary.mka", Kind: "audio", Language: "fr", Commentary: true},
		{Ref: "/m/Film (2001)/Film (2001).srt", Kind: "subtitle", Codec: "subrip"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("external tracks =\n%+v\nwant\n%+v", got, want)
	}
}

// An .edl's spans are read in milliseconds with their action; a line that does not parse, names
// no known action or runs backwards is skipped, and a missing file has none.
func TestReadEDL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Film.edl")
	body := "12.5 30 3\n0 1.25\n40 41 1\nnot a line\n50 49 0\n60 61 9\n70 71 2\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	want := []marker{
		{StartMs: 12500, EndMs: 30000, Kind: "commercial"},
		{StartMs: 0, EndMs: 1250, Kind: "cut"},
		{StartMs: 40000, EndMs: 41000, Kind: "mute"},
		{StartMs: 70000, EndMs: 71000, Kind: "scene"},
	}
	if got := readEDL(path); !slices.Equal(got, want) {
		t.Fatalf("markers = %+v, want %+v", got, want)
	}
	if got := readEDL(filepath.Join(t.TempDir(), "none.edl")); got != nil {
		t.Fatalf("missing .edl markers = %+v, want none", got)
	}
}

// A movie's subtitle file and .edl beside it reach its source: the subtitle as a track after the
// file's own, and the .edl's spans as markers. The file's chapters come with its header.
func TestAMovieKeepsItsSidecarTracksMarkersAndChapters(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(context.Context, string) (header.Header, error) {
		return header.Header{Container: "matroska,webm", DurationMillis: 1_320_000, ChaptersKnown: true,
			Chapters: []header.Chapter{{StartMillis: 0, EndMillis: 600_000, Title: "Opening"}, {StartMillis: 600_000, EndMillis: 1_320_000}},
			Streams: []inventory.Stream{
				{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: 1920, Height: 1080},
				{Index: 1, Kind: inventory.StreamAudio, Codec: "aac", Channels: 2, Language: "eng",
					Disposition: inventory.Disposition{Comment: true}},
			}}, nil
	}
	dir := "Quiet Harbor (2003)/"
	h.file(dir+"Quiet Harbor (2003).mkv", 10)
	srt := h.file(dir+"Quiet Harbor (2003).es.sdh.srt", 1)
	edl := h.file(dir+"Quiet Harbor (2003).edl", 0)
	if err := os.WriteFile(edl, []byte("5 65 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()

	sources := librarySources(h.item(fileOrigin(h.root, dir+"Quiet Harbor (2003).mkv")))
	if len(sources) != 1 {
		t.Fatalf("sources = %+v", sources)
	}
	s := sources[0]
	if len(s.Streams) != 3 || !s.Streams[1].Commentary {
		t.Fatalf("streams = %+v, want the file's two (audio commentary) and the subtitle file", s.Streams)
	}
	if got, want := s.Streams[2], (store.LibraryStream{Index: 2, Kind: "subtitle", Codec: "subrip",
		Language: "es", LanguageName: "Spanish", HearingImpaired: true, ExternalRef: srt}); got != want {
		t.Errorf("external stream = %+v, want %+v", got, want)
	}
	if want := []store.LibraryMarker{{StartMs: 5000, EndMs: 65000, Kind: "commercial"}}; !slices.Equal(s.Markers, want) {
		t.Errorf("markers = %+v, want %+v", s.Markers, want)
	}
	if want := []store.LibraryChapter{{StartMs: 0, EndMs: 600_000, Title: "Opening"}, {StartMs: 600_000, EndMs: 1_320_000}}; !slices.Equal(s.Chapters, want) {
		t.Errorf("chapters = %+v, want %+v", s.Chapters, want)
	}
}
