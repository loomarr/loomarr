package scan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/media/header"
)

// A title's page says where its details came from and what its file holds: the picture, the sound
// that plays (the default audio stream, not the first), each subtitle's language, two-letter where
// one exists (the file's own, then a subtitle file beside it), and the measured length.
func TestTitlePageSaysWhereDetailsCameFromAndWhatTheFileHolds(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(context.Context, string) (header.Header, error) {
		return header.Header{Container: "matroska,webm", DurationMillis: 5_880_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "hevc", Width: 3840, Height: 2160, HDR: true},
			{Index: 1, Kind: inventory.StreamAudio, Codec: "aac", Channels: 2, Language: "eng"},
			{Index: 2, Kind: inventory.StreamAudio, Codec: "eac3", Channels: 6, Language: "eng",
				Disposition: inventory.Disposition{Default: true}},
			{Index: 3, Kind: inventory.StreamSubtitle, Codec: "subrip", Language: "eng"},
			{Index: 4, Kind: inventory.StreamSubtitle, Codec: "subrip"},
		}}, nil
	}
	path := h.file("Echo/Echo.mkv", 10)
	h.file("Echo/Echo.es.srt", 1)
	doc := `<movie><title>Echo Feature</title><plot>A story.</plot><mpaa>PG</mpaa>` +
		`<uniqueid type="tmdb" default="true">90000621</uniqueid></movie>`
	if err := os.WriteFile(filepath.Join(h.root, "Echo", "Echo.nfo"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()
	id := string(h.item(fileOrigin(h.root, "Echo/Echo.mkv")).ID)

	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	d, err := lib.Item(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := (DetailsFrom{Name: mp.IDNFO, Description: mp.IDNFO, Rating: mp.IDNFO}); d.From != want {
		t.Errorf("from = %+v, want %+v", d.From, want)
	}
	if len(d.Files) != 1 {
		t.Fatalf("files = %+v", d.Files)
	}
	f := d.Files[0]
	if f.Path != path || f.LengthMillis != 5_880_000 || f.AudioChannels != 6 ||
		f.Picture == nil || *f.Picture != (Picture{Width: 3840, Height: 2160, HDR: true}) ||
		!slices.Equal(f.Subtitles, []string{"en", "", "es"}) {
		t.Errorf("file = %+v (picture %+v)", f, f.Picture)
	}
}

// The copy that plays is a file whose source row plays: the default edition's before a labelled
// one, a whole file or the first part before a later part. A file with no source row, or one that
// never plays, is never it, and a title with none playing has none.
func TestPlayingCopyIsTheDefaultEditionsFirstPlayingFile(t *testing.T) {
	file := func(name, edition string, part int, sourced, plays bool) ItemFile {
		f := ItemFile{Edition: edition, Part: part, sourced: sourced, plays: plays}
		f.Path = name
		return f
	}
	for _, tc := range []struct {
		name  string
		files []ItemFile
		want  string
	}{
		{"extra copy first", []ItemFile{file("extra", "", 0, true, false), file("plays", "", 0, true, true)}, "plays"},
		{"labelled edition first", []ItemFile{file("extended", "Extended", 0, true, true), file("theatrical", "", 0, true, true)}, "theatrical"},
		{"later part first", []ItemFile{file("part2", "", 2, true, true), file("part1", "", 1, true, true)}, "part1"},
		{"only a labelled edition", []ItemFile{file("unsourced", "", 0, false, false), file("extended", "Extended", 0, true, true)}, "extended"},
		{"none plays", []ItemFile{file("unsourced", "", 0, false, false), file("extra", "", 0, true, false)}, ""},
		{"a series", nil, ""},
	} {
		got, ok := ItemDetail{Files: tc.files}.PlayingCopy()
		if ok != (tc.want != "") || got.Path != tc.want {
			t.Errorf("%s: playing copy = %q, %v; want %q", tc.name, got.Path, ok, tc.want)
		}
	}
}
