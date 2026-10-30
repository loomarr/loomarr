package storedb

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDBRunsStatementsAndTransactions(t *testing.T) {
	ctx := context.Background()
	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1) // one connection, so the in-memory database is shared by every call
	t.Cleanup(func() { _ = pool.Close() })

	var h Handle = NewDB(pool, DialectSQLite, Passthrough)
	if h.Dialect() != DialectSQLite {
		t.Fatalf("Dialect = %q", h.Dialect())
	}
	if got := h.Rebind("a = ? AND b = ?"); got != "a = ? AND b = ?" {
		t.Fatalf("sqlite Rebind = %q, want passthrough", got)
	}
	if got := NewDB(pool, DialectPostgres, PgPlaceholders).Rebind("a = ? AND b = ?"); got != "a = $1 AND b = $2" {
		t.Fatalf("postgres Rebind = %q", got)
	}
	if _, err := h.ExecContext(ctx, `CREATE TABLE t (n INTEGER)`); err != nil {
		t.Fatal(err)
	}

	tx, err := h.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, h.Rebind(`INSERT INTO t (n) VALUES (?)`), 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.QueryRowContext(ctx, `SELECT COUNT(*) FROM t`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("after rollback: n=%d err=%v, want 0", n, err)
	}

	tx, err = h.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t (n) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback() // after Commit, a deferred Rollback must stay harmless
	if err := h.QueryRowContext(ctx, `SELECT COUNT(*) FROM t`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after commit: n=%d err=%v, want 1", n, err)
	}
	rows, err := h.QueryContext(ctx, `SELECT n FROM t`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	// SQLite has no advisory lock; the kernel refuses rather than silently not locking.
	if err := h.WithAdvisoryLock(ctx, "label", "ns:", "k", func(context.Context) error { return nil }); err == nil {
		t.Fatal("WithAdvisoryLock on SQLite = nil, want an unsupported-dialect error")
	}
}

// fakeLocker records which path KeyedLock took.
type fakeLocker struct {
	dialect Dialect
	calls   []string
}

func (f *fakeLocker) Dialect() Dialect { return f.dialect }

func (f *fakeLocker) WithAdvisoryLock(ctx context.Context, label, namespace, key string, fn func(context.Context) error) error {
	f.calls = append(f.calls, label+"|"+namespace+"|"+key)
	return fn(ctx)
}

func TestKeyedLockUsesAdvisoryLockOnPostgres(t *testing.T) {
	var lock KeyedLock
	pg := &fakeLocker{dialect: DialectPostgres}
	ran := false
	if err := lock.With(context.Background(), pg, "invitation identity lock", "invitation-identity:", "k", func(context.Context) error {
		ran = true
		return nil
	}); err != nil || !ran {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
	if len(pg.calls) != 1 || pg.calls[0] != "invitation identity lock|invitation-identity:|k" {
		t.Fatalf("advisory calls = %v", pg.calls)
	}
}

func TestKeyedLockSerializesSQLiteByKey(t *testing.T) {
	var lock KeyedLock
	sqlite := &fakeLocker{dialect: DialectSQLite}
	ctx := context.Background()

	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_ = lock.With(ctx, sqlite, "l", "ns:", "same", func(context.Context) error {
				if inside.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(2 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()
	if overlaps.Load() != 0 {
		t.Fatalf("%d overlapping runs under one key", overlaps.Load())
	}
	if len(sqlite.calls) != 0 {
		t.Fatalf("SQLite took the advisory path: %v", sqlite.calls)
	}

	// A different key is a different lock: it must not wait on a held one.
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = lock.With(ctx, sqlite, "l", "ns:", "a", func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	done := make(chan struct{})
	go func() {
		_ = lock.With(ctx, sqlite, "l", "ns:", "b", func(context.Context) error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("key b blocked behind key a")
	}
	close(release)
}

// Two domains each own a KeyedLock, so equal keys in different domains never contend on SQLite.
func TestKeyedLocksAreIndependentDomains(t *testing.T) {
	var a, b KeyedLock
	sqlite := &fakeLocker{dialect: DialectSQLite}
	ctx := context.Background()
	held, release := make(chan struct{}), make(chan struct{})
	go func() {
		_ = a.With(ctx, sqlite, "l", "a:", "k", func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	done := make(chan struct{})
	go func() {
		_ = b.With(ctx, sqlite, "l", "b:", "k", func(context.Context) error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("domain b blocked behind domain a")
	}
	close(release)
}
