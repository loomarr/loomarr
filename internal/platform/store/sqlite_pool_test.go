package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// The live SQLite pool must let reads run while a write transaction is open, and make a second
// writer wait for the lock rather than fail (#2446). With MaxOpenConns(1) the read below queued
// behind the open transaction until its context expired, which is what prod's River fetch
// timeouts were.
func TestSQLitePool_ReadsProceedAndWritersWaitWhileAWriteIsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "pool.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}

	writer, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `INSERT INTO t (x) VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var n int
	if err := s.db.QueryRowContext(readCtx, `SELECT COUNT(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("read while a write transaction is open: %v", err)
	}
	if n != 0 {
		t.Fatalf("read saw %d uncommitted rows, want 0", n)
	}

	second := make(chan error, 1)
	go func() {
		_, err := s.db.ExecContext(ctx, `INSERT INTO t (x) VALUES (2)`)
		second <- err
	}()
	select {
	case err := <-second:
		t.Fatalf("second writer finished while the first held the write lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second writer after the first committed: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM t`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows = %d, %v; want 2", n, err)
	}
}

// Every pooled connection carries the DSN's settings, not just the first one opened.
func TestSQLitePool_EveryConnectionHasTheLiveSettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	conns := make([]*sql.Conn, 0, sqliteMaxOpenConns)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for i := 0; i < sqliteMaxOpenConns; i++ {
		c, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		conns = append(conns, c)
		var mode string
		var busy, synchronous, foreignKeys int
		if err := c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || busy != 15000 || synchronous != 1 || foreignKeys != 1 {
			t.Errorf("connection %d: journal_mode=%s busy_timeout=%d synchronous=%d foreign_keys=%d; want wal 15000 1 (NORMAL) 1",
				i, mode, busy, synchronous, foreignKeys)
		}
	}
}
