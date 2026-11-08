package records

import (
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The filler conformance suite moved here from internal/platform/store unedited (#1747); these test-only
// aliases name the core store types and errors it asserts against. They leave the package's
// non-test API untouched: production code in this package spells them store.X.
type (
	Clip                 = clipstore.Clip
	ClipFilter           = clipstore.ClipFilter
	ClipSort             = clipstore.ClipSort
	TaxonomyEdit         = clipstore.TaxonomyEdit
	InteractiveOperation = store.InteractiveOperation
	Image                = clipstore.Image
	ImageRef             = clipstore.ImageRef
	InferiorCopy         = clipstore.InferiorCopy
)

const (
	ClipSortAdded    = clipstore.ClipSortAdded
	ClipSortDuration = clipstore.ClipSortDuration
	ClipSortName     = clipstore.ClipSortName
	ClipSortPlays    = clipstore.ClipSortPlays

	InteractiveOperationRunning     = store.InteractiveOperationRunning
	InteractiveOperationSuccess     = store.InteractiveOperationSuccess
	InteractiveOperationError       = store.InteractiveOperationError
	InteractiveOperationLLMPull     = store.InteractiveOperationLLMPull
	InteractiveOperationFillerSplit = store.InteractiveOperationFillerSplit

	DialectPostgres = store.DialectPostgres
)

var (
	ErrNotFound                        = store.ErrNotFound
	ErrTaxonConflict                   = clipstore.ErrTaxonConflict
	ErrUnknownClipSort                 = clipstore.ErrUnknownClipSort
	ErrConditioningPublicationMismatch = filler.ErrConditioningOwnershipMismatch
)
