//go:build integration

package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestPostgresUniqueChannelIntentRefMigrationPreservesDuplicateChannels(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 54); err != nil {
		t.Fatalf("migrate through 54: %v", err)
	}
	insert := func(id, intentRef string, number int, updatedAt int64) {
		t.Helper()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO channels (id, intent_ref, name, number, strategy, status, updated_at)
			 VALUES ($1, $2, $3, $4, 'sequential', 'building', $5)`,
			id, intentRef, id, number, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	insert("older", "job-one", 1, 10)
	insert("newer", "job-one", 2, 20)
	insert("tie-a", "job-tie", 3, 30)
	insert("tie-z", "job-tie", 4, 30)
	insert("empty-a", "", 5, 40)
	insert("empty-b", "", 6, 50)
	insert("active-old", "job-detached", 7, 10)
	insert("detached-new", "job-detached", 8, 100)
	if _, err := db.ExecContext(ctx, `UPDATE channels SET status='detached' WHERE id='detached-new'`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 55); err != nil {
		t.Fatalf("migrate through 55: %v", err)
	}
	refs := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT id, intent_ref FROM channels`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, ref string
		if err := rows.Scan(&id, &ref); err != nil {
			t.Fatal(err)
		}
		refs[id] = ref
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 8 {
		t.Fatalf("channels after migration = %d, want all 8 preserved", len(refs))
	}
	if refs["newer"] != "job-one" || refs["older"] != "" {
		t.Errorf("updated_at winner refs = %+v", refs)
	}
	if refs["tie-z"] != "job-tie" || refs["tie-a"] != "" {
		t.Errorf("id tie-break winner refs = %+v", refs)
	}
	if refs["empty-a"] != "" || refs["empty-b"] != "" {
		t.Errorf("empty refs changed = %+v", refs)
	}
	if refs["active-old"] != "job-detached" || refs["detached-new"] != "" {
		t.Errorf("managed row did not win over newer detached duplicate: %+v", refs)
	}

	insert("empty-c", "", 9, 60)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, intent_ref, name, number, strategy, status)
		 VALUES ('duplicate', 'job-one', 'duplicate', 10, 'sequential', 'building')`); err == nil {
		t.Fatal("partial unique index accepted a second non-empty intent_ref")
	}
}
