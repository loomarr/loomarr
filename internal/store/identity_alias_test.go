package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store/identitystore"
	"github.com/loomarr/loomarr/internal/store/storedb"
)

// The store re-exports the identity cluster by alias and re-declares its sentinels as the same
// values. errors.Is compares by identity, so a copy that merely prints the same text would make
// every caller matching the other name silently false.
func TestIdentitySentinelsAreTheIdentitystoreValues(t *testing.T) {
	for _, c := range []struct {
		name         string
		store, inner error
		message      string
	}{
		{"ErrContactAddressConflict", ErrContactAddressConflict, identitystore.ErrContactAddressConflict, "store: contact address conflict"},
		{"ErrInvitationIdentityConflict", ErrInvitationIdentityConflict, identitystore.ErrInvitationIdentityConflict, "store: invitation identity conflict"},
		{"ErrNotFound", ErrNotFound, identitystore.ErrNotFound, "store: not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !errors.Is(c.store, c.inner) || !errors.Is(c.inner, c.store) {
				t.Fatalf("store.%s and identitystore.%s are not the same value", c.name, c.name)
			}
			if !errors.Is(fmt.Errorf("wrapped: %w", c.inner), c.store) {
				t.Fatalf("errors.Is(wrapped identitystore error, store.%s) = false", c.name)
			}
			if got := c.store.Error(); got != c.message {
				t.Fatalf("message = %q, want %q (a log string must not change)", got, c.message)
			}
		})
	}
	if !errors.Is(identitystore.ErrNotFound, storedb.ErrNotFound) {
		t.Fatal("identitystore.ErrNotFound is not the storedb.ErrNotFound value")
	}
}

// Real read paths: the sentinel is produced inside identitystore and must match the store's name.
func TestIdentityMissesMatchTheStoreSentinels(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t)
	if _, err := st.GetUser(ctx, "nobody"); !errors.Is(err, ErrNotFound) || !errors.Is(err, identitystore.ErrNotFound) {
		t.Fatalf("GetUser miss = %v, want store.ErrNotFound and identitystore.ErrNotFound", err)
	}
	if _, err := st.GetSession(ctx, "nope", time.Unix(1, 0)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession miss = %v, want store.ErrNotFound", err)
	}
	// A write refused inside the cluster for an unknown person.
	if err := st.CreateSession(ctx, Session{TokenHash: "h", UserID: "nobody", CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(9, 0)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateSession for an unknown user = %v, want store.ErrNotFound", err)
	}
}

// Types are aliases: values built under either name need no conversion, and the aggregate store
// returns the cluster's own types.
var (
	_ identitystore.User          = User{}
	_ identitystore.Session       = Session{}
	_ identitystore.SessionSeen   = SessionSeen{}
	_ identitystore.DevicePairing = DevicePairing{}
	_ identitystore.DeviceToken   = DeviceToken{}
	_ identitystore.Role          = RoleAdmin
	_ Role                        = identitystore.RoleMember
)

// The embedded cluster is what satisfies the identity role interfaces: the aggregate store hands
// out the same *identitystore.Store methods, with no forwarding layer in between.
func TestSQLStoreIdentityIsTheEmbeddedCluster(t *testing.T) {
	s, ok := adapterOf(newSQLiteStore(t))
	if !ok || s.Store == nil {
		t.Fatal("sqlStore has no identity cluster bound")
	}
	var _ UserStore = s.Store
	var _ InvitationStore = s.Store
	var _ PasswordRecoveryStore = s.Store
}
