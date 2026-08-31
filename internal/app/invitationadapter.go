package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
)

// invitationLibraryResolver adapts the configured Library client to the exact-account
// lookup seam. The server's list is authoritative: ids absent from it are never invented.
type invitationLibraryResolver struct{ client *library.Client }

func (r invitationLibraryResolver) ResolveLibraryAccount(
	ctx context.Context,
	id string,
) (invitation.LibraryAccount, error) {
	users, err := r.client.ListUsers(ctx)
	if err != nil {
		return invitation.LibraryAccount{}, err
	}
	for _, user := range users {
		if user.ID == id {
			return invitation.LibraryAccount{ID: user.ID, Name: user.Name, Disabled: user.Disabled}, nil
		}
	}
	return invitation.LibraryAccount{}, store.ErrNotFound
}
