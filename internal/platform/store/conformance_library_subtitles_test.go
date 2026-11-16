package store

import (
	"context"
	"testing"
	"time"
)

// A listed file's subtitle tracks, its own and those beside it, answer only while the scanner's
// read of it is current.
func testLibrarySubtitleTracks(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	item := LibraryItem{ID: "m1", Kind: "movie", Name: "Film", Sources: []LibrarySource{{
		Path: "/media/film/a.mkv", Plays: true, Streams: []LibraryStream{
			{Index: 0, Kind: "video", Codec: "h264"},
			{Index: 1, Kind: "audio", Codec: "ac3", Language: "en"},
			{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en", Title: "English"},
			{Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "fr", Forced: true},
			{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "en", HearingImpaired: true,
				ExternalRef: "/media/film/a.en.sdh.srt"},
		},
	}}}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{{Path: "/media/film/a.mkv", Root: "/media", Size: 9,
		ModifiedNs: 100, Identity: "digest", Kind: "movie", Header: LibraryHeaderRead, SeenAt: at, ItemID: "m1"}}); err != nil {
		t.Fatal(err)
	}

	tracks, ok, err := st.LibrarySubtitleTracks(ctx, "/media/film/a.mkv", 9, 100)
	if err != nil || !ok {
		t.Fatalf("ok = %v, %v; want the file's subtitle tracks", ok, err)
	}
	want := []LibraryStream{
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en", Title: "English"},
		{Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "fr", Forced: true},
		{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "en", HearingImpaired: true,
			ExternalRef: "/media/film/a.en.sdh.srt"},
	}
	if len(tracks) != len(want) {
		t.Fatalf("tracks = %+v, want %+v", tracks, want)
	}
	for i := range want {
		if tracks[i] != want[i] {
			t.Errorf("track %d = %+v, want %+v", i, tracks[i], want[i])
		}
	}
	if _, ok, err := st.LibrarySubtitleTracks(ctx, "/media/film/a.mkv", 9, 101); err != nil || ok {
		t.Errorf("a file touched since: ok = %v, %v; want no answer", ok, err)
	}
}
