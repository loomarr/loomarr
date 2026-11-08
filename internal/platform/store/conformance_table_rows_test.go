package store

import (
	"context"
	"testing"
	"time"
)

// testTableRows: the nightly count covers every table but the migration ledger and the history
// itself, a second count the same day replaces the first, and days older than the window go.
func testTableRows(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	h := HandleOf(s)
	day := time.Date(2026, 11, 8, 4, 30, 0, 0, time.UTC)
	midnight := day.Truncate(24 * time.Hour).Unix()

	rowsOn := func(at int64, table string) (int64, bool) {
		t.Helper()
		var n int64
		err := h.QueryRowContext(ctx, h.Rebind(`SELECT row_count FROM database_table_rows WHERE recorded_at = ? AND table_name = ?`),
			at, table).Scan(&n)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	usersNow := func() int64 {
		t.Helper()
		var n int64
		if err := h.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	counted, err := s.RecordTableRows(ctx, day, day.Add(-400*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if counted < 50 {
		t.Fatalf("counted %d tables, want every application table", counted)
	}
	if n, ok := rowsOn(midnight, "users"); !ok || n != usersNow() {
		t.Fatalf("users on %d = %d (%v), want %d", midnight, n, ok, usersNow())
	}
	for _, skipped := range []string{"goose_db_version", "database_table_rows"} {
		if _, ok := rowsOn(midnight, skipped); ok {
			t.Fatalf("%s was counted; it says nothing about retention", skipped)
		}
	}

	// A second count the same day replaces the first.
	mustUpsertPreferenceUser(t, s, "u-9000101")
	if _, err := s.RecordTableRows(ctx, day.Add(time.Hour), day.Add(-400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, _ := rowsOn(midnight, "users"); n != usersNow() {
		t.Fatalf("users after a second count the same day = %d, want %d", n, usersNow())
	}

	// The next night keeps yesterday; a night past the window forgets it.
	next := day.Add(24 * time.Hour)
	if _, err := s.RecordTableRows(ctx, next, day.Truncate(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := rowsOn(midnight, "users"); !ok {
		t.Fatal("yesterday's count was forgotten inside the window")
	}
	if _, err := s.RecordTableRows(ctx, next, next.Truncate(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := rowsOn(midnight, "users"); ok {
		t.Fatal("a count older than the window was kept")
	}
	if _, ok := rowsOn(next.Truncate(24*time.Hour).Unix(), "users"); !ok {
		t.Fatal("tonight's count was forgotten")
	}
}
