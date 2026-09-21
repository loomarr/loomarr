package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRemoveEmptyFillerGeographyMigrationSQLite(t *testing.T) {
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "empty-geography.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 119); err != nil {
		t.Fatalf("migrate through 119: %v", err)
	}
	for hash, fixture := range map[string]struct {
		value, evidence string
	}{
		"empty":          {`{"geography":{}}`, "item_metadata"},
		"operator-empty": {`{"geography":{}}`, "operator"},
		"country":        {`{"geography":{"scope":"national","country":"US"}}`, "item_metadata"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_enrichment_axes
			(clip_hash, axis, state, value_json, evidence_kind, evidence_rank, evidence_reference,
			 confidence, producer, producer_version, taxonomy_version, observed_at, updated_at)
			VALUES (?, 'geography', 'complete', ?, ?, 5, 'fixture', 100,
			 'deterministic-metadata', '3', '', 1, 1)`, hash, fixture.value, fixture.evidence); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 120); err != nil {
		t.Fatalf("apply empty-geography cleanup: %v", err)
	}
	var empty, operatorEmpty, country int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM filler_enrichment_axes WHERE clip_hash = 'empty'`).Scan(&empty); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM filler_enrichment_axes WHERE clip_hash = 'operator-empty'`).Scan(&operatorEmpty); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM filler_enrichment_axes WHERE clip_hash = 'country'`).Scan(&country); err != nil {
		t.Fatal(err)
	}
	if empty != 0 || operatorEmpty != 1 || country != 1 {
		t.Fatalf("post-migration rows: empty=%d operator-empty=%d country=%d", empty, operatorEmpty, country)
	}
}
