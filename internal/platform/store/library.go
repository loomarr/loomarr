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

// The library's value types and the readers and writers that carry them.
type (
	LibraryAnswer            = librarystore.LibraryAnswer
	LibraryAnswerBy          = librarystore.LibraryAnswerBy
	LibraryArt               = librarystore.LibraryArt
	LibraryArtReader         = librarystore.LibraryArtReader
	LibraryCandidate         = librarystore.LibraryCandidate
	LibraryCatalogDue        = librarystore.LibraryCatalogDue
	LibraryCatalogKey        = librarystore.LibraryCatalogKey
	LibraryCatalogStore      = librarystore.LibraryCatalogStore
	LibraryChapter           = librarystore.LibraryChapter
	LibraryCollection        = librarystore.LibraryCollection
	LibraryCollectionMember  = librarystore.LibraryCollectionMember
	LibraryCollectionStore   = librarystore.LibraryCollectionStore
	LibraryCredit            = librarystore.LibraryCredit
	LibraryEpisode           = librarystore.LibraryEpisode
	LibraryEpisodeOrder      = librarystore.LibraryEpisodeOrder
	LibraryEpisodeOrderStore = librarystore.LibraryEpisodeOrderStore
	LibraryEpisodeReader     = librarystore.LibraryEpisodeReader
	LibraryFile              = librarystore.LibraryFile
	LibraryFileCounts        = librarystore.LibraryFileCounts
	LibraryFilePath          = librarystore.LibraryFilePath
	LibraryFileStore         = librarystore.LibraryFileStore
	LibraryFilmEntry         = librarystore.LibraryFilmEntry
	LibraryFilmographyDue    = librarystore.LibraryFilmographyDue
	LibraryFrom              = librarystore.LibraryFrom
	LibraryHeldCount         = librarystore.LibraryHeldCount
	LibraryID                = librarystore.LibraryID
	LibraryItem              = librarystore.LibraryItem
	LibraryItemFacts         = librarystore.LibraryItemFacts
	LibraryItemChannel       = librarystore.LibraryItemChannel
	LibraryItemFactsReader   = librarystore.LibraryItemFactsReader
	LibraryItemPage          = librarystore.LibraryItemPage
	LibraryItemQuery         = librarystore.LibraryItemQuery
	LibraryItemStore         = librarystore.LibraryItemStore
	LibraryItemSummary       = librarystore.LibraryItemSummary
	LibraryMarker            = librarystore.LibraryMarker
	LibraryMatch             = librarystore.LibraryMatch
	LibraryNeed              = librarystore.LibraryNeed
	LibraryNeedsStore        = librarystore.LibraryNeedsStore
	LibraryOrderChoice       = librarystore.LibraryOrderChoice
	LibraryOrderNeed         = librarystore.LibraryOrderNeed
	LibraryOrigin            = librarystore.LibraryOrigin
	LibraryPeopleStore       = librarystore.LibraryPeopleStore
	LibraryPersonRef         = librarystore.LibraryPersonRef
	LibraryProviderID        = librarystore.LibraryProviderID
	LibraryRoot              = librarystore.LibraryRoot
	LibraryRootCheck         = librarystore.LibraryRootCheck
	LibraryRootDiscovery     = librarystore.LibraryRootDiscovery
	LibraryRootFlip          = librarystore.LibraryRootFlip
	LibraryRootPath          = librarystore.LibraryRootPath
	LibraryRootStore         = librarystore.LibraryRootStore
	LibraryRoots             = librarystore.LibraryRoots
	LibrarySource            = librarystore.LibrarySource
	LibraryStream            = librarystore.LibraryStream
	LibraryTitle             = librarystore.LibraryTitle
	LibraryTitleArt          = librarystore.LibraryTitleArt
	LibraryTitleReader       = librarystore.LibraryTitleReader
	LibraryTitleSearcher     = librarystore.LibraryTitleSearcher
	LibraryTrickplay         = librarystore.LibraryTrickplay
	LibraryWaitingFile       = librarystore.LibraryWaitingFile
)

// The library's constants: who answered a need, a file's header state, the item order and the
// source of an inherited rating.
const (
	AnsweredBySearch      = librarystore.AnsweredBySearch
	AnsweredByYou         = librarystore.AnsweredByYou
	LibraryHeaderFailed   = librarystore.LibraryHeaderFailed
	LibraryHeaderPending  = librarystore.LibraryHeaderPending
	LibraryHeaderRead     = librarystore.LibraryHeaderRead
	LibraryItemImageOwner = librarystore.LibraryItemImageOwner
	OrderByCatalog        = librarystore.OrderByCatalog
	OrderByFiles          = librarystore.OrderByFiles
	OrderByOrder          = librarystore.OrderByOrder
	RatingFromShow        = librarystore.RatingFromShow
)

// ErrBadEpisodeOrder is an answer whose choice and order id disagree, and ErrUnknownLibraryItem a
// collection write naming a title the library has no row for (the librarystore errors, the same
// values under second names).
var (
	ErrBadEpisodeOrder    = librarystore.ErrBadEpisodeOrder
	ErrUnknownLibraryItem = librarystore.ErrUnknownLibraryItem
)

// IDIn and ParseLibraryCatalogKey are the cluster's helpers under their store names.
var (
	IDIn                   = librarystore.IDIn
	ParseLibraryCatalogKey = librarystore.ParseLibraryCatalogKey
)
