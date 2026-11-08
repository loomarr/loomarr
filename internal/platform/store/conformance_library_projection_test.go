package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// The Library's rows follow inventory writes: a snapshot writes its item's row, an origin marked
// unseen projects its item again, and an item the projector does not list keeps the row it had.
func testLibraryProjection(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	// The stub lists an item under its first origin's name, and notes whether that origin is
	// present; an item named "Unlisted" is not listed.
	st.ProjectLibraryWith(func(item inventory.Item) (LibraryItem, bool) {
		o := item.Origins[0]
		if o.Observation.Facts.Name == "Unlisted" {
			return LibraryItem{}, false
		}
		name := o.Observation.Facts.Name
		if !o.MissingAt.IsZero() {
			name += " (gone)"
		}
		return LibraryItem{ID: string(item.ID), Kind: "movie", Name: name,
			Origin: LibraryOrigin{Authority: string(o.Key.Authority), Key: o.Key.ExternalItemID}}, true
	})
	name := func(id inventory.ItemID) string {
		t.Helper()
		it, err := st.LibraryItem(ctx, string(id))
		if err != nil {
			t.Fatalf("library item %s: %v", id, err)
		}
		return it.Name
	}

	snap := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Pilot", at)
	id, err := st.ApplyInventorySnapshot(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	// A title is read with a present file.
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{{Path: "/media/p/p.mkv", Root: "/media", Size: 1, ModifiedNs: 1,
		Identity: "digest", Kind: "movie", Header: LibraryHeaderRead, SeenAt: at, ItemID: string(id)}}); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot" {
		t.Fatalf("after a snapshot, name = %q, want Pilot", got)
	}

	// A write the projector declines leaves the row as it was.
	unlisted := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Unlisted", at.Add(time.Minute))
	if _, err := st.ApplyInventorySnapshot(ctx, unlisted); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot" {
		t.Fatalf("after a declined projection, name = %q, want Pilot kept", got)
	}

	renamed := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Pilot Redux", at.Add(2*time.Minute))
	if _, err := st.ApplyInventorySnapshot(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkInventoryUnseen(ctx, "library-a", at.Add(time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot Redux (gone)" {
		t.Fatalf("after its origin went missing, name = %q, want it projected again", got)
	}
}

// A title's facts land in columns and rows (decisions 40, 45), and an episode is linked to its
// series whichever is written first, taking the show's rating when it has none (decision 42).
func testLibraryTitleFacts(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		t.Skip("reads rows directly")
	}
	at := time.UnixMilli(1_790_000_000_000).UTC()
	showOrigin := LibraryOrigin{Authority: "scan:a", Key: "series:dir:1"}
	// The stub lists a series with a rating, genres and ids, and an episode under that series;
	// an episode named "Rated" has a rating of its own.
	st.ProjectLibraryWith(func(item inventory.Item) (LibraryItem, bool) {
		o := item.Origins[0]
		it := LibraryItem{ID: string(item.ID), Kind: string(item.Kind), Name: o.Observation.Facts.Name,
			Origin: LibraryOrigin{Authority: string(o.Key.Authority), Key: o.Key.ExternalItemID}}
		switch item.Kind {
		case "series":
			it.Rating, it.From.Rating = "TV-PG", "tmdb"
			it.Genres, it.Studios = []string{"Drama", "Comedy", "Drama"}, []string{"Network"}
			it.IDs = []LibraryID{{"tmdb", "90000701"}, {"tvdb", "90000702"}}
		case "episode":
			it.Season, it.Episode, it.Series = 1, 2, showOrigin
			if it.Name == "Rated" {
				it.Rating, it.From.Rating = "TV-14", "nfo"
			}
		}
		return it, true
	})
	snap := func(key, kind, name string) inventory.ItemID {
		t.Helper()
		s := inventorySnapshot("scan:a", key, "", "", name, at)
		s.Kind, s.Sources, s.ExternalIDs = inventory.ItemKind(kind), nil, nil
		id, err := st.ApplyInventorySnapshot(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	row := func(id inventory.ItemID) (seriesID, rating, from string) {
		t.Helper()
		if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT series_id, rating, rating_from FROM library_items
			WHERE id = ?`), string(id)).Scan(&seriesID, &rating, &from); err != nil {
			t.Fatal(err)
		}
		return seriesID, rating, from
	}

	early := snap("file:1", "episode", "Early")
	rated := snap("file:2", "episode", "Rated")
	if sid, rating, _ := row(early); sid != "" || rating != "" {
		t.Fatalf("before its series, episode = %q, %q; want unlinked", sid, rating)
	}
	show := snap(showOrigin.Key, "series", "Show")
	if sid, rating, from := row(early); sid != string(show) || rating != "TV-PG" || from != RatingFromShow {
		t.Fatalf("after its series, episode = %q, %q, %q; want linked with the show's rating", sid, rating, from)
	}
	if sid, rating, from := row(rated); sid != string(show) || rating != "TV-14" || from != "nfo" {
		t.Fatalf("a rated episode = %q, %q, %q; want its own rating kept", sid, rating, from)
	}
	late := snap("file:3", "episode", "Late")
	if sid, rating, from := row(late); sid != string(show) || rating != "TV-PG" || from != RatingFromShow {
		t.Fatalf("an episode after its series = %q, %q, %q; want linked with the show's rating", sid, rating, from)
	}

	names := func(table string) []string {
		t.Helper()
		rows, err := sqlSt.db.QueryContext(ctx, sqlSt.ph(`SELECT name FROM `+table+` WHERE item_id = ? ORDER BY position`), string(show))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := names("library_genres"); len(got) != 2 || got[0] != "Drama" || got[1] != "Comedy" {
		t.Fatalf("genres = %v, want Drama, Comedy once each in order", got)
	}
	if got := names("library_studios"); len(got) != 1 || got[0] != "Network" {
		t.Fatalf("studios = %v", got)
	}
	var ids int
	if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT COUNT(*) FROM library_ids WHERE item_id = ?`), string(show)).Scan(&ids); err != nil || ids != 2 {
		t.Fatalf("ids = %d, %v; want 2", ids, err)
	}
}
