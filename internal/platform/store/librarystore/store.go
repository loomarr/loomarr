// Package librarystore persists the Library: its scan roots, files, items, titles, catalog keys,
// collections, people, episodes, episode orders and the needs that ask the household a question.
//
// It is a cluster of the store, built on the storedb kernel and never importing the store: the
// store embeds a Store in its aggregate and re-exports every name here by alias, so every caller of
// store.Store keeps working unchanged. The inventory projection (internal/platform/store) writes
// library rows inside its own transaction through SaveItem.
package librarystore

import "github.com/loomarr/loomarr/internal/platform/store/storedb"

// Store is the Library persistence over one storedb.Handle.
type Store struct {
	h storedb.Handle
}

// New returns the library store for h.
func New(h storedb.Handle) *Store { return &Store{h: h} }

// ph rebinds a query written with ? markers to the backend's placeholder style.
func (s *Store) ph(query string) string { return s.h.Rebind(query) }

// lower is expr folded to lower case in every script, on either backend: SQLite's own LOWER folds
// ASCII only, so it goes through the unicode_lower function the store registers.
func (s *Store) lower(expr string) string {
	return storedb.ByDialect(s.h.Dialect(), "unicode_lower("+expr+")", "LOWER("+expr+")")
}
