// Package catalog serves the title catalog: the provisioning titles under /v1/titles and the
// federated catalog search under /v1/search.
package catalog

import (
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Store is the slice of the aggregate store these routes read and write.
type Store interface {
	store.TitleStore
	store.ChannelStore
}

// Deps is what the catalog routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Store Store
	// LiveSettings reads the TMDB key the search scopes depend on.
	LiveSettings settings.Reader
	// Search is the catalog behind /v1/search.
	Search SearchService
	// Now is the request clock; nil is time.Now.
	Now func() time.Time
}

// Handlers serves the catalog routes.
type Handlers struct {
	store        Store
	liveSettings settings.Reader
	search       SearchService
	now          func() time.Time
}

// New returns the catalog handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{store: d.Store, liveSettings: d.LiveSettings, search: d.Search, now: d.Now}
}

// clock is the request clock: the pinned one in tests, time.Now otherwise.
func (s *Handlers) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Register mounts every route this package serves.
func (s *Handlers) Register(api huma.API) {
	s.registerTitles(api)
	s.registerSearch(api)
}
