package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// invitationLibraryResolver adapts the configured Library client to the exact-account
// lookup seam. The server's list is authoritative: ids absent from it are never invented.
type invitationLibraryResolver struct{ client *library.Client }

func (r invitationLibraryResolver) ResolveLibraryAccount(
	ctx context.Context,
	id string,
) (access.LibraryAccount, error) {
	users, err := r.client.ListUsers(ctx)
	if err != nil {
		return access.LibraryAccount{}, err
	}
	for _, user := range users {
		if user.ID == id {
			return access.LibraryAccount{ID: user.ID, Name: user.Name, Disabled: user.Disabled}, nil
		}
	}
	return access.LibraryAccount{}, store.ErrNotFound
}
