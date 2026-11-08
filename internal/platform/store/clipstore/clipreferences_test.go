package clipstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Every clip_hash column in the migrated schema is one the re-key accounts for, and every column
// the re-key names still exists. A table added without an entry would keep pointing at a clip's
// retired hash after its bytes are re-encoded, the way nine tables did before #2121.
func TestEveryClipHashColumnIsAccountedFor(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema.db")
	migrated, err := store.Open(ctx, "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.QueryContext(ctx, `SELECT m.name, p.name FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	schema := map[string]bool{}
	var clipHashColumns []string
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		schema[table+"."+column] = true
		if column == "clip_hash" {
			clipHashColumns = append(clipHashColumns, table+"."+column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	listed := map[string]bool{}
	for _, column := range clipstore.ClipReferenceColumns() {
		listed[column] = true
		if !schema[column] {
			t.Errorf("clipReferences names %s, which the schema no longer has", column)
		}
	}
	for _, column := range clipHashColumns {
		if !listed[column] {
			t.Errorf("%s names a clip, but clipReferences does not say what a re-key does with it", column)
		}
	}
}
