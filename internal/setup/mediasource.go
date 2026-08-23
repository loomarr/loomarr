package setup

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/library"
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

// MediaSourceLibrary is one bound library connection used by a media-source
// operation. Connection and ListUsers must describe the same immutable snapshot.
type MediaSourceLibrary interface {
	UserLister
	Connection() library.Connection
}

// MediaSourceLibrarySource starts one media-source operation. Production returns
// library.Client.Snapshot; fixed tests and adapters return the same value each time.
type MediaSourceLibrarySource func() MediaSourceLibrary

// MediaSourceConnector wires the media server as *Tunarr's* media source using
// Loomarr's existing admin token — no separate Emby user login (§6). It backs
// POST /v1/setup/tunarr-connect and the tunarr_library setup check. Idempotent.
type MediaSourceConnector struct {
	library MediaSourceLibrarySource
	prog    MediaSourceProgrammer
}

// NewMediaSourceConnector builds a live connector. library starts one immutable
// library operation, coupling flavor, URL, token, and user listing across rotation.
func NewMediaSourceConnector(library MediaSourceLibrarySource, prog MediaSourceProgrammer) *MediaSourceConnector {
	return &MediaSourceConnector{library: library, prog: prog}
}

// Connect ensures Tunarr's media source exists (admin token as its access token),
// then enables + scans the movie/show libraries. Returns the source id + the number
// of movie/show libraries enabled. Idempotent — safe to re-run.
func (c *MediaSourceConnector) Connect(ctx context.Context) (sourceID string, librariesEnabled int, err error) {
	operation := c.library()
	connection := operation.Connection()
	if connection.BaseURL == "" || connection.Token == "" {
		return "", 0, fmt.Errorf("media server not configured — set its URL + token first")
	}
	adminID, err := adminID(ctx, operation)
	if err != nil {
		return "", 0, err
	}
	sourceID, err = c.prog.EnsureEmbySource(ctx, string(connection.Flavor), connection.BaseURL, connection.Token, adminID)
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
func adminID(ctx context.Context, users UserLister) (string, error) {
	found, err := users.ListUsers(ctx)
	if err != nil {
		return "", fmt.Errorf("list media-server users: %w", err)
	}
	for _, u := range found {
		if u.IsAdmin && !u.Disabled {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("no enabled admin user found on the media server")
}
