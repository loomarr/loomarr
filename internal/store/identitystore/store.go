package identitystore

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/store/storedb"
)

// ErrNotFound is storedb.ErrNotFound, the kernel's value under this package's name, so errors.Is
// matches an error whichever of internal/store, identitystore or storedb produced it.
var ErrNotFound = storedb.ErrNotFound

// ErrContactAddressConflict means the normalized mailbox is already attached to another person
// or pending replacement. Public recovery must never resolve one address ambiguously.
var ErrContactAddressConflict = errors.New("store: contact address conflict")

// ErrInvitationIdentityConflict means an allowlist row or active Invitation already owns the
// reserved local username or exact Library account id.
var ErrInvitationIdentityConflict = errors.New("store: invitation identity conflict")

// Store is the identity cluster's persistence: users, sessions, devices, invitations, contact
// addresses, password recovery and notifications, over one storedb.Handle.
//
// A Store holds locks and must not be copied; build one with New and share the pointer.
type Store struct {
	h storedb.Handle
	// invitationLock serializes admission checks per identity (SQLite mutex, Postgres advisory
	// lock). One KeyedLock per lock domain, so the domains never share a SQLite mutex.
	invitationLock storedb.KeyedLock
	// recoveryLock serializes replacement of a person's pending recovery, the same way.
	recoveryLock storedb.KeyedLock
}

// New returns the identity store for h.
func New(h storedb.Handle) *Store { return &Store{h: h} }

// The kernel's codecs and scan shape under the names this cluster's statements already use.
type scannable = storedb.Scannable

func epoch(t time.Time) int64            { return storedb.Epoch(t) }
func fromEpoch(n int64) time.Time        { return storedb.FromEpoch(n) }
func isConstraintViolation(e error) bool { return storedb.IsConstraintViolation(e) }

// GetUserIn reads one user through q, which is the pool or a caller's transaction, so a
// transaction that must see the user inside its own snapshot (proposal approval's quota check)
// reads through the same statement as Store.GetUser. It returns ErrNotFound when absent.
func GetUserIn(ctx context.Context, q storedb.Querier, rebind storedb.Placeholder, id string) (User, error) {
	return scanUser(q.QueryRowContext(ctx, rebind(
		`SELECT id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at
		 FROM users WHERE id = ?`), id))
}

// InsertNotificationReferenceRecipient records that personID asked for the thing referenceKind/
// referenceID names, so a later notification about it reaches them. It runs on q (a caller's
// transaction) and is idempotent per (kind, id, person).
func InsertNotificationReferenceRecipient(
	ctx context.Context,
	q storedb.Querier,
	rebind storedb.Placeholder,
	referenceKind, referenceID, personID string,
	createdAt int64,
) error {
	_, err := q.ExecContext(ctx, rebind(`INSERT INTO notification_reference_recipients
		(reference_kind, reference_id, person_id, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (reference_kind, reference_id, person_id) DO NOTHING`),
		referenceKind, referenceID, personID, createdAt)
	return err
}
