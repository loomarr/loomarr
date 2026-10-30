package storedb

import (
	"context"
	"database/sql"
)

// Querier runs statements, on the pool or inside one transaction. *sql.DB and *sql.Tx both
// satisfy it, so a helper written against it runs either way.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx is one transaction begun through the store's Handle. Rollback after Commit is a no-op, so
// callers defer it straight after Begin.
type Tx interface {
	Querier
	Commit() error
	Rollback() error
}

// Scannable is the one method *sql.Row and *sql.Rows share, so one scan function reads either.
//
// ⚠ THE ONE NAME FOR THIS SHAPE. The package had four — `scannable`, `rowScanner`
// (fillerpulls.go), `scanner` (images.go) and an anonymous `interface{ Scan(...any) error }`
// inline in clippipeline.go — all structurally identical, so every one of them was already
// satisfied by every other's arguments. Four names for one idea is a tax on reading, not a
// distinction: a reader meeting the third has to prove to themselves it is not subtly
// different. Consolidated 2026-08-10; put new scan helpers on this one.
type Scannable interface {
	Scan(dest ...any) error
}
