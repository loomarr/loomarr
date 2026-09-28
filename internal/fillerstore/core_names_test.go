package fillerstore

import "github.com/loomarr/loomarr/internal/store"

// The filler conformance suite moved here from internal/store unedited (#1747); these test-only
// aliases name the core store types and errors it asserts against. They leave the package's
// non-test API untouched: production code in this package spells them store.X.
type (
	Clip                 = store.Clip
	ClipFilter           = store.ClipFilter
	ClipSort             = store.ClipSort
	TaxonomyEdit         = store.TaxonomyEdit
	InteractiveOperation = store.InteractiveOperation
	Image                = store.Image
	ImageRef             = store.ImageRef
)

const (
	ClipSortAdded      = store.ClipSortAdded
	ClipSortConfidence = store.ClipSortConfidence
	ClipSortDuration   = store.ClipSortDuration
	ClipSortName       = store.ClipSortName
	ClipSortPlays      = store.ClipSortPlays

	InteractiveOperationRunning     = store.InteractiveOperationRunning
	InteractiveOperationSuccess     = store.InteractiveOperationSuccess
	InteractiveOperationError       = store.InteractiveOperationError
	InteractiveOperationLLMPull     = store.InteractiveOperationLLMPull
	InteractiveOperationFillerSplit = store.InteractiveOperationFillerSplit

	DialectPostgres = store.DialectPostgres
)

var (
	ErrNotFound                        = store.ErrNotFound
	ErrTaxonConflict                   = store.ErrTaxonConflict
	ErrUnknownClipSort                 = store.ErrUnknownClipSort
	ErrConditioningPublicationMismatch = store.ErrConditioningPublicationMismatch
)
