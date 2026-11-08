package auth

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Loomarr owns the disabled flag (#1861). Sync may disable a user the media server reports
// disabled, but it never clears a disable: re-enabling is an admin action in Loomarr.
func TestSyncNeverClearsADisable(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return now }
	newSync := func(t *testing.T) (*testkit.MediaServer, *UserSync, *LoginService) {
		t.Helper()
		st := newStore(t)
		ms := testkit.NewMediaServer(t)
		t.Cleanup(ms.Close)
		lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
		importOne(t, st, "u-bob", "bob", false)
		return ms, NewUserSync(lib, st, clock), NewLoginService(lib, st, NewManager(st, time.Hour, clock), nil, clock)
	}

	t.Run("an admin disable survives a sync that reports the user enabled", func(t *testing.T) {
		ms, sync, svc := newSync(t)
		if err := svc.Disable(ctx, "u-bob"); err != nil {
			t.Fatal(err)
		}
		ms.Users = []testkit.MediaServerUser{{ID: "u-bob", Name: "robert"}}
		if _, err := sync.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		u, err := sync.store.GetUser(ctx, "u-bob")
		if err != nil {
			t.Fatal(err)
		}
		if !u.Disabled {
			t.Error("sync re-enabled a user an admin disabled in Loomarr")
		}
		if u.Name != "robert" {
			t.Errorf("name = %q, want the synced %q (sync still refreshes the name)", u.Name, "robert")
		}
	})

	t.Run("a server-side disable sticks after the server re-enables the user", func(t *testing.T) {
		ms, sync, _ := newSync(t)
		ms.Users = []testkit.MediaServerUser{{ID: "u-bob", Name: "bob", Disabled: true}}
		if _, err := sync.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		if u, err := sync.store.GetUser(ctx, "u-bob"); err != nil || !u.Disabled {
			t.Fatalf("after a server-side disable: u-bob = %+v (%v), want disabled", u, err)
		}
		ms.Users = []testkit.MediaServerUser{{ID: "u-bob", Name: "bob"}}
		if _, err := sync.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		if u, err := sync.store.GetUser(ctx, "u-bob"); err != nil || !u.Disabled {
			t.Errorf("after the server re-enabled u-bob: %+v (%v), want still disabled until an admin re-enables", u, err)
		}
	})
}
