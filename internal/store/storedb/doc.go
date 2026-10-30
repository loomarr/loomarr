// Package storedb is the dependency-free kernel of Loomarr's persistence layer (design §5): the
// small, backend-level pieces every store package needs and that know nothing about any table.
//
// What belongs here: the database/sql abstractions the stores are written against (Querier, Tx,
// Scannable), the backend name and its placeholder styles (Dialect, Placeholder), the Unix-seconds
// time-column codec (Epoch, FromEpoch), the cross-replica advisory lock, ErrNotFound, and the
// portable unique-violation test. Handle (with its concrete DB) bundles the pool, Begin, dialect,
// placeholder rebinding and advisory lock a cluster store needs to run its own statements and
// transactions; KeyedLock is the per-key serialization each cluster owns for its own domain. Each of these is about talking to SQLite or Postgres, not about
// what Loomarr stores.
//
// What does not belong here: any domain type (Channel, Job, Proposal, ...), any SQL that names a
// Loomarr table, the Store interface and its role interfaces, the sqlStore adapter, migrations,
// and anything that needs another Loomarr package. This package imports no github.com/loomarr/loomarr
// package; TestStoreDBImportsNoLoomarrPackage (internal/architecture_test.go) holds that line, so a
// future cluster package (programming, catalog, identity, ops) can depend on it without
// pulling in, or cycling through, internal/store.
//
// internal/store re-exports these names as aliases so existing importers keep compiling; the
// alias and the original are the same type or value, never a copy.
package storedb
