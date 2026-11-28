package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/housekeeping"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/quality"
)

// A builder takes a struct of the roles it hands on, not the aggregate store.Store: the struct
// says what the builder can reach. Each is filled from the one store Build holds; a nil store
// leaves every role nil, and the builder skips what needs persistence.

type approvalStores struct {
	binder       approval.Store
	approver     accept.ApproveStore
	observations quality.ObservationRecorder
	library      librarystore.LibraryTitleReader
}

func newApprovalStores(st store.Store) approvalStores {
	if st == nil {
		return approvalStores{}
	}
	return approvalStores{binder: st, approver: st, observations: st, library: st}
}

type provisioningStores struct {
	titles       store.TitleStore
	library      librarystore.LibraryTitleReader
	observations quality.ObservationRecorder
	housekeeping housekeeping.Store
	// tidy compacts the database; nil for a backend that needs none.
	tidy func(context.Context) (freedBytes int64, err error)
}

// Housekeeping purges filler records too, so this one is filled from the filler extension.
func newProvisioningStores(st records.Store) provisioningStores {
	if st == nil {
		return provisioningStores{}
	}
	roles := provisioningStores{titles: st, library: st, observations: st, housekeeping: st}
	if tidier := store.DatabaseTidier(st); tidier != nil {
		roles.tidy = tidier.Tidy
	}
	return roles
}
