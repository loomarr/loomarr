package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A home video is an answer: kept only for a title that needs the maintainer and has no guess, read
// by members under its file's name, and gone from Needs you until the answer is taken back.
func testLibraryHomeVideo(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/media", Key: key}
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "Beach Day", Origin: origin("/media/a.mkv")},
		{ID: "m2", Kind: "movie", Name: "Guessed Film", Origin: origin("/media/b.mkv")},
	}, at); err != nil {
		t.Fatal(err)
	}
	var files []librarystore.LibraryFile
	for _, f := range [][2]string{{"/media/a.mkv", "m1"}, {"/media/b.mkv", "m2"}} {
		files = append(files, librarystore.LibraryFile{Path: f[0], Root: "/media", Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: f[1]})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryCandidates(ctx, "m2", "guessed film", []librarystore.LibraryCandidate{
		{Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000301"}, Name: "Guessed Film"},
	}); err != nil {
		t.Fatal(err)
	}
	memberIDs := func() []string {
		page, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Member: true})
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, i := range page.Items {
			ids = append(ids, i.ID)
		}
		return ids
	}
	if got := memberIDs(); len(got) != 0 {
		t.Fatalf("a member reads %v before any answer", got)
	}

	for _, id := range []string{"m2", "nope"} {
		if _, err := st.KeepLibraryNeed(ctx, id, at); !errors.Is(err, librarystore.ErrNotKeepable) {
			t.Fatalf("keep %s: err = %v, want ErrNotKeepable", id, err)
		}
	}
	kept, err := st.KeepLibraryNeed(ctx, "m1", at)
	if err != nil || kept.ID != "m1" || kept.Origin != origin("/media/a.mkv") {
		t.Fatalf("keep = %+v, %v", kept, err)
	}
	if _, err := st.KeepLibraryNeed(ctx, "m1", at); !errors.Is(err, librarystore.ErrNotKeepable) {
		t.Fatalf("a second keep: err = %v, want ErrNotKeepable", err)
	}
	want := librarystore.LibraryAnswer{Origin: kept.Origin, Kind: "movie", Kept: true, AnsweredAt: at, By: librarystore.AnsweredByYou}
	if got, ok, err := st.LibraryAnswerFor(ctx, kept.Origin); err != nil || !ok || got != want {
		t.Fatalf("answer = %+v, %v, %v; want %+v", got, ok, err, want)
	}
	if needs, err := st.LibraryNeeds(ctx); err != nil || len(needs) != 1 || needs[0].ID != "m2" {
		t.Fatalf("needs after keeping = %+v, %v; want m2 only", needs, err)
	}
	if got := memberIDs(); len(got) != 1 || got[0] != "m1" {
		t.Fatalf("a member reads %v, want the home video", got)
	}
	// The Matcher cannot replace it.
	if err := st.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{Origin: kept.Origin, Kind: "movie", AnsweredAt: at,
		ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000302"}, By: librarystore.AnsweredBySearch}); err != nil {
		t.Fatal(err)
	}
	if got, _, err := st.LibraryAnswerFor(ctx, kept.Origin); err != nil || got != want {
		t.Fatalf("answer after a search = %+v, %v; want %+v", got, err, want)
	}
	for _, bad := range []librarystore.LibraryAnswer{
		{Origin: kept.Origin, Kind: "movie", AnsweredAt: at, Kept: true, Ignored: true},
		{Origin: kept.Origin, Kind: "movie", AnsweredAt: at, Kept: true, ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000303"}},
		{Origin: kept.Origin, Kind: "movie", AnsweredAt: at, Kept: true, By: librarystore.AnsweredBySearch},
	} {
		if err := st.SaveLibraryAnswer(ctx, bad); err == nil {
			t.Fatalf("saved %+v", bad)
		}
	}
	isKept := func(o librarystore.LibraryOrigin, groups ...librarystore.LibraryID) bool {
		got, err := st.KeptLibraryTitle(ctx, "movie", o, groups)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	group := librarystore.LibraryID{Namespace: "parts", Value: "beach day"}
	if !isKept(kept.Origin) || isKept(origin("/media/b.mkv"), group) {
		t.Fatalf("kept reads: the home video %v, another file %v", isKept(kept.Origin), isKept(origin("/media/b.mkv"), group))
	}
	if _, err := st.IgnoreLibraryNeeds(ctx, []string{"m1"}, at); !errors.Is(err, librarystore.ErrNotNeeded) {
		t.Fatalf("a batch ignore of the home video: err = %v, want ErrNotNeeded", err)
	}
	if err := st.DeleteLibraryAnswer(ctx, kept.Origin); err != nil {
		t.Fatal(err)
	}
	if needs, err := st.LibraryNeeds(ctx); err != nil || len(needs) != 2 || isKept(kept.Origin) {
		t.Fatalf("needs after taking it back = %+v, %v", needs, err)
	}

	// Any other answer the maintainer gives ends the keep.
	if _, err := st.KeepLibraryNeed(ctx, "m1", at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryAnswer(ctx, librarystore.LibraryAnswer{Origin: kept.Origin, Kind: "movie", AnsweredAt: at, Ignored: true}); err != nil {
		t.Fatal(err)
	}
	if got, _, err := st.LibraryAnswerFor(ctx, kept.Origin); err != nil || !got.Ignored || got.Kept || isKept(kept.Origin) {
		t.Fatalf("answer after an ignore = %+v, %v; want the ignore alone", got, err)
	}
	if _, err := st.RestoreLibraryIgnored(ctx, []string{"m1"}); err != nil {
		t.Fatal(err)
	}
	if needs, err := st.LibraryNeeds(ctx); err != nil || len(needs) != 2 {
		t.Fatalf("needs after the restore = %+v, %v", needs, err)
	}
}
