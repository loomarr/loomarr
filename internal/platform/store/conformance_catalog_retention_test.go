package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// The catalog's retention (#2528) deletes what is long gone and nothing a reader can still reach:
// every rule is seeded on both sides, present or missing, inside or outside its window, named or
// not.
func testCatalogRetention(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)
	sqlSt, ok := st.(*sqlStore)
	if !ok {
		t.Skip("needs the SQL store")
	}
	now := time.Unix(1_900_000_000, 0).UTC()
	old, recent := now.AddDate(-1, -2, 0), now.Add(-time.Hour) // older than every window, and inside all
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(query), args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	// Importer reports: inv-kept is present, inv-gone has been missing past the window (its report,
	// source report, source and item go), inv-back went missing an hour ago.
	for _, it := range []struct {
		id      string
		missing any
	}{{"inv-kept", nil}, {"inv-gone", old.Unix()}, {"inv-back", recent.Unix()}} {
		exec(`INSERT INTO inventory_items (id, kind, created_at, updated_at) VALUES (?, 'movie', ?, ?)`,
			it.id, old.Unix(), old.Unix())
		exec(`INSERT INTO inventory_item_origins (authority_id, external_item_id, item_id, observation_json,
			observed_at, last_seen_at, missing_at) VALUES ('scan', ?, ?, '{}', ?, ?, ?)`,
			"x-"+it.id, it.id, old.Unix(), old.Unix(), it.missing)
		exec(`INSERT INTO inventory_sources (id, item_id, kind, revision, created_at, updated_at)
			VALUES (?, ?, 'local_file', 'r', ?, ?)`, "src-"+it.id, it.id, old.Unix(), old.Unix())
		exec(`INSERT INTO inventory_source_origins (authority_id, external_item_id, external_source_id, source_id,
			locator_json, observation_json, observed_at, last_seen_at, missing_at)
			VALUES ('scan', ?, 's', ?, '{}', '{}', ?, ?, ?)`,
			"x-"+it.id, "src-"+it.id, old.Unix(), old.Unix(), it.missing)
	}

	// Files: a present one, one missing past the window, and one whose removal waits for a person.
	for _, f := range []struct {
		path, item string
		missingMs  int64
		held       bool
	}{{"/m/present.mkv", "lib-ep", 0, false}, {"/m/gone.mkv", "", old.UnixMilli(), false}, {"/m/held.mkv", "lib-held", old.UnixMilli(), true}} {
		exec(`INSERT INTO library_files (path, root, size_bytes, modified_ns, seen_at_ms, missing_at_ms, held,
			inventory_item_id) VALUES (?, '/m', 1, 1, ?, ?, ?, ?)`, f.path, old.UnixMilli(), f.missingMs, f.held, f.item)
	}

	// Library rows: only lib-gone has nothing left. inv-kept has its item, lib-held a held file,
	// lib-member a collection, lib-named a title, lib-series an episode, lib-recent is recent.
	for _, it := range []struct {
		id, kind, series, catalog string
		updated                   time.Time
	}{
		{"inv-kept", "movie", "", "tmdb:movie:90000201", old}, {"lib-gone", "movie", "", "", old},
		{"lib-held", "movie", "", "", old}, {"lib-member", "movie", "", "", old}, {"lib-named", "series", "", "", old},
		{"lib-series", "series", "", "", old}, {"lib-ep", "episode", "lib-series", "", old},
		{"lib-recent", "movie", "", "", recent},
	} {
		exec(`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms, catalog, series_id)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, it.id, it.kind, it.id, it.id, it.updated.UnixMilli(), it.catalog, it.series)
	}
	exec(`INSERT INTO library_collections (id, name, created_at_ms, updated_at_ms) VALUES ('c1', 'Kept', 1, 1)`)
	exec(`INSERT INTO library_collection_members (collection_id, inventory_item_id, position) VALUES ('c1', 'lib-member', 0)`)
	exec(`INSERT INTO library_candidates (inventory_item_id, rank, kind, tmdb_id, name) VALUES ('lib-gone', 0, 'movie', '90000220', 'Guess')`)
	exec(`INSERT INTO images (hash, origin, mime) VALUES ('img-gone', 'remote', 'image/png')`)
	exec(`INSERT INTO image_refs (image_hash, owner_kind, owner_id, role) VALUES ('img-gone', ?, 'lib-gone', 'poster')`,
		LibraryItemImageOwner)
	named := sampleRecord("movie:tmdb:90000210", provision.Available, time.Time{})
	named.LibraryID = "lib-named"
	if err := st.UpsertTitle(ctx, named); err != nil {
		t.Fatal(err)
	}

	// Catalog records: named, unnamed and old, wanted and never tried, failing for a year, recent.
	for _, c := range []struct {
		id               string
		fetched, retryAt int64
	}{
		{"90000201", old.UnixMilli(), 0}, {"90000202", old.UnixMilli(), 0}, {"90000203", 0, 0},
		{"90000204", 0, old.UnixMilli()}, {"90000205", recent.UnixMilli(), 0},
	} {
		exec(`INSERT INTO library_catalog (provider, kind, external_id, fetched_at_ms, retry_at_ms)
			VALUES ('tmdb', 'movie', ?, ?, ?)`, c.id, c.fetched, c.retryAt)
	}

	// The media server's episode cache: a title's show stays; nobody's goes once old.
	for _, c := range []struct {
		id string
		at time.Time
	}{{"lib-named", old}, {"emby-gone", old}, {"emby-recent", recent}} {
		exec(`INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at) VALUES (?, '[]', 0, ?)`,
			c.id, c.at.Unix())
	}

	n, err := st.PurgeSpentCatalog(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	// 2 reports + 1 source + 1 item + 1 file + 1 candidate + 1 image ref + 1 library row
	// + 2 catalog records + 1 episode list.
	if n != 11 {
		t.Errorf("purged %d rows, want 11", n)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM inventory_items`:                                               2,
		`SELECT count(*) FROM inventory_items WHERE id = 'inv-gone'`:                         0,
		`SELECT count(*) FROM inventory_item_origins`:                                        2,
		`SELECT count(*) FROM inventory_sources`:                                             2,
		`SELECT count(*) FROM inventory_source_origins`:                                      2,
		`SELECT count(*) FROM library_files`:                                                 2,
		`SELECT count(*) FROM library_files WHERE path = '/m/gone.mkv'`:                      0,
		`SELECT count(*) FROM library_items`:                                                 7,
		`SELECT count(*) FROM library_items WHERE id = 'lib-gone'`:                           0,
		`SELECT count(*) FROM library_candidates`:                                            0,
		`SELECT count(*) FROM image_refs`:                                                    0,
		`SELECT count(*) FROM library_catalog`:                                               3,
		`SELECT count(*) FROM library_catalog WHERE external_id IN ('90000202', '90000204')`: 0,
		`SELECT count(*) FROM series_episodes`:                                               2,
		`SELECT count(*) FROM series_episodes WHERE library_id = 'emby-gone'`:                0,
	} {
		var got int
		if err := sqlSt.db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}
