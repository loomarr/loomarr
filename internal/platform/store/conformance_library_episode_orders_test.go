package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// A show with a present episode its catalog could not number is listed with how many such episodes
// it has and how far each season runs on disk (a multi-episode file through its last episode, a
// missing file not at all); an answer names an order only when it picks one, round-trips, and a
// later answer replaces it (#2151 decision 53).
func testLibraryEpisodeOrders(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	show := LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000701"}
	harbor := LibraryOrigin{Authority: "scan:test", Key: "series:dir:harbor"}
	other := LibraryOrigin{Authority: "scan:test", Key: "series:dir:other"}
	harborID, otherID := inventorySeries(t, st, harbor, at), inventorySeries(t, st, other, at)
	episode := func(id string, season, number, end int, unnumbered bool) LibraryItem {
		return LibraryItem{ID: id, Kind: "episode", Name: id, Series: harbor, Season: season, Episode: number,
			EpisodeEnd: end, Unnumbered: unnumbered}
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{
		{ID: harborID, Kind: "series", Name: "Harbor Show", Origin: harbor, Catalog: show},
		episode("e1", 1, 1, 0, true),
		episode("e2", 1, 2, 3, false),
		episode("e3", 2, 1, 0, true),
		episode("e4", 2, 5, 0, true),
		{ID: otherID, Kind: "series", Name: "Numbered Show", Origin: other},
		{ID: "o1", Kind: "episode", Name: "o1", Series: other, Season: 1, Episode: 1},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(item, series string) LibraryFile {
		return LibraryFile{Path: "/media/" + item + ".mkv", Root: "/media", Size: 1, ModifiedNs: 1, Identity: "digest",
			Kind: "episode", Header: LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	gone := file("e4", harborID)
	gone.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{
		file("e1", harborID), file("e2", harborID), file("e3", harborID), gone, file("o1", otherID),
	}); err != nil {
		t.Fatal(err)
	}

	needs, err := st.LibraryOrderNeeds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 1 || needs[0].ID != harborID || needs[0].Catalog != show || needs[0].Unnumbered != 2 ||
		!reflect.DeepEqual(needs[0].Last, map[int]int{1: 3, 2: 1}) {
		t.Fatalf("order needs = %+v, want Harbor Show with 2 unnumbered episodes, seasons running to 3 and 1", needs)
	}

	if _, ok, err := st.LibraryEpisodeOrderFor(ctx, show); err != nil || ok {
		t.Fatalf("answer before any = %v, %v; want none", ok, err)
	}
	for _, bad := range []LibraryEpisodeOrder{
		{Series: show, Choice: OrderByOrder},
		{Series: show, Choice: OrderByFiles, OrderID: "90000799a"},
		{Series: show, Choice: OrderByCatalog, OrderID: "90000799a"},
	} {
		if err := st.SaveLibraryEpisodeOrder(ctx, bad); !errors.Is(err, ErrBadEpisodeOrder) {
			t.Errorf("save %+v = %v, want ErrBadEpisodeOrder", bad, err)
		}
	}
	if err := st.SaveLibraryEpisodeOrder(ctx, LibraryEpisodeOrder{Series: show, Choice: "aired"}); err == nil {
		t.Error("an unknown choice was saved")
	}
	for _, want := range []LibraryEpisodeOrder{
		{Series: show, Choice: OrderByOrder, OrderID: "90000799a", AnsweredAt: at},
		{Series: show, Choice: OrderByFiles, AnsweredAt: at.Add(time.Minute)},
	} {
		if err := st.SaveLibraryEpisodeOrder(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, ok, err := st.LibraryEpisodeOrderFor(ctx, show)
		if err != nil || !ok || got.Choice != want.Choice || got.OrderID != want.OrderID || !got.AnsweredAt.Equal(want.AnsweredAt) {
			t.Fatalf("answer = %+v, %v, %v; want %+v", got, ok, err, want)
		}
	}
}
