package scan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// A title's page says where its details came from and what its file holds: the picture, the sound
// that plays (the default audio stream, not the first), each subtitle, and the measured length.
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
	doc := `<movie><title>Echo Feature</title><plot>A story.</plot><mpaa>PG</mpaa>` +
		`<uniqueid type="tmdb" default="true">90000621</uniqueid></movie>`
	if err := os.WriteFile(filepath.Join(h.root, "Echo", "Echo.nfo"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()
	id := string(h.item(fileOrigin(h.root, "Echo/Echo.mkv")).ID)

	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	d, err := lib.Item(context.Background(), id)
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
		!slices.Equal(f.Subtitles, []string{"eng", ""}) {
		t.Errorf("file = %+v (picture %+v)", f, f.Picture)
	}
}
