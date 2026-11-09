package httpcore

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// PersonNames resolves user ids to display names for a payload. One ListUsers per request (a
// household has a handful of accounts) rather than a lookup per row.
type PersonNames map[string]string

// UserLister is the one store read PersonNames needs.
type UserLister interface {
	ListUsers(ctx context.Context) ([]identitystore.User, error)
}

// LoadPersonNames reads every account's name once.
func LoadPersonNames(ctx context.Context, users UserLister) (PersonNames, error) {
	list, err := users.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	names := make(PersonNames, len(list))
	for _, u := range list {
		names[u.ID] = u.Name
	}
	return names, nil
}

// Name maps a stored user id to something a person can read. An unknown id yields "" rather than
// the id itself: the raw id is exactly what this exists to keep off screen.
func (n PersonNames) Name(id string) string { return n[id] }
