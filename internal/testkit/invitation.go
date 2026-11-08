package testkit

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/people/access"
)

// LibraryAccountResolver is the shared in-memory adapter for Invitation account selection.
type LibraryAccountResolver struct {
	Accounts map[string]access.LibraryAccount
	Err      error
}

func (r LibraryAccountResolver) ResolveLibraryAccount(
	_ context.Context,
	id string,
) (access.LibraryAccount, error) {
	if r.Err != nil {
		return access.LibraryAccount{}, r.Err
	}
	account, ok := r.Accounts[id]
	if !ok {
		return access.LibraryAccount{}, fmt.Errorf("library account not found")
	}
	return account, nil
}
