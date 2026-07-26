package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
)

// ErrBootstrapClosed is returned when bootstrap is attempted after an admin
// already exists (§11: first-run only, exactly once).
var ErrBootstrapClosed = errors.New("bootstrap already completed")

// ErrInvalidBootstrap is returned for an empty username/password.
var ErrInvalidBootstrap = errors.New("username and password are required")

// bootstrapCost is the bcrypt cost for stored passwords. DefaultCost (10) is the right
// balance for a homelab.
//
// ⚠ A var, not a const, ONLY so the test suite can lower it — see TestMain in this package.
// Production never changes it, and nothing reads it from config: a cost knob an operator
// could turn down is a way to weaken stored credentials by accident, which is exactly the
// kind of "safety for convenience" trade §3 of CLAUDE.md forbids.
//
// Why it is worth the seam: bcrypt is deliberately CPU-bound, and under `-race` on a CI
// runner that cost dominates the entire Go job. Measured on this repo — `internal/api` +
// `internal/auth` take 79s with `-race` and 3s without, and CI's `internal/api` alone was
// 102s. The hashing being slow is the POINT in production and pure waste in a test that
// only needs "the hash verifies".
var bootstrapCost = bcrypt.DefaultCost

// IDGen mints a unique id for a local user (a Loomarr-owned id space, distinct
// from media-server ids). Injected for determinism in tests.
type IDGen func() string

// Provisioner owns local-admin bootstrap and explicit media-server import (§11).
// It is the ONLY path that creates users — login never does (the allowlist).
type Provisioner struct {
	store store.Store
	lib   UserLister // may be nil (import unavailable without a media server)
	newID IDGen
	now   func() time.Time
}

// NewProvisioner builds the bootstrap+import service (lib is the media-server
// user lister shared with UserSync; nil disables import).
func NewProvisioner(st store.Store, lib UserLister, newID IDGen, now func() time.Time) *Provisioner {
	if now == nil {
		now = time.Now
	}
	return &Provisioner{store: st, lib: lib, newID: newID, now: now}
}

// Bootstrap creates the first local admin (§11), succeeding only while no admin
// exists — the first success closes the door (a later call → ErrBootstrapClosed).
// The password is bcrypt-hashed; a local id is minted (never a media-server id).
func (p *Provisioner) Bootstrap(ctx context.Context, username, password string) (store.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return store.User{}, ErrInvalidBootstrap
	}
	// Gate on "no admin exists yet". This is a check-then-act; a UNIQUE-less users
	// table can't enforce single-bootstrap at the DB, but bootstrap is a rare
	// first-run action — the CountAdmins gate + the first-writer-wins on the id
	// PK is sufficient (two concurrent bootstraps both pass the gate only in a
	// race window measured in ms on a fresh install; the second's distinct id
	// would create a second admin, acceptable and visible in the users list).
	n, err := p.store.CountAdmins(ctx)
	if err != nil {
		return store.User{}, err
	}
	if n > 0 {
		return store.User{}, ErrBootstrapClosed
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bootstrapCost)
	if err != nil {
		return store.User{}, fmt.Errorf("hash password: %w", err)
	}
	now := p.now()
	u := store.User{
		ID:           p.newID(),
		Name:         username,
		Role:         store.RoleAdmin,
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := p.store.UpsertUser(ctx, u); err != nil {
		return store.User{}, err
	}
	return u, nil
}

// Bootstrapped reports whether the install has an owning admin yet — the single
// fact behind the unauthenticated GET /v1/setup/state (§7). It is the same
// CountAdmins gate Bootstrap enforces, exposed as a read so the frontend can send
// a first-run visitor to the wizard rather than to a login they cannot pass.
func (p *Provisioner) Bootstrapped(ctx context.Context) (bool, error) {
	n, err := p.store.CountAdmins(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Candidate is a media-server account an admin may import (§11), with whether it
// is already allowlisted so a picker can show it as done rather than hiding it.
type Candidate struct {
	ID       string
	Name     string
	IsAdmin  bool
	Disabled bool
	Imported bool
}

// Candidates lists the media-server accounts available to import, flagged against
// the local allowlist (§11). This is the read side of explicit import: without it an
// admin would have to know raw media-server user ids to call Import at all. It never
// creates anything — listing is not provisioning.
func (p *Provisioner) Candidates(ctx context.Context) ([]Candidate, error) {
	if p.lib == nil {
		return nil, errors.New("no media server configured")
	}
	serverUsers, err := p.lib.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list media-server users: %w", err)
	}
	out := make([]Candidate, 0, len(serverUsers))
	for _, su := range serverUsers {
		c := Candidate{ID: su.ID, Name: su.Name, IsAdmin: su.IsAdmin, Disabled: su.Disabled}
		if _, err := p.store.GetUser(ctx, su.ID); err == nil {
			c.Imported = true
		}
		out = append(out, c)
	}
	return out, nil
}

// Import upserts the named media-server users as allowlisted rows (§11), the ONLY
// way a media-server user gains access. ids are media-server user ids; makeAdmin
// grants admin to those that are media-server admins (else member). Returns the
// count imported. Un-listed ids are skipped (not invented). Existing local users
// are never touched (import only concerns media-server ids).
func (p *Provisioner) Import(ctx context.Context, ids []string, makeAdmin bool) (int, error) {
	if p.lib == nil {
		return 0, errors.New("no media server configured for import")
	}
	serverUsers, err := p.lib.ListUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("list media-server users: %w", err)
	}
	byID := make(map[string]library.User, len(serverUsers))
	for _, su := range serverUsers {
		byID[su.ID] = su
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	now := p.now()
	imported := 0
	for id := range want {
		su, ok := byID[id]
		if !ok {
			continue // asked to import an id the server doesn't have — skip, never invent
		}
		role := store.RoleMember
		if makeAdmin && su.IsAdmin {
			role = store.RoleAdmin
		}
		// Preserve an existing row's Loomarr-owned role/quota; only (re)assert
		// identity + disabled. A brand-new import creates the row.
		u := store.User{
			ID: su.ID, Name: su.Name, Role: role, Disabled: su.Disabled,
			CreatedAt: now, UpdatedAt: now,
		}
		if existing, gerr := p.store.GetUser(ctx, su.ID); gerr == nil {
			u.Role = existing.Role // keep the admin's earlier role decision
			u.Quota = existing.Quota
			u.AutoApprove = existing.AutoApprove
			u.CreatedAt = existing.CreatedAt
		}
		if err := p.store.UpsertUser(ctx, u); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}
