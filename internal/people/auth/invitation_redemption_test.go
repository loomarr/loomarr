package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/store/identitystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestInvitationRedemption_LocalCreatesArgon2idUserAndSession(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2030, 3, 17, 12, 0, 0, 0, time.UTC)
	invited := access.Invitation{
		ID: "invitation-local", Kind: access.KindLocal, Username: "Greta Finch",
		IdentityKey: access.NormalizeLocalIdentity("Greta Finch"), Role: access.RoleAdmin,
		Status: access.StatusPending, CreatedAt: at, ExpiresAt: at.Add(access.Expiry),
	}
	if err := st.CreateInvitation(ctx, invited, nil); err != nil {
		t.Fatal(err)
	}
	const bearer = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := st.ReplaceInvitationGrant(ctx, invited.ID, access.Grant{
		TokenHash: access.HashGrant(bearer), InvitationID: invited.ID,
		Kind: access.GrantActivation, Conveyance: access.ConveyanceCopy,
		CreatedAt: at, ExpiresAt: invited.ExpiresAt,
	}, at); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(st, time.Hour, func() time.Time { return at.Add(time.Minute) })
	svc := NewInvitationRedemptionService(st, nil, mgr, func() string { return "local-user" }, func() time.Time {
		return at.Add(time.Minute)
	})

	preview, err := svc.Preview(ctx, bearer)
	if err != nil || preview.ID != invited.ID || preview.Username != invited.Username || preview.Role != invited.Role {
		t.Fatalf("Preview = %+v, %v", preview, err)
	}
	if _, err := st.GetUser(ctx, "local-user"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("preview admitted user: %v", err)
	}

	token, expires, user, err := svc.RedeemLocal(ctx, bearer, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "local-user" || user.Name != invited.Username || user.Role != identitystore.RoleAdmin ||
		user.MediaServerLinked || !verifyPassword(user.PasswordHash, "correct horse battery staple") {
		t.Fatalf("redeemed user = %+v", user)
	}
	if expires != at.Add(time.Minute).Add(time.Hour) {
		t.Fatalf("session expiry = %v", expires)
	}
	resolved, err := mgr.Resolve(ctx, token, "")
	if err != nil || resolved.ID != user.ID {
		t.Fatalf("session resolve = %+v, %v", resolved, err)
	}
}

func TestInvitationRedemption_LibraryFailuresNeverFallbackOrConsumeInvitation(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2030, 3, 17, 12, 0, 0, 0, time.UTC)
	invited := access.Invitation{
		ID: "invitation-library-failures", Kind: access.KindLibrary, LibraryUserID: "selected-id",
		DisplayName: "Selected", IdentityKey: "selected-id", Role: access.RoleMember,
		Status: access.StatusPending, CreatedAt: at, ExpiresAt: at.Add(access.Expiry),
	}
	if err := st.CreateInvitation(ctx, invited, nil); err != nil {
		t.Fatal(err)
	}
	const bearer = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if err := st.ReplaceInvitationGrant(ctx, invited.ID, access.Grant{
		TokenHash: access.HashGrant(bearer), InvitationID: invited.ID,
		Kind: access.GrantActivation, Conveyance: access.ConveyanceEmail,
		CreatedAt: at, ExpiresAt: invited.ExpiresAt,
	}, at); err != nil {
		t.Fatal(err)
	}
	mediaServer := testkit.NewMediaServer(t)
	t.Cleanup(mediaServer.Close)
	mediaServer.Accounts = map[string]testkit.Account{
		"Selected": {Password: "provider-password", ID: "different-id"},
	}
	provider := library.New(library.Emby, mediaServer.URL, mediaServer.AdminToken, "invitation-device")
	mgr := NewManager(st, time.Hour, func() time.Time { return at.Add(time.Minute) })
	svc := NewInvitationRedemptionService(st, provider, mgr, func() string { return "unused" }, func() time.Time {
		return at.Add(time.Minute)
	})

	if _, _, _, err := svc.RedeemLibrary(ctx, bearer, "Selected", "provider-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("mismatched successful identity = %v, want ErrInvalidCredentials", err)
	}
	mediaServer.Accounts["Selected"] = testkit.Account{Password: "provider-password", ID: "selected-id"}
	mediaServer.AuthStatus = http.StatusServiceUnavailable
	if _, _, _, err := svc.RedeemLibrary(ctx, bearer, "Selected", "provider-password"); !errors.Is(err, ErrInvitationProviderUnavailable) {
		t.Fatalf("provider outage = %v, want ErrInvitationProviderUnavailable", err)
	}
	mediaServer.AuthStatus = http.StatusUnauthorized
	if _, _, _, err := svc.RedeemLibrary(ctx, bearer, "Selected", "provider-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("provider rejection = %v, want ErrInvalidCredentials", err)
	}

	if _, err := svc.Preview(ctx, bearer); err != nil {
		t.Fatalf("failed provider proof consumed invitation: %v", err)
	}
	if _, err := st.GetUser(ctx, "selected-id"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed provider proof admitted user: %v", err)
	}
}

func TestInvitationRedemption_LibraryProvesPinnedIdentityAndStoresOfflineVerifier(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	at := time.Date(2030, 3, 17, 12, 0, 0, 0, time.UTC)
	invited := access.Invitation{
		ID: "invitation-library", Kind: access.KindLibrary, LibraryUserID: "emby-grace",
		DisplayName: "Grace", IdentityKey: "emby-grace", Role: access.RoleMember,
		Status: access.StatusPending, CreatedAt: at, ExpiresAt: at.Add(access.Expiry),
	}
	if err := st.CreateInvitation(ctx, invited, nil); err != nil {
		t.Fatal(err)
	}
	const bearer = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := st.ReplaceInvitationGrant(ctx, invited.ID, access.Grant{
		TokenHash: access.HashGrant(bearer), InvitationID: invited.ID,
		Kind: access.GrantActivation, Conveyance: access.ConveyanceQR,
		CreatedAt: at, ExpiresAt: invited.ExpiresAt,
	}, at); err != nil {
		t.Fatal(err)
	}
	mediaServer := testkit.NewMediaServer(t)
	t.Cleanup(mediaServer.Close)
	mediaServer.Accounts = map[string]testkit.Account{
		"Grace": {Password: "provider-password", ID: "emby-grace", IsAdmin: true},
	}
	provider := library.New(library.Emby, mediaServer.URL, mediaServer.AdminToken, "invitation-device")
	mgr := NewManager(st, time.Hour, func() time.Time { return at.Add(time.Minute) })
	svc := NewInvitationRedemptionService(st, provider, mgr, func() string { return "must-not-be-used" }, func() time.Time {
		return at.Add(time.Minute)
	})

	token, _, user, err := svc.RedeemLibrary(ctx, bearer, "Grace", "provider-password")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != invited.LibraryUserID || user.Name != "Grace" || user.Role != identitystore.RoleMember ||
		!user.MediaServerLinked || !verifyPassword(user.PasswordHash, "provider-password") {
		t.Fatalf("redeemed user = %+v", user)
	}
	resolved, err := mgr.Resolve(ctx, token, "")
	if err != nil || resolved.ID != invited.LibraryUserID {
		t.Fatalf("session resolve = %+v, %v", resolved, err)
	}
}
