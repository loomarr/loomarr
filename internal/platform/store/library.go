package store

import "github.com/loomarr/loomarr/internal/platform/store/librarystore"

// The Library lives in internal/platform/store/librarystore: its scan roots, files, items, titles,
// catalog keys, collections, people, episodes, episode orders and needs. Every name below is the
// cluster's own under a second name, so the aggregate interfaces in store.go and the projection in
// this package read unchanged. Callers outside the store name librarystore directly;
// internal/storearchitecture_test.go enforces that.

// libraryCluster names the embedded library store; see identityCluster for why an alias.
type libraryCluster = librarystore.Store

// bindLibrary installs the library cluster on s; bindClusters calls it on every construction.
func bindLibrary(s *sqlStore) *sqlStore {
	s.libraryCluster = librarystore.New(s.kernel())
	return s
}

// The library's readers and writers, and the item they write.
type (
	LibraryArtReader         = librarystore.LibraryArtReader
	LibraryCatalogStore      = librarystore.LibraryCatalogStore
	LibraryCollectionStore   = librarystore.LibraryCollectionStore
	LibraryEpisodeOrderStore = librarystore.LibraryEpisodeOrderStore
	LibraryEpisodeReader     = librarystore.LibraryEpisodeReader
	LibraryFileStore         = librarystore.LibraryFileStore
	LibraryItem              = librarystore.LibraryItem
	LibraryItemFactsReader   = librarystore.LibraryItemFactsReader
	SeriesEpisodesReader     = librarystore.SeriesEpisodesReader
	LibraryEvidenceReader    = librarystore.LibraryEvidenceReader
	LibraryItemStore         = librarystore.LibraryItemStore
	LibraryNeedsStore        = librarystore.LibraryNeedsStore
	LibraryPeopleStore       = librarystore.LibraryPeopleStore
	LibraryRootStore         = librarystore.LibraryRootStore
	LibraryTitleReader       = librarystore.LibraryTitleReader
	LibraryTitleSearcher     = librarystore.LibraryTitleSearcher
)
