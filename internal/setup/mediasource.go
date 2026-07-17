package setup

import (
	"context"
	"fmt"

	"github.com/mantonx/loomarr/internal/library"
)

// MediaSourceProgrammer is Tunarr's media-source surface (§6) — satisfied by
// *programmer.Tunarr and the testkit double. Wiring the media server as Tunarr's
// source is what lets Tunarr stream the files + index them into its program table.
type MediaSourceProgrammer interface {
	EnsureEmbySource(ctx context.Context, flavor, embyURL, token, userID string) (string, error)
	ConnectLibraries(ctx context.Context, sourceID string) (int, error)
	MediaLibrariesReady(ctx context.Context) (bool, error)
}

// UserLister resolves the media server's users (to find the admin id whose token
// Tunarr uses for the source). Satisfied by *library.Client.
type UserLister interface {
	ListUsers(ctx context.Context) ([]library.User, error)
}

// MediaSourceConnector wires the media server as *Tunarr's* media source using
// Loomarr's existing admin token — no separate Emby user login (§6). It backs
// POST /v1/setup/tunarr-connect and the tunarr_library setup check. Idempotent.
type MediaSourceConnector struct {
	users UserLister
	prog  MediaSourceProgrammer
	conn  func() (flavor, embyURL, token string) // live library connection (hot-applies)
}

// NewMediaSourceConnector builds the connector. conn resolves the CURRENT library
// flavor/url/token so a connection saved through the wizard takes effect live (§8.1).
func NewMediaSourceConnector(users UserLister, prog MediaSourceProgrammer, conn func() (string, string, string)) *MediaSourceConnector {
	return &MediaSourceConnector{users: users, prog: prog, conn: conn}
}

// Connect ensures Tunarr's media source exists (admin token as its access token),
// then enables + scans the movie/show libraries. Returns the source id + the number
// of movie/show libraries enabled. Idempotent — safe to re-run.
func (c *MediaSourceConnector) Connect(ctx context.Context) (sourceID string, librariesEnabled int, err error) {
	flavor, embyURL, token := c.conn()
	if embyURL == "" || token == "" {
		return "", 0, fmt.Errorf("media server not configured — set its URL + token first")
	}
	adminID, err := c.adminID(ctx)
	if err != nil {
		return "", 0, err
	}
	sourceID, err = c.prog.EnsureEmbySource(ctx, flavor, embyURL, token, adminID)
	if err != nil {
		return "", 0, fmt.Errorf("wire Tunarr media source: %w", err)
	}
	librariesEnabled, err = c.prog.ConnectLibraries(ctx, sourceID)
	if err != nil {
		return sourceID, librariesEnabled, fmt.Errorf("enable/scan Tunarr libraries: %w", err)
	}
	return sourceID, librariesEnabled, nil
}

// LibrariesReady reports whether Tunarr can already see the library (the
// tunarr_library setup check) — false ⇒ channels would degrade to flex/dead-air.
func (c *MediaSourceConnector) LibrariesReady(ctx context.Context) (bool, error) {
	return c.prog.MediaLibrariesReady(ctx)
}

// adminID returns the id of an enabled admin user on the media server — the user
// whose token authorizes Tunarr's source (§6).
func (c *MediaSourceConnector) adminID(ctx context.Context) (string, error) {
	users, err := c.users.ListUsers(ctx)
	if err != nil {
		return "", fmt.Errorf("list media-server users: %w", err)
	}
	for _, u := range users {
		if u.IsAdmin && !u.Disabled {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("no enabled admin user found on the media server")
}
