package testkit

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/invitation"
)

// LibraryAccountResolver is the shared in-memory adapter for Invitation account selection.
type LibraryAccountResolver struct {
	Accounts map[string]invitation.LibraryAccount
	Err      error
}

func (r LibraryAccountResolver) ResolveLibraryAccount(
	_ context.Context,
	id string,
) (invitation.LibraryAccount, error) {
	if r.Err != nil {
		return invitation.LibraryAccount{}, r.Err
	}
	account, ok := r.Accounts[id]
	if !ok {
		return invitation.LibraryAccount{}, fmt.Errorf("Library account not found")
	}
	return account, nil
}
