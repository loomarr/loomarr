package store

import (
	"time"

	"github.com/loomarr/loomarr/internal/store/storedb"
)

// Querier, Tx and Scannable are the storedb kernel's types, re-exported so importers of this
// package keep one import. They are aliases: the same types, not copies.
type (
	// Querier runs statements, on the pool or inside one transaction (storedb.Querier).
	Querier = storedb.Querier
	// Tx is one transaction begun through Handle.Begin (storedb.Tx).
	Tx = storedb.Tx
	// Scannable is the one method *sql.Row and *sql.Rows share (storedb.Scannable).
	Scannable = storedb.Scannable
)

// Handle is the core store's database handle, lent to the stores that extend it
// (internal/fillerstore). Those stores keep their own tables but run on this handle, and Begin
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
	Clips(tx Tx) ClipTx
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

func (h sqlHandle) Clips(tx Tx) ClipTx { return h.s.clipsIn(tx) }

// kernel is this store's pool, dialect and placeholder style as a storedb.Handle, for the store
// packages that run on the kernel and for this package's own keyed locks.
func (s *sqlStore) kernel() storedb.DB { return storedb.NewDB(s.db, s.dialect, s.ph) }

// Epoch and FromEpoch are the store's Unix-seconds time-column codec, exported so extension stores
// encode time exactly as the core does.
func Epoch(t time.Time) int64 { return storedb.Epoch(t) }

// FromEpoch decodes Epoch.
func FromEpoch(n int64) time.Time { return storedb.FromEpoch(n) }
