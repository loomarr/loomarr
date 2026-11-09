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
	// The file the title plays: its source marked plays, while the file is present.
	if path, ok, err := st.LibraryPlayingFile(ctx, "m1"); err != nil || !ok || path != "/media/film/a.mkv" {
		t.Errorf("playing file = %q, %v, %v; want the listed file", path, ok, err)
	}
	if _, ok, err := st.LibraryPlayingFile(ctx, "90000001"); err != nil || ok {
		t.Errorf("a media-server id: ok = %v, %v; want no answer", ok, err)
	}
	if v, ok, err := st.LibraryPlayingVideo(ctx, "m1"); err != nil || !ok || v.Codec != "h264" || v.Index != 0 {
		t.Errorf("playing video = %+v, %v, %v; want the file's h264 stream", v, ok, err)
	}
	if _, ok, err := st.LibraryPlayingVideo(ctx, "90000001"); err != nil || ok {
		t.Errorf("a media-server id's video: ok = %v, %v; want no answer", ok, err)
	}
	// A movie plays a copy in each edition (decision 54); a channel airs the default one, though
	// an edition's path sorts first.
	cut, plain := "/media/cut/Cut {edition-Directors Cut}.mkv", "/media/cut/Cut.mkv"
	editions := LibraryItem{ID: "m2", Kind: "movie", Name: "Cut", Sources: []LibrarySource{
		{Path: cut, Edition: "Directors Cut", Plays: true, DurationMs: 9_000_000,
			Streams: []LibraryStream{{Index: 0, Kind: "video", Codec: "hevc"}}},
		{Path: plain, Plays: true, DurationMs: 7_200_000, Streams: []LibraryStream{{Index: 0, Kind: "video", Codec: "h264"}}},
	}}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{editions}, at); err != nil {
		t.Fatal(err)
	}
	cutFile, plainFile := file(cut, LibraryHeaderRead), file(plain, LibraryHeaderRead)
	cutFile.ItemID, plainFile.ItemID = "m2", "m2"
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{cutFile, plainFile}); err != nil {
		t.Fatal(err)
	}
	if path, ok, err := st.LibraryPlayingFile(ctx, "m2"); err != nil || !ok || path != plain {
		t.Errorf("playing file of a movie with an edition = %q, %v, %v; want the default %q", path, ok, err, plain)
	}
	if v, ok, err := st.LibraryPlayingVideo(ctx, "m2"); err != nil || !ok || v.Codec != "h264" {
		t.Errorf("playing video of a movie with an edition = %+v, %v, %v; want the default's h264", v, ok, err)
	}
	// Its runtime is the copy that airs, so a schedule leaves no gap after the shorter default.
	if facts, err := st.LibraryItemFacts(ctx, []string{"m2"}); err != nil || facts["m2"].RuntimeMs != 7_200_000 {
		t.Errorf("runtime of a movie with an edition = %+v, %v; want the default's 7200000", facts["m2"], err)
	}
	gone := file("/media/film/a.mkv", LibraryHeaderRead)
	gone.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{gone}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.LibraryPlayingFile(ctx, "m1"); err != nil || ok {
		t.Errorf("a title whose file went missing: ok = %v, %v; want no answer", ok, err)
	}
	if _, ok, err := st.LibraryPlayingVideo(ctx, "m1"); err != nil || ok {
		t.Errorf("the video of a file gone missing: ok = %v, %v; want no answer", ok, err)
	}
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{file("/media/film/a.mkv", LibraryHeaderPending)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.LibraryAudioTracks(ctx, "/media/film/a.mkv", 9, 100); err != nil || ok {
		t.Errorf("a file whose header waits to be read again: ok = %v, %v; want no answer", ok, err)
	}
}
