package store

import (
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Handle is the core store's database handle, lent to the stores that extend it
// (internal/filler/records). Those stores keep their own tables but run on this handle, and Begin
// is the one way to start a transaction: a write spanning core and extension tables begins here,
// passes the Tx to both halves, and commits or rolls back as one.
//
// ⚠ For extension stores only, and deliberately NOT on the Store interface, for PoolOf's reason:
// everything else goes through Store's methods, which own the tables' invariants.
//
// Handle is storedb.Handle (pool, Begin, Dialect, Rebind, advisory lock: everything a store package
// needs to run on its own) plus Clips, the one thing that still ties it to this package. A store
// that needs no clip writes takes the narrower storedb.Handle, and a store.Handle is one.
type Handle interface {
	storedb.Handle
	// Clips writes clip columns and tags inside tx, which Begin started. Extension stores write
	// the clips table only through it.
	Clips(tx storedb.Tx) clipstore.ClipTx
}

// HandleOf lends st's database handle to an extension store, or returns nil for a non-SQL store.
func HandleOf(st Store) Handle {
	s, ok := adapterOf(st)
	if !ok {
		return nil
	}
	return sqlHandle{DB: s.kernel(), s: s}
}

// adapterOf returns the SQL adapter behind st, looking through a store that extends it (the
// filler store's Core). Backups, migration and invalidation need the adapter itself.
func adapterOf(st Store) (*sqlStore, bool) {
	if ext, ok := st.(interface{ Core() Store }); ok {
		st = ext.Core()
	}
	s, ok := st.(*sqlStore)
	return s, ok
}

// sqlHandle is the kernel handle plus Clips. Everything but Clips is promoted from the embedded
// storedb.DB, so the pool, Begin, dialect, rebinding and advisory lock have one implementation
// shared with every cluster store that takes a storedb.Handle.
type sqlHandle struct {
	storedb.DB
	s *sqlStore
}

func (h sqlHandle) Clips(tx storedb.Tx) clipstore.ClipTx {
	return clipstore.ClipsOn(h.s.catalogCluster, tx)
}

// kernel is this store's pool, dialect and placeholder style as a storedb.Handle, for the store
// packages that run on the kernel and for this package's own keyed locks.
func (s *sqlStore) kernel() storedb.DB { return storedb.NewDB(s.db, s.dialect, s.ph) }
