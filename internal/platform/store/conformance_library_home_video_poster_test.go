package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Only a kept title without a poster asks for a frame, of its main file.
func testLibraryHomeVideoPoster(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	source := func(path string, ms int64, part int, transfer string) librarystore.LibrarySource {
		return librarystore.LibrarySource{Path: path, DurationMs: ms, Part: part, Plays: true,
			Streams: []librarystore.LibraryStream{{Kind: "video", Codec: "hevc", ColorTransfer: transfer}}}
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		// The second part's name sorts first; the first part is still the main file.
		{ID: "m1", Kind: "movie", Name: "Beach Day", Origin: origin("/media/m1"), Sources: []librarystore.LibrarySource{
			source("/media/a-second.mkv", 2000, 2, ""), source("/media/b-first.mkv", 1000, 1, "smpte2084")}},
		{ID: "waiting", Kind: "movie", Name: "Not Kept", Origin: origin("/media/w.mkv"),
			Sources: []librarystore.LibrarySource{source("/media/w.mkv", 1000, 0, "")}},
		{ID: "s1", Kind: "series", Name: "Garden Diary", Origin: origin("/media/show")},
		{ID: "e1", Kind: "episode", Name: "One", Sources: []librarystore.LibrarySource{source("/media/show/e1.mkv", 3000, 0, "")}},
		{ID: "e2", Kind: "episode", Name: "Two", Sources: []librarystore.LibrarySource{source("/media/show/e2.mkv", 4000, 0, "")}},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, f := range [][3]string{
		{"/media/b-first.mkv", "m1", ""}, {"/media/a-second.mkv", "m1", ""}, {"/media/w.mkv", "waiting", ""},
		{"/media/show/e1.mkv", "e1", "s1"}, {"/media/show/e2.mkv", "e2", "s1"},
	} {
		files = append(files, librarystore.LibraryFile{Path: f[0], Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: f[1], SeriesID: f[2]})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	for _, a := range []librarystore.LibraryAnswer{
		{Origin: origin("/media/m1"), Kind: "movie", Kept: true, AnsweredAt: at},
		{Origin: origin("/media/show"), Kind: "series", Kept: true, AnsweredAt: at},
	} {
		if err := st.SaveLibraryAnswer(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	frames := func() []librarystore.LibraryHomeVideoFrame {
		got, err := st.HomeVideosWithoutPoster(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	movie := librarystore.LibraryHomeVideoFrame{ItemID: "m1", Path: "/media/b-first.mkv", DurationMs: 1000, HDR: true}
	show := librarystore.LibraryHomeVideoFrame{ItemID: "s1", Path: "/media/show/e1.mkv", DurationMs: 3000}
	if got, want := frames(), []librarystore.LibraryHomeVideoFrame{movie, show}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frames wanted = %+v, want %+v", got, want)
	}

	img := imageAt("m1-frame", at)
	if err := st.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if err := st.PutImageRef(ctx, clipstore.ImageRef{ImageHash: img.Hash, OwnerKind: librarystore.LibraryItemImageOwner,
		OwnerID: "m1", Role: "poster"}); err != nil {
		t.Fatal(err)
	}
	if got, want := frames(), []librarystore.LibraryHomeVideoFrame{show}; !reflect.DeepEqual(got, want) {
		t.Fatalf("with a poster = %+v, want %+v", got, want)
	}
	if err := st.DeleteLibraryAnswer(ctx, origin("/media/show")); err != nil {
		t.Fatal(err)
	}
	if got := frames(); len(got) != 0 {
		t.Fatalf("after taking the answer back = %+v, want none", got)
	}
}
