package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A batch ignore takes only titles waiting in Needs you and a batch restore only ignored ones,
// and a batch with any other id changes nothing.
func testLibraryNeedsBatch(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "m1", Kind: "movie", Name: "First Film", Origin: origin("/media/a.mkv")},
		{ID: "s1", Kind: "series", Name: "Mystery Show", Origin: origin("/media/show")},
		{ID: "m2", Kind: "movie", Name: "Linked Film", Origin: origin("/media/b.mkv")},
		{ID: "e1", Kind: "episode", Name: "Pilot"},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, f := range [][3]string{{"/media/a.mkv", "m1", ""}, {"/media/show/e1.mkv", "e1", "s1"}, {"/media/b.mkv", "m2", ""}} {
		files = append(files, librarystore.LibraryFile{Path: f[0], Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: f[1], SeriesID: f[2]})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	link := librarystore.LibraryAnswer{Origin: origin("/media/b.mkv"), Kind: "movie",
		ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000201"}, AnsweredAt: at}
	if err := st.SaveLibraryAnswer(ctx, link); err != nil {
		t.Fatal(err)
	}
	counts := func(step string, waiting, ignored int) {
		t.Helper()
		needs, err := st.LibraryNeeds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, total, err := st.LibraryIgnored(ctx, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(needs) != waiting || total != ignored {
			t.Fatalf("%s: %d waiting and %d ignored, want %d and %d", step, len(needs), total, waiting, ignored)
		}
	}

	// The good id comes first, so a batch that saved as it went would leave it ignored.
	for _, stale := range []string{"nope", "m2", "e1"} {
		if _, err := st.IgnoreLibraryNeeds(ctx, []string{"m1", stale}, at); !errors.Is(err, librarystore.ErrNotNeeded) {
			t.Fatalf("ignore with %q = %v, want ErrNotNeeded", stale, err)
		}
		counts("after a refused ignore with "+stale, 2, 0)
	}
	got, err := st.IgnoreLibraryNeeds(ctx, []string{"m1", "s1"}, at)
	if err != nil || len(got) != 2 || got[0] != (librarystore.LibraryAnswered{ID: "m1", Origin: origin("/media/a.mkv")}) ||
		got[1] != (librarystore.LibraryAnswered{ID: "s1", Origin: origin("/media/show")}) {
		t.Fatalf("ignore = %+v, %v", got, err)
	}
	counts("after the ignore", 0, 2)
	want := librarystore.LibraryAnswer{Origin: origin("/media/show"), Kind: "series", Ignored: true, AnsweredAt: at,
		By: librarystore.AnsweredByYou}
	if a, _, err := st.LibraryAnswerFor(ctx, want.Origin); err != nil || a != want {
		t.Fatalf("saved ignore = %+v, %v; want %+v", a, err, want)
	}
	if _, err := st.IgnoreLibraryNeeds(ctx, []string{"m1"}, at); !errors.Is(err, librarystore.ErrNotNeeded) {
		t.Fatalf("ignore twice = %v, want ErrNotNeeded", err)
	}

	for _, stale := range []string{"nope", "m2"} {
		if _, err := st.RestoreLibraryIgnored(ctx, []string{"m1", stale}); !errors.Is(err, librarystore.ErrNotIgnored) {
			t.Fatalf("restore with %q = %v, want ErrNotIgnored", stale, err)
		}
		counts("after a refused restore with "+stale, 0, 2)
	}
	if _, ok, err := st.LibraryAnswerFor(ctx, link.Origin); err != nil || !ok {
		t.Fatalf("the link was removed: %v, %v", ok, err)
	}
	if got, err := st.RestoreLibraryIgnored(ctx, []string{"m1", "s1"}); err != nil || len(got) != 2 {
		t.Fatalf("restore = %+v, %v", got, err)
	}
	counts("after the restore", 2, 0)
}
