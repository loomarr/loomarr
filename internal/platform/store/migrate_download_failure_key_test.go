package store

import (
	"context"
	"database/sql"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"slices"
	"testing"
)

// downloadFailureKeyMigration rewrites the NUL separators in a download failure's key to the unit
// separator, which Postgres text can hold.
const downloadFailureKeyMigration = 249

// A row stored with NUL separators comes out with unit separators and is otherwise unchanged; a
// key with no NUL is left alone.
func TestDownloadFailureKeyMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, downloadFailureKeyMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	provider, err := newMigrationProvider(db, storedb.DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, downloadFailureKeyMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", downloadFailureKeyMigration-1, err)
	}
	// Keys as DownloadItem.Key() built them: one with a remote id, one known only by its URL.
	if _, err := db.ExecContext(ctx, `INSERT INTO filler_download_failures (item_key, provider, source_id, remote_id, url, failures)
		VALUES (?, 'YouTube', '9000a1', 'item-1', 'https://example.invalid/1', 2),
		       (?, 'archive', '9000a2', '', 'https://example.invalid/2', 1),
		       ('plain', 'archive', '', '', 'https://example.invalid/3', 1)`,
		"youtube\x009000a1\x00item-1", "archive\x009000a2\x00url:https://example.invalid/2"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, downloadFailureKeyMigration); err != nil {
		t.Fatalf("apply %d: %v", downloadFailureKeyMigration, err)
	}
	rows, err := db.QueryContext(ctx, `SELECT item_key, failures FROM filler_download_failures ORDER BY url`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var key string
		var failures int
		if err := rows.Scan(&key, &failures); err != nil {
			t.Fatal(err)
		}
		got = append(got, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"youtube\x1f9000a1\x1fitem-1", "archive\x1f9000a2\x1furl:https://example.invalid/2", "plain"}
	if !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q: the NUL keys rebuilt with unit separators, the plain key kept", got, want)
	}
}
