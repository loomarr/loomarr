package auth

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
)

// UserLister lists media-server users (the Phase-5 library adapter).
type UserLister interface {
	ListUsers(ctx context.Context) ([]library.User, error)
}

// UserSync imports/syncs users from the media server (§11): upserts each, and
// disables + revokes sessions for any user the server now reports disabled.
// Local role/quota/auto_approve are preserved (admin-managed); name + disabled
// track the source.
type UserSync struct {
	lib   UserLister
	store store.Store
	now   func() time.Time
}

// NewUserSync builds the user-sync service.
func NewUserSync(lib UserLister, st store.Store, now func() time.Time) *UserSync {
	if now == nil {
		now = time.Now
	}
	return &UserSync{lib: lib, store: st, now: now}
}

// Sync runs one import pass, returning how many users were upserted.
func (s *UserSync) Sync(ctx context.Context) (int, error) {
	msUsers, err := s.lib.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now()
	n := 0
	for _, ms := range msUsers {
		existing, err := s.store.GetUser(ctx, ms.ID)
		switch {
		case err == nil:
			wasEnabled := !existing.Disabled
			existing.Name = ms.Name
			existing.Disabled = ms.Disabled
			existing.UpdatedAt = now
			if err := s.store.UpsertUser(ctx, existing); err != nil {
				return n, err
			}
			// Newly disabled server-side → revoke sessions immediately (§11).
			if wasEnabled && ms.Disabled {
				if err := s.store.RevokeSessionsForUser(ctx, ms.ID); err != nil {
					return n, err
				}
			}
		case errors.Is(err, store.ErrNotFound):
			role := store.RoleMember
			if ms.IsAdmin {
				role = store.RoleAdmin
			}
			u := store.User{ID: ms.ID, Name: ms.Name, Role: role, Disabled: ms.Disabled, CreatedAt: now, UpdatedAt: now}
			if err := s.store.UpsertUser(ctx, u); err != nil {
				return n, err
			}
		default:
			return n, err
		}
		n++
	}
	return n, nil
}
