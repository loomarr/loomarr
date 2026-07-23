// Package library is the Library port (design §6, §2 boundaries): a shared
// Emby/Jellyfin adapter. Both flavors share every endpoint and response shape;
// they differ ONLY in how auth rides on the request, so flavor-specific code is
// isolated to header construction (§6, §11) and everything else is one impl.
package library

import (
	"context"
	"fmt"
	"time"
)

// Flavor selects the media-server dialect (§15 LIBRARY_FLAVOR).
type Flavor string

const (
	Emby     Flavor = "emby"
	Jellyfin Flavor = "jellyfin"
)

// ParseFlavor validates the configured flavor.
func ParseFlavor(s string) (Flavor, error) {
	switch Flavor(s) {
	case Emby:
		return Emby, nil
	case Jellyfin:
		return Jellyfin, nil
	default:
		return "", fmt.Errorf("unknown LIBRARY_FLAVOR %q (want emby|jellyfin)", s)
	}
}

// User is a media-server account as needed for import/sync and roles (§11).
type User struct {
	ID       string // media-server user id — the local key (§11)
	Name     string
	IsAdmin  bool
	Disabled bool
}

// Library is the port the provisioner and auth layer depend on (§2). Concrete
// impl is the flavored Emby/Jellyfin client below.
type Library interface {
	// Lookup reports whether a title is present in the library and its item id
	// (§6). Presence is signaled by a non-empty result, never by status code.
	Lookup(ctx context.Context, providerKind ProviderKind, providerID string, mediaType MediaType) (itemID string, present bool, err error)

	// AuthenticateByName verifies a user's credentials against the media server
	// (§11 login). Returns the authenticated User on success. The media-server
	// access token is used only to confirm + best-effort logout; callers issue
	// their own session (Phase 9).
	AuthenticateByName(ctx context.Context, username, password string) (User, error)

	// ListUsers returns all media-server users for import/sync (§11), using the
	// admin LIBRARY_TOKEN.
	ListUsers(ctx context.Context) ([]User, error)
}

// LibraryScanner is the narrow port the poll-based availability scan depends on (§4, §18.1):
// bulk reads of the library with provider ids, so the scan job confirms every in-flight title
// in one call instead of an N-lookup storm. Segregated from Library so consumers that only
// need presence (ingest) or auth don't take on the scan surface; the concrete *Client
// satisfies both. RecentlyAdded is the incremental path; AllItems the periodic full sweep.
type LibraryScanner interface {
	RecentlyAdded(ctx context.Context, since time.Time) ([]SearchResult, error)
	AllItems(ctx context.Context) ([]SearchResult, error)
}

// ProviderKind is the external id namespace used in a Lookup (§6).
type ProviderKind string

const (
	TMDB ProviderKind = "tmdb"
	TVDB ProviderKind = "tvdb"
)

// MediaType maps to the media server's IncludeItemTypes (§6).
type MediaType string

const (
	Movie  MediaType = "Movie"
	Series MediaType = "Series"
)
