package storedb

import (
	"context"
	"database/sql"
	"testing"
)

func TestRowLocksAppendOnPostgresOnly(t *testing.T) {
	const q = `SELECT 1 FROM t WHERE id = ?`
	tests := []struct {
		name         string
		got          func(Dialect) string
		wantPostgres string
	}{
		{"LockRows", func(d Dialect) string { return LockRows(d, q) }, q + ` FOR UPDATE`},
		{"LockRowsShared", func(d Dialect) string { return LockRowsShared(d, q) }, q + ` FOR SHARE`},
		{"LockRowsSkipLocked", func(d Dialect) string { return LockRowsSkipLocked(d, q) }, q + ` FOR UPDATE SKIP LOCKED`},
		{"LockRowsOf", func(d Dialect) string { return LockRowsOf(d, q, `a, b`) }, q + ` FOR UPDATE OF a, b`},
	}
	for _, tt := range tests {
		if got := tt.got(DialectPostgres); got != tt.wantPostgres {
			t.Errorf("%s on postgres = %q, want %q", tt.name, got, tt.wantPostgres)
		}
		if got := tt.got(DialectSQLite); got != q {
			t.Errorf("%s on sqlite = %q, want the query unchanged", tt.name, got)
		}
	}
}

func TestByDialectPicksTheBackendText(t *testing.T) {
	if got := ByDialect(DialectSQLite, "lite", "pg"); got != "lite" {
		t.Errorf("sqlite = %q, want lite", got)
	}
	if got := ByDialect(DialectPostgres, "lite", "pg"); got != "pg" {
		t.Errorf("postgres = %q, want pg", got)
	}
}

type recordingQuerier struct {
	Querier
	stmts []string
}

func (r *recordingQuerier) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	r.stmts = append(r.stmts, query)
	return nil, nil
}

func TestXactLockRunsOnPostgresOnly(t *testing.T) {
	const lock = `SELECT pg_advisory_xact_lock(1)`
	sqlite := &recordingQuerier{}
	if err := XactLock(context.Background(), DialectSQLite, sqlite, lock); err != nil || len(sqlite.stmts) != 0 {
		t.Fatalf("sqlite: err=%v stmts=%v, want no statement", err, sqlite.stmts)
	}
	pg := &recordingQuerier{}
	if err := XactLock(context.Background(), DialectPostgres, pg, lock); err != nil || len(pg.stmts) != 1 || pg.stmts[0] != lock {
		t.Fatalf("postgres: err=%v stmts=%v, want exactly the lock statement", err, pg.stmts)
	}
}
