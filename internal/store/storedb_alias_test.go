package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store/storedb"
)

// The store re-exports the storedb kernel by alias so importers keep one import. Aliases are only
// safe if they are IDENTITY, not copies: a second errors.New("store: not found") would print the
// same and make errors.Is silently false for every caller that matches the other one.
func TestErrNotFoundIsTheStoredbError(t *testing.T) {
	// errors.Is compares by identity, so a distinct errors.New copy with the same text fails here.
	if !errors.Is(ErrNotFound, storedb.ErrNotFound) {
		t.Fatal("store.ErrNotFound is not the storedb.ErrNotFound value")
	}
	if !errors.Is(fmt.Errorf("wrapped: %w", storedb.ErrNotFound), ErrNotFound) {
		t.Fatal("errors.Is(storedb error, store.ErrNotFound) = false")
	}
	if !errors.Is(fmt.Errorf("wrapped: %w", ErrNotFound), storedb.ErrNotFound) {
		t.Fatal("errors.Is(store error, storedb.ErrNotFound) = false")
	}
	if got, want := ErrNotFound.Error(), "store: not found"; got != want {
		t.Fatalf("message = %q, want %q (a log string must not change)", got, want)
	}
}

// A real read path: the miss is produced by store code and must match both names.
func TestMissingRowMatchesBothNotFoundNames(t *testing.T) {
	st := newSQLiteStore(t)
	_, err := st.GetTitle(context.Background(), provision.Key("tmdb:movie:0"))
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, storedb.ErrNotFound) {
		t.Fatalf("GetTitle miss = %v, want a match for store.ErrNotFound and storedb.ErrNotFound", err)
	}
}

// Querier, Tx, Dialect and Scannable are aliases: values of the storedb types satisfy the store's
// names with no conversion.
var (
	_ Querier   = storedb.Querier(nil)
	_ Tx        = storedb.Tx(nil)
	_ Scannable = storedb.Scannable(nil)
	_ Dialect   = storedb.DialectSQLite
)

// store.Handle is a storedb.Handle plus Clips: a cluster store written against the kernel handle
// runs on the one HandleOf lends, and store.Handle carries the same advisory lock and dialect.
func TestStoreHandleIsAKernelHandle(t *testing.T) {
	var kernel storedb.Handle = HandleOf(newSQLiteStore(t))
	if kernel.Dialect() != DialectSQLite {
		t.Fatalf("Dialect = %q", kernel.Dialect())
	}
	tx, err := kernel.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(context.Background(), kernel.Rebind(`SELECT COUNT(*) FROM users WHERE id <> ?`), "x").Scan(&n); err != nil {
		t.Fatalf("query through the kernel handle: %v", err)
	}
}
