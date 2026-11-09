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
			it.Countries = []string{"Canada", "Ireland"}
			it.Tagline, it.Trailer = "Keep the lamps lit.", "https://www.youtube.com/watch?v=show9000"
			it.Credits = []LibraryCredit{
				{Name: "Station Lead", Role: "actor", Character: "Keeper", ID: LibraryID{Namespace: "tmdb", Value: "90003202"},
					Photo: "https://img.invalid/lead.jpg"},
				{Name: "Station Lead", Role: "creator", ID: LibraryID{Namespace: "tmdb", Value: "90003202"}},
				{Name: "Pen Name", Role: "writer"},
				{Name: "", Role: "actor"},
			}
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
		if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT COALESCE(series_id, ''), rating, rating_from FROM library_items
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
	if got := names("library_countries"); len(got) != 2 || got[0] != "Canada" || got[1] != "Ireland" {
		t.Fatalf("countries = %v, want Canada, Ireland in order", got)
	}
	var tagline, trailer string
	if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT tagline, trailer FROM library_items WHERE id = ?`),
		string(show)).Scan(&tagline, &trailer); err != nil || tagline != "Keep the lamps lit." ||
		trailer != "https://www.youtube.com/watch?v=show9000" {
		t.Fatalf("tagline, trailer = %q, %q, %v", tagline, trailer, err)
	}
	// One person credited twice is one row, keeping the photo the creator credit lacks; a person
	// with no TMDB id is kept by name; a credit with no name is dropped.
	var credits, people int
	if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT COUNT(*) FROM library_credits WHERE item_id = ? AND
		((position = 0 AND person_id = 'tmdb:90003202' AND role = 'actor' AND character_name = 'Keeper') OR
		 (position = 1 AND person_id = 'tmdb:90003202' AND role = 'creator') OR
		 (position = 2 AND person_id = 'name:Pen Name' AND role = 'writer'))`), string(show)).Scan(&credits); err != nil || credits != 3 {
		t.Fatalf("credits = %d, %v; want the actor, the creator and the writer", credits, err)
	}
	if err := sqlSt.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_people WHERE
		(id = 'tmdb:90003202' AND namespace = 'tmdb' AND external_id = '90003202' AND photo = 'https://img.invalid/lead.jpg') OR
		(id = 'name:Pen Name' AND namespace = '' AND external_id = '' AND name = 'Pen Name')`).Scan(&people); err != nil || people != 2 {
		t.Fatalf("people = %d, %v; want the linked lead with their photo and the unlinked writer", people, err)
	}
	var ids int
	if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(`SELECT COUNT(*) FROM library_ids WHERE item_id = ?`), string(show)).Scan(&ids); err != nil || ids != 2 {
		t.Fatalf("ids = %d, %v; want 2", ids, err)
	}
}

// A title's files, tracks and artwork are rows of their own, replaced whole on each write.
func testLibraryFilesAndTracks(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		t.Skip("reads rows directly")
	}
	at := time.UnixMilli(1_790_000_000_000).UTC()
	video := LibraryStream{Index: 0, Kind: "video", Codec: "hevc", Width: 3840, Height: 1600,
		ColorTransfer: "smpte2084", Resolution: "4K", HDR: "HDR10"}
	audio := LibraryStream{Index: 1, Kind: "audio", Codec: "eac3", Channels: 6, Language: "en",
		LanguageName: "English", AudioFormat: "Dolby Digital Plus", Default: true}
	commentary := audio
	commentary.Index, commentary.Commentary, commentary.Default = 2, true, false
	external := LibraryStream{Index: 3, Kind: "subtitle", Codec: "subrip", Language: "en", Forced: true,
		HearingImpaired: true, ExternalRef: "/m/a.en.forced.sdh.srt"}
	item := LibraryItem{ID: "m1", Kind: "movie", Name: "Film",
		Sources: []LibrarySource{
			{Path: "/m/a.mkv", Container: "matroska", SizeBytes: 9, DurationMs: 1000, Plays: true,
				Streams:  []LibraryStream{video, audio, commentary, external},
				Chapters: []LibraryChapter{{StartMs: 0, EndMs: 500, Title: "One"}, {StartMs: 500, EndMs: -1}},
				Markers:  []LibraryMarker{{StartMs: 100, EndMs: 200, Kind: "commercial"}}},
			{Path: "/m/b.mkv", SizeBytes: 5, Streams: []LibraryStream{audio},
				Trickplay: []LibraryTrickplay{{Ref: "/m/b.bif", Format: "bif"}}},
		},
		Art: []LibraryArt{{Kind: "poster", Ref: "/m/poster.jpg"}, {Kind: "fanart", Ref: "/m/fanart.jpg"},
			{Kind: "season-poster", Season: 0, Ref: "/m/s0.jpg"}, {Kind: "season-poster", Season: 1, Ref: "/m/s1.jpg"}},
	}
	if err := st.SaveLibraryItems(ctx, []LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := sqlSt.db.QueryRowContext(ctx, sqlSt.ph(query), args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM library_sources WHERE item_id = 'm1' AND plays = TRUE AND path = '/m/a.mkv'`); n != 1 {
		t.Fatalf("playing copy rows = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_streams WHERE item_id = 'm1' AND resolution = '4K' AND hdr = 'HDR10'`); n != 1 {
		t.Fatalf("labelled video rows = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_streams WHERE kind = 'audio' AND language = 'en' AND is_default = TRUE`); n != 2 {
		t.Fatalf("English audio rows = %d, want one per file", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_art WHERE item_id = 'm1'`); n != 4 {
		t.Fatalf("art rows = %d, want two plus a poster for each of two seasons", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_streams WHERE item_id = 'm1' AND commentary = TRUE AND stream_index = 2`); n != 1 {
		t.Fatalf("commentary rows = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_streams WHERE external_ref = '/m/a.en.forced.sdh.srt'
		AND hearing_impaired = TRUE AND forced = TRUE AND path = '/m/a.mkv'`); n != 1 {
		t.Fatalf("external subtitle rows = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_chapters WHERE path = '/m/a.mkv' AND
		((position = 0 AND title = 'One' AND end_ms = 500) OR (position = 1 AND end_ms = -1))`); n != 2 {
		t.Fatalf("chapter rows = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_markers WHERE path = '/m/a.mkv' AND kind = 'commercial'
		AND start_ms = 100 AND end_ms = 200`); n != 1 {
		t.Fatalf("marker rows = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_trickplay WHERE item_id = 'm1' AND path = '/m/b.mkv' AND format = 'bif'`); n != 1 {
		t.Fatalf("trickplay rows = %d, want b.mkv's", n)
	}

	// Written again with one file: the other file and its tracks go.
	item.Sources = item.Sources[:1]
	item.Art = nil
	if err := st.SaveLibraryItems(ctx, []LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM library_sources WHERE item_id = 'm1'`); n != 1 {
		t.Fatalf("sources after a rewrite = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_streams WHERE path = '/m/b.mkv'`); n != 0 {
		t.Fatalf("a gone file's tracks = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_trickplay WHERE item_id = 'm1'`); n != 0 {
		t.Fatalf("a gone file's trickplay = %d, want 0", n)
	}
	if n := count(`SELECT COUNT(*) FROM library_art WHERE item_id = 'm1'`); n != 0 {
		t.Fatalf("art after a rewrite = %d, want 0", n)
	}
	item.Sources[0].Chapters, item.Sources[0].Markers = nil, nil
	if err := st.SaveLibraryItems(ctx, []LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM library_chapters`) + count(`SELECT COUNT(*) FROM library_markers`); n != 0 {
		t.Fatalf("chapters and markers after a rewrite without them = %d, want 0", n)
	}
}
