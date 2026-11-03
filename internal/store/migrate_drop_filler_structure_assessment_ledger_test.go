package store

import (
	"context"
	"path/filepath"
	"testing"
)

// The whole-video assessment ledger retires with no backfill; the window call journal that
// replaced it must survive the drop.
func TestDropFillerStructureAssessmentLedgerMigrationKeepsWindowLedgerSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "drop-assessment-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	tableExists := func(name string) bool {
		t.Helper()
		var found int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found == 1
	}
	if _, err := provider.UpTo(ctx, 146); err != nil {
		t.Fatalf("migrate through 146: %v", err)
	}
	if !tableExists("filler_structure_assessment_ledger") || !tableExists("filler_structure_window_call_ledger") {
		t.Fatal("both structure ledgers should exist before the drop")
	}
	if _, err := provider.UpTo(ctx, 149); err != nil {
		t.Fatalf("apply the drop: %v", err)
	}
	if tableExists("filler_structure_assessment_ledger") {
		t.Fatal("the whole-video assessment ledger survived the drop")
	}
	if !tableExists("filler_structure_window_call_ledger") {
		t.Fatal("the drop took the window call ledger with it")
	}
}
