package library

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func newClient(t *testing.T, flavor Flavor) (*Client, *testkit.MediaServer) {
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	return New(flavor, ms.URL, ms.AdminToken, "loomarr-test-device"), ms
}

// Lookup returns present + item id for a title in the library (pinned fixture).
func TestLookupPresent(t *testing.T) {
	c, _ := newClient(t, Emby)
	id, present, err := c.Lookup(context.Background(), TMDB, "16153", Movie)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("expected present for tmdb.16153 (pinned present fixture)")
	}
	if id == "" {
		t.Error("present lookup returned empty item id")
	}
}

// Lookup returns absent (empty Items) for an unknown title.
func TestLookupAbsent(t *testing.T) {
	c, _ := newClient(t, Emby)
	_, present, err := c.Lookup(context.Background(), TMDB, "999999999", Movie)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("expected absent for an unknown provider id")
	}
}

// Flavor auth: Emby uses X-Emby-Token; Jellyfin uses Authorization: MediaBrowser.
func TestFlavorTokenHeaders(t *testing.T) {
	t.Run("emby", func(t *testing.T) {
		c, ms := newClient(t, Emby)
		_, _, _ = c.Lookup(context.Background(), TMDB, "16153", Movie)
		if ms.LastEmbyToken != ms.AdminToken {
			t.Errorf("Emby X-Emby-Token = %q, want %q", ms.LastEmbyToken, ms.AdminToken)
		}
		if ms.LastAuthHeader != "" {
			t.Errorf("Emby must not send Authorization header, got %q", ms.LastAuthHeader)
		}
	})
	t.Run("jellyfin", func(t *testing.T) {
		c, ms := newClient(t, Jellyfin)
		_, _, _ = c.Lookup(context.Background(), TMDB, "16153", Movie)
		if !strings.HasPrefix(ms.LastAuthHeader, "MediaBrowser ") {
			t.Errorf("Jellyfin Authorization = %q, want MediaBrowser scheme", ms.LastAuthHeader)
		}
		if !strings.Contains(ms.LastAuthHeader, `Token="`+ms.AdminToken+`"`) {
			t.Errorf("Jellyfin header missing token: %q", ms.LastAuthHeader)
		}
		if ms.LastEmbyToken != "" {
			t.Errorf("Jellyfin must not send X-Emby-Token, got %q", ms.LastEmbyToken)
		}
	})
}

// AuthenticateByName: the login header (client-id, no token yet) differs by
// flavor — Emby X-Emby-Authorization, Jellyfin Authorization: MediaBrowser.
func TestAuthenticateLoginHeader(t *testing.T) {
	t.Run("emby", func(t *testing.T) {
		c, ms := newClient(t, Emby)
		if _, err := c.AuthenticateByName(context.Background(), ms.GoodUser, ms.GoodPass); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ms.LastEmbyAuthz, `DeviceId="loomarr-test-device"`) {
			t.Errorf("Emby login header missing DeviceId: %q", ms.LastEmbyAuthz)
		}
	})
	t.Run("jellyfin", func(t *testing.T) {
		c, ms := newClient(t, Jellyfin)
		if _, err := c.AuthenticateByName(context.Background(), ms.GoodUser, ms.GoodPass); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(ms.LastAuthHeader, "MediaBrowser ") {
			t.Errorf("Jellyfin login header = %q, want MediaBrowser scheme", ms.LastAuthHeader)
		}
	})
}

// AuthenticateByName success maps User fields incl. admin/disabled (§11).
func TestAuthenticateSuccess(t *testing.T) {
	c, ms := newClient(t, Emby)
	u, err := c.AuthenticateByName(context.Background(), ms.GoodUser, ms.GoodPass)
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != ms.GoodUser || !u.IsAdmin || u.Disabled {
		t.Errorf("authenticated user = %+v, want admin non-disabled %q", u, ms.GoodUser)
	}
	if u.ID == "" {
		t.Error("authenticated user has empty id")
	}
}

// The §11/§19 negative path: bad password → error (real Emby 401, pinned fixture).
func TestAuthenticateBadPasswordErrors(t *testing.T) {
	c, ms := newClient(t, Emby)
	const password = "wrong-password-must-not-leak"
	_, err := c.AuthenticateByName(context.Background(), ms.GoodUser, password)
	if !errors.Is(err, ErrCredentialsRejected) {
		t.Fatalf("bad password error = %v, want ErrCredentialsRejected", err)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatal("authentication error leaked the submitted password")
	}
}

func TestAuthenticateProviderUnavailableErrors(t *testing.T) {
	c, ms := newClient(t, Emby)
	ms.AuthStatus = http.StatusServiceUnavailable
	_, err := c.AuthenticateByName(context.Background(), ms.GoodUser, ms.GoodPass)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("provider 503 error = %v, want ErrProviderUnavailable", err)
	}
}

// ListUsers parses the pinned /Users fixture into User structs with policy.
func TestListUsers(t *testing.T) {
	c, _ := newClient(t, Emby)
	users, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		t.Fatal("expected users from the pinned /Users fixture")
	}
	for _, u := range users {
		if u.ID == "" || u.Name == "" {
			t.Errorf("user missing id/name: %+v", u)
		}
	}
}

func TestParseFlavor(t *testing.T) {
	for _, ok := range []string{"emby", "jellyfin"} {
		if _, err := ParseFlavor(ok); err != nil {
			t.Errorf("ParseFlavor(%q) errored: %v", ok, err)
		}
	}
	if _, err := ParseFlavor("plex"); err == nil {
		t.Error("ParseFlavor(plex) should error")
	}
}
