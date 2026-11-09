package library

import (
	"context"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// MovieCollectionService resolves authoritative TMDB movie-collection rosters
// for the bounded Title Keys visible in proposal review. The returned members
// are ordinary grounded candidates so choosing them still uses the existing
// per-title approval gate.
type MovieCollectionService interface {
	ResolveMovieCollections(context.Context, MovieCollectionRequest) (MovieCollectionResolution, error)
}

type MovieCollectionRequest struct {
	Keys []string
}

type MovieCollectionResolution struct {
	Collections []MovieCollection `json:"collections"`
	// Complete is false when a requested Key or referenced roster could not be
	// resolved or validated. That keeps incomplete evidence distinct from an
	// authoritative standalone.
	Complete bool `json:"complete"`
}

type MovieCollection struct {
	TMDBID  int                        `json:"tmdbId" minimum:"1"`
	Name    string                     `json:"name" minLength:"1"`
	Members []httpcore.SearchCandidate `json:"members" minItems:"2"`
}

// CollectionService backs GET /v1/library/collections — the media server's collections
// (Emby/Jellyfin BoxSets) that back `scope.collections` (programming-design §2.2).
//
// ⚠ A LIST, not a search. Collections are a small closed set the operator curates by hand,
// so the picker shows all of them; there is no query parameter because there is nothing to
// narrow. This is why it is not a `scope` on /v1/search — a Candidate models a provisionable
// title and a collection is not one, the same distinction that keeps clips off that route.
type CollectionService interface {
	Collections(ctx context.Context) ([]LibraryCollection, error)
}

// LibraryCollection is the API view of one media-server collection. The ID is opaque and
// server-local — it is what `scope.collections` stores — so the Name is carried alongside it
// purely so the picker can show a human something. ⚠ Never a TMDB collection (franchise) id;
// see programming-design §2.2 on the two namespaces sharing one English word.
type LibraryCollection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ChildCount int    `json:"childCount,omitempty"`
}
