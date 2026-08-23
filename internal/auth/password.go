package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/loomarr/loomarr/internal/store"
)

// Local account management (§11). Two operations the identity model implied but
// never had: a local user changing their own password, and an admin creating a
// second local account.
//
// Until now the ONLY way a local user came into existence was `POST
// /v1/setup/bootstrap`, which succeeds exactly once — so the password chosen in
// wizard step 1 was permanent, and an install could never gain a second local
// account. Meanwhile the users list labels every row `local` / `Media-server
// account` expressly "to explain why a password action applies to some users and
// not others" — for an action that did not exist.
//
// The §11 invariants are unchanged by any of this:
//   - The `users` table stays the allowlist and the source of truth.
//   - A local user is one WITH a password hash; an imported one has none. Creating
//     a local account adds a row, exactly like import does — it is not a new way to
//     bypass the allowlist.
//   - Media-server users have no Loomarr-side password to change. Attempting it is
//     an error, not a silent no-op that would imply their media-server password had
//     been altered.

var (
	// ErrWrongPassword — the current password didn't verify. Deliberately distinct
	// from "no such user" internally, but callers must surface both identically.
	ErrWrongPassword = errors.New("auth: current password does not match")
	// ErrNotLocalUser — the account authenticates against the media server, so
	// Loomarr holds no hash it could change.
	ErrNotLocalUser = errors.New("auth: not a local account")
	// ErrWeakPassword — below the minimum length.
	ErrWeakPassword = errors.New("auth: password too short")
	// ErrDuplicateUsername — a user with that name already exists.
	ErrDuplicateUsername = errors.New("auth: username already taken")
)

// MinPasswordLen is the floor for a Loomarr-stored password. Deliberately modest:
// this is a household appliance behind a homelab network, and a long minimum
// pushes people toward reuse. bcrypt handles the rest.
const MinPasswordLen = 8

// PasswordService owns local-credential mutations. Split from LoginService because
// that one verifies credentials and this one changes them — different authority.
type PasswordService struct {
	store store.Store
	newID IDGen
	now   func() time.Time
}

// NewPasswordService builds the local-account service.
func NewPasswordService(st store.Store, newID IDGen, now func() time.Time) *PasswordService {
	if now == nil {
		now = time.Now
	}
	return &PasswordService{store: st, newID: newID, now: now}
}

// ChangePassword verifies the caller's CURRENT password before setting a new one,
// then revokes every other session.
//
// Requiring the current password matters even though the caller already holds a
// valid session: a session is "this browser was logged in at some point", which is
// exactly what an unattended laptop or a stolen cookie also looks like. Proving
// knowledge of the password is what makes it the account owner rather than whoever
// is sitting at the machine.
//
// EVERY session dies on success, including the caller's. A change made because of a
// suspected compromise must actually end the intruder's access, and "all sessions
// except the one making the request" is a weaker guarantee that depends on the
// request itself being trustworthy. One re-login is a small price; the alternative
// is a password change that leaves the attacker signed in.
func (s *PasswordService) ChangePassword(ctx context.Context, userID, current, next string) error {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if u.PasswordHash == "" {
		return ErrNotLocalUser // imported: the media server owns this credential
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(current)); err != nil {
		return ErrWrongPassword
	}
	if len(next) < MinPasswordLen {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bootstrapCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = string(hash)
	u.UpdatedAt = s.now()
	if err := s.store.UpsertUser(ctx, u); err != nil {
		return err
	}
	// NOTE for the Account UI: the v2 mock's copy reads "other sessions revoked, this
	// one kept". That is the friendlier promise but the weaker guarantee — keeping a
	// session means trusting the request that might itself be the compromise. The
	// copy should follow the code here, not the other way round.
	return s.store.RevokeSessionsForUser(ctx, userID)
}

// CreateLocal adds a local account: a `users` row WITH a password hash. Admin-only
// at the route layer (§11 — roles gate the actions that spend real resources, and
// minting an identity is one).
//
// This is the same kind of write as `POST /v1/users/import`: both add an allowlist
// row an admin explicitly asked for. The difference is only which credential path
// the row carries — a bcrypt hash here, none for an imported media-server account.
func (s *PasswordService) CreateLocal(ctx context.Context, username, password string, role store.Role, quota int) (store.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return store.User{}, ErrInvalidBootstrap
	}
	if len(password) < MinPasswordLen {
		return store.User{}, ErrWeakPassword
	}
	// Reject a name that CASE-INSENSITIVELY collides with an existing one.
	//
	// Login resolves names case-sensitively (`WHERE name = ?`), so `owner` and
	// `OWNER` are technically distinct accounts and creating both would "work". They
	// would also be indistinguishable in the users list, in a support conversation,
	// and to the person typing one at the login box. Creation is the right place to
	// refuse that — tightening what we mint, without changing how login resolves
	// what already exists.
	existing, err := s.store.ListUsers(ctx)
	if err != nil {
		return store.User{}, err
	}
	for _, e := range existing {
		if strings.EqualFold(strings.TrimSpace(e.Name), username) {
			return store.User{}, ErrDuplicateUsername
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bootstrapCost)
	if err != nil {
		return store.User{}, fmt.Errorf("hash password: %w", err)
	}
	now := s.now()
	u := store.User{
		ID:           s.newID(),
		Name:         username,
		Role:         role,
		Quota:        quota,
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.UpsertUser(ctx, u); err != nil {
		return store.User{}, err
	}
	return u, nil
}

// ResetPassword lets an ADMIN set another local user's password without knowing the
// current one — the "someone forgot theirs" path. Distinct from ChangePassword on
// purpose: that one proves possession, this one relies on the caller's admin role,
// so they must not share an implementation.
//
// Every session for the target user is revoked: an admin resetting a password is
// either onboarding someone or responding to a problem, and in both cases the old
// sessions should not survive.
func (s *PasswordService) ResetPassword(ctx context.Context, targetUserID, next string) error {
	u, err := s.store.GetUser(ctx, targetUserID)
	if err != nil {
		return err
	}
	if u.PasswordHash == "" {
		return ErrNotLocalUser // imported: Loomarr never had this credential to reset
	}
	if len(next) < MinPasswordLen {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bootstrapCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u.PasswordHash = string(hash)
	u.UpdatedAt = s.now()
	if err := s.store.UpsertUser(ctx, u); err != nil {
		return err
	}
	return s.store.RevokeSessionsForUser(ctx, targetUserID)
}
