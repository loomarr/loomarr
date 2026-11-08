package store

import (
	"context"
	"testing"
	"time"
)

// A listed file's own audio tracks answer only while the scanner's read of it is current.
func testLibraryAudioTracks(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	item := LibraryItem{ID: "m1", Kind: "movie", Name: "Film", Sources: []LibrarySource{{
		Path: "/media/film/a.mkv", Plays: true, Streams: []LibraryStream{
			{Index: 0, Kind: "video", Codec: "h264"},
			{Index: 1, Kind: "audio", Codec: "ac3", Language: "ru", Channels: 6, Default: true},
			{Index: 2, Kind: "audio", Codec: "aac", Language: "en", Channels: 2, Commentary: true},
			{Index: 3, Kind: "audio", Codec: "ac3", Language: "fr", ExternalRef: "/media/film/a.fr.mka"},
		},
	}}}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, header string) LibraryFile {
		return LibraryFile{Path: path, Root: "/media", Size: 9, ModifiedNs: 100, Identity: "digest", Kind: "movie",
			Header: header, SeenAt: at, ItemID: "m1"}
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("/media/film/a.mkv", LibraryHeaderRead), file("/media/film/unlisted.mkv", LibraryHeaderRead),
	}); err != nil {
		t.Fatal(err)
	}

	tracks, ok, err := st.LibraryAudioTracks(ctx, "/media/film/a.mkv", 9, 100)
	if err != nil || !ok || len(tracks) != 2 {
		t.Fatalf("tracks = %+v, %v, %v; want the file's own two audio tracks", tracks, ok, err)
	}
	want := []LibraryStream{
		{Index: 1, Kind: "audio", Codec: "ac3", Language: "ru", Channels: 6, Default: true},
		{Index: 2, Kind: "audio", Codec: "aac", Language: "en", Channels: 2, Commentary: true},
	}
	for i := range want {
		if tracks[i] != want[i] {
			t.Errorf("track %d = %+v, want %+v", i, tracks[i], want[i])
		}
	}
	for name, probe := range map[string]struct {
		path        string
		size, mtime int64
	}{
		"a file changed since it was read": {"/media/film/a.mkv", 10, 100},
		"a file touched since":             {"/media/film/a.mkv", 9, 101},
		"a file no title lists":            {"/media/film/unlisted.mkv", 9, 100},
		"a file the scanner never saw":     {"/media/film/other.mkv", 9, 100},
	} {
		if _, ok, err := st.LibraryAudioTracks(ctx, probe.path, probe.size, probe.mtime); err != nil || ok {
			t.Errorf("%s: ok = %v, %v; want no answer", name, ok, err)
		}
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{file("/media/film/a.mkv", LibraryHeaderPending)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.LibraryAudioTracks(ctx, "/media/film/a.mkv", 9, 100); err != nil || ok {
		t.Errorf("a file whose header waits to be read again: ok = %v, %v; want no answer", ok, err)
	}
}
