package fillerstore

import (
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/store/catalogstore"
)

// The filler conformance suite moved here from internal/store unedited (#1747); these test-only
// aliases name the core store types and errors it asserts against. They leave the package's
// non-test API untouched: production code in this package spells them store.X.
type (
	Clip                 = catalogstore.Clip
	ClipFilter           = catalogstore.ClipFilter
	ClipSort             = catalogstore.ClipSort
	TaxonomyEdit         = catalogstore.TaxonomyEdit
	InteractiveOperation = store.InteractiveOperation
	Image                = catalogstore.Image
	ImageRef             = catalogstore.ImageRef
	InferiorCopy         = catalogstore.InferiorCopy
)

const (
	ClipSortAdded      = catalogstore.ClipSortAdded
	ClipSortConfidence = catalogstore.ClipSortConfidence
	ClipSortDuration   = catalogstore.ClipSortDuration
	ClipSortName       = catalogstore.ClipSortName
	ClipSortPlays      = catalogstore.ClipSortPlays

	InteractiveOperationRunning     = store.InteractiveOperationRunning
	InteractiveOperationSuccess     = store.InteractiveOperationSuccess
	InteractiveOperationError       = store.InteractiveOperationError
	InteractiveOperationLLMPull     = store.InteractiveOperationLLMPull
	InteractiveOperationFillerSplit = store.InteractiveOperationFillerSplit

	DialectPostgres = store.DialectPostgres
)

var (
	ErrNotFound                        = store.ErrNotFound
	ErrTaxonConflict                   = catalogstore.ErrTaxonConflict
	ErrUnknownClipSort                 = catalogstore.ErrUnknownClipSort
	ErrConditioningPublicationMismatch = filler.ErrConditioningOwnershipMismatch
)
