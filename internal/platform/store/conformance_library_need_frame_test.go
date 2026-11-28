package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Only a title waiting in Needs you without a frame asks for one, of its main file; the frame goes
// once that file changes or the title stops waiting, and Needs you reads it and each file's length.
func testLibraryNeedFrame(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	source := func(path string, ms int64, plays bool, revision string) librarystore.LibrarySource {
		return librarystore.LibrarySource{Path: path, DurationMs: ms, Plays: plays, Revision: revision}
	}
	waiting := func(revision string) LibraryItem {
		return LibraryItem{ID: "w1", Kind: "movie", Name: "Clip One", Origin: origin("/media/w1.mkv"),
			Sources: []librarystore.LibrarySource{source("/media/w1.mkv", 1000, true, revision)}}
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		waiting("stat:1:1"),
		{ID: "k1", Kind: "movie", Name: "Kept", Origin: origin("/media/k1.mkv"),
			Sources: []librarystore.LibrarySource{source("/media/k1.mkv", 1000, true, "")}},
		{ID: "u1", Kind: "movie", Name: "Broken", Origin: origin("/media/u1.mkv"),
			Sources: []librarystore.LibrarySource{source("/media/u1.mkv", 1000, false, "")}},
		{ID: "p1", Kind: "movie", Name: "Unread", Origin: origin("/media/p1.mkv"),
			Sources: []librarystore.LibrarySource{source("/media/p1.mkv", 0, true, "")}},
		{ID: "s1", Kind: "series", Name: "Garden Diary", Origin: origin("/media/show")},
		{ID: "e1", Kind: "episode", Name: "One", Sources: []librarystore.LibrarySource{source("/media/show/e1.mkv", 3000, true, "")}},
		{ID: "e2", Kind: "episode", Name: "Two", Sources: []librarystore.LibrarySource{source("/media/show/e2.mkv", 4000, true, "")}},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, f := range [][3]string{
		{"/media/w1.mkv", "w1", ""}, {"/media/k1.mkv", "k1", ""}, {"/media/u1.mkv", "u1", ""}, {"/media/p1.mkv", "p1", ""},
		{"/media/show/e1.mkv", "e1", "s1"}, {"/media/show/e2.mkv", "e2", "s1"},
	} {
		files = append(files, librarystore.LibraryFile{Path: f[0], Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: f[1], SeriesID: f[2]})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{Origin: origin("/media/k1.mkv"), Kind: "movie",
		Kept: true, AnsweredAt: at}); err != nil {
		t.Fatal(err)
	}
	wanted := func() []librarystore.LibraryFrameFile {
		t.Helper()
		got, err := st.LibraryNeedsWithoutFrame(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	show := librarystore.LibraryFrameFile{ItemID: "s1", Path: "/media/show/e1.mkv", DurationMs: 3000}
	movie := librarystore.LibraryFrameFile{ItemID: "w1", Path: "/media/w1.mkv", DurationMs: 1000, Revision: "stat:1:1"}
	if got, want := wanted(), []librarystore.LibraryFrameFile{show, movie}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frames wanted = %+v, want %+v", got, want)
	}
	lengths, err := st.LibraryItemsFiles(ctx, []string{"w1", "s1", "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{lengths["w1"][0].DurationMs, lengths["s1"][0].DurationMs, lengths["s1"][1].DurationMs,
		lengths["p1"][0].DurationMs}; !reflect.DeepEqual(got, []int64{1000, 3000, 4000, 0}) {
		t.Fatalf("file lengths = %v, want 1000, 3000, 4000 and 0 for a header not read", got)
	}

	img := imageAt("w1-frame", at)
	if err := st.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	other := movie
	other.Revision = "stat:9:9"
	for _, c := range []struct {
		frame librarystore.LibraryFrameFile
		want  bool
	}{{other, false}, {movie, true}, {movie, false}} {
		if put, err := st.PutLibraryNeedFrame(ctx, c.frame, img.Hash); err != nil || put != c.want {
			t.Fatalf("put %+v = %v, %v; want %v", c.frame, put, err, c.want)
		}
	}
	frameOf := func(id string) string {
		t.Helper()
		needs, err := st.LibraryNeeds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range needs {
			if n.ID == id {
				if n.Poster != "" {
					t.Fatalf("%s has poster %q; a frame is not a poster", id, n.Poster)
				}
				return n.Frame
			}
		}
		return ""
	}
	if got := frameOf("w1"); got != img.Hash {
		t.Fatalf("Needs you frame = %q, want %q", got, img.Hash)
	}
	if got, want := wanted(), []librarystore.LibraryFrameFile{show}; !reflect.DeepEqual(got, want) {
		t.Fatalf("with a frame = %+v, want %+v", got, want)
	}
	if put, err := st.PutHomeVideoPoster(ctx, movie, img.Hash); err != nil || put {
		t.Fatalf("a waiting title took a home video poster: %v, %v", put, err)
	}

	// The sweep keeps the frame of the file still played, and drops it once the file is replaced.
	if err := st.ClearStaleLibraryFrames(ctx); err != nil || frameOf("w1") != img.Hash {
		t.Fatalf("a frame of the file still played was dropped: %v", err)
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{waiting("stat:2:2")}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.ClearStaleLibraryFrames(ctx); err != nil || frameOf("w1") != "" {
		t.Fatalf("a frame of a replaced file was kept: %v", err)
	}
	movie.Revision = "stat:2:2"
	if got, want := wanted(), []librarystore.LibraryFrameFile{show, movie}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the file changed = %+v, want %+v", got, want)
	}

	// A title that stops waiting is not asked for a frame, and loses the one it has.
	if put, err := st.PutLibraryNeedFrame(ctx, movie, img.Hash); err != nil || !put {
		t.Fatalf("put again = %v, %v", put, err)
	}
	if err := st.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{Origin: origin("/media/w1.mkv"), Kind: "movie",
		Ignored: true, AnsweredAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := st.ClearStaleLibraryFrames(ctx); err != nil {
		t.Fatal(err)
	}
	owned, err := st.ImagesForOwner(ctx, librarystore.LibraryItemImageOwner, "w1")
	if err != nil || len(owned) != 0 {
		t.Fatalf("an ignored title kept %d images, %v", len(owned), err)
	}
	if got, want := wanted(), []librarystore.LibraryFrameFile{show}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the answer = %+v, want %+v", got, want)
	}
	if put, err := st.PutLibraryNeedFrame(ctx, movie, img.Hash); err != nil || put {
		t.Fatalf("put after the answer = %v, %v", put, err)
	}
}
