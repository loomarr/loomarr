// Package library serves the household's media library: its scan roots and scanner status, the
// items, the titles that need a look and the fixes for them, the *arr webhooks, the household's
// collections and ready-made sets, and the media server's collections and TMDB movie collections.
package library

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Store is the slice of the aggregate store these routes read directly: collections list the
// channels that use them.
type Store interface {
	store.ChannelStore
}

// Deps is what the library routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 409, 501 or empty, as the API's nil-semantics describe.
type Deps struct {
	Store Store
	// LiveSettings reads the TMDB key the movie-collection route needs.
	LiveSettings settings.Reader
	// The Library and Collections names match the api.Options fields they are copied from.
	LibraryRoots     LibraryRootService
	LibraryHooks     LibraryHookService
	Collections      CollectionService
	MovieCollections MovieCollectionService
	// LibraryImage renders one title image by hash, or nil while its bytes have not arrived.
	// nil leaves every poster and logo out.
	LibraryImage func(ctx context.Context, hash string) *httpcore.ImageDTO
	// CandidatePoster adopts a candidate's poster and renders it once its bytes are here.
	CandidatePoster func(ctx context.Context, itemID, url string) *httpcore.ImageDTO
}

// Handlers serves the library routes.
type Handlers struct {
	store             Store
	liveSettings      settings.Reader
	libraryRoots      LibraryRootService
	libraryHooks      LibraryHookService
	collections       CollectionService
	movieCollections  MovieCollectionService
	libraryImageFn    func(ctx context.Context, hash string) *httpcore.ImageDTO
	candidatePosterFn func(ctx context.Context, itemID, url string) *httpcore.ImageDTO
}

// libraryImage renders one of a title's images by hash: nil for none, or while its bytes have not
// arrived, so a surface draws without it rather than showing a broken image.
func (s *Handlers) libraryImage(ctx context.Context, hash string) *httpcore.ImageDTO {
	if s.libraryImageFn == nil || hash == "" {
		return nil
	}
	return s.libraryImageFn(ctx, hash)
}

// candidatePoster renders a candidate's poster once the image service has its bytes, or nil.
func (s *Handlers) candidatePoster(ctx context.Context, itemID, url string) *httpcore.ImageDTO {
	if s.candidatePosterFn == nil || url == "" {
		return nil
	}
	return s.candidatePosterFn(ctx, itemID, url)
}

// New returns the library handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		store: d.Store, liveSettings: d.LiveSettings, libraryRoots: d.LibraryRoots, libraryHooks: d.LibraryHooks,
		collections: d.Collections, movieCollections: d.MovieCollections,
		libraryImageFn: d.LibraryImage, candidatePosterFn: d.CandidatePoster,
	}
}

// Register mounts every route this package serves.
func (s *Handlers) Register(api huma.API) {
	s.registerLibrary(api) // also mounts the hooks, items, needs and household-collection routes
	s.registerCollections(api)
	s.registerMovieCollections(api)
}
