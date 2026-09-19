// Package moviecollections resolves authoritative TMDB movie-collection rosters
// for a bounded set of provisioned movie Keys.
package moviecollections

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// Source is the internal seam over TMDB's movie-detail and collection-detail
// operations. CollectionForMovie distinguishes an authoritative standalone
// (zero ID, nil error) from an unresolved lookup (error).
type Source interface {
	CollectionForMovie(context.Context, int) (CollectionRef, error)
	MovieCollection(context.Context, int) (SourceCollection, error)
}

type CollectionRef struct {
	TMDBID int
	Name   string
}

type SourceCollection struct {
	TMDBID  int
	Name    string
	Members []catalog.Candidate
}

type Collection struct {
	TMDBID  int
	Name    string
	Members []catalog.Candidate
}

type Resolution struct {
	Collections []Collection
	Complete    bool
}

type Resolver struct {
	source   Source
	presence catalog.LibraryPresenceSource
}

func New(source Source) *Resolver {
	return &Resolver{source: source}
}

func (r *Resolver) WithPresenceSource(source catalog.LibraryPresenceSource) *Resolver {
	r.presence = source
	return r
}

func (r *Resolver) Resolve(ctx context.Context, keys []provision.Key) (Resolution, error) {
	resolution := Resolution{Collections: make([]Collection, 0), Complete: true}
	if r.source == nil {
		return resolution, errors.New("movie collection source is not configured")
	}

	seenMovies := make(map[int]bool, len(keys))
	refs := make(map[int]CollectionRef)
	collectionOrder := make([]int, 0)
	resolvedMovies := 0
	var firstErr error
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return resolution, err
		}
		mediaType, provider, movieID, ok := provision.ParseKey(key)
		if !ok || mediaType != provision.Movie || provider != "tmdb" || movieID <= 0 {
			resolution.Complete = false
			continue
		}
		if seenMovies[movieID] {
			continue
		}
		seenMovies[movieID] = true
		ref, err := r.source.CollectionForMovie(ctx, movieID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return resolution, ctxErr
			}
			resolution.Complete = false
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resolvedMovies++
		if ref.TMDBID <= 0 {
			continue // authoritative standalone movie
		}
		if _, exists := refs[ref.TMDBID]; exists {
			continue
		}
		refs[ref.TMDBID] = ref
		collectionOrder = append(collectionOrder, ref.TMDBID)
	}
	if resolvedMovies == 0 && firstErr != nil {
		return resolution, firstErr
	}

	var presence catalog.LibraryPresence
	if r.presence != nil {
		presence = r.presence()
	}
	detailResponses := 0
	for _, collectionID := range collectionOrder {
		if err := ctx.Err(); err != nil {
			return resolution, err
		}
		sourceCollection, err := r.source.MovieCollection(ctx, collectionID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return resolution, ctxErr
			}
			resolution.Complete = false
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		detailResponses++
		if sourceCollection.TMDBID != collectionID {
			resolution.Complete = false
			continue
		}
		members, rosterComplete := validMembers(sourceCollection.Members)
		if !rosterComplete {
			resolution.Complete = false
		}
		if len(members) < 2 {
			continue // a one-title roster offers no collection-level choice
		}
		for i := range members {
			// TMDB is authoritative for collection membership, not Library
			// ownership. Only the current presence snapshot may set these fields.
			members[i].InLibrary = false
			members[i].LibraryItemID = ""
		}
		if presence != nil {
			for i := range members {
				owned, ok, presenceErr := presence.Present(ctx, provision.Movie, members[i].TMDBID, 0)
				if presenceErr != nil {
					resolution.Complete = false
					continue
				}
				if !ok {
					continue
				}
				members[i].InLibrary = true
				members[i].LibraryItemID = owned.LibraryItemID
				if members[i].OfficialRating == "" {
					members[i].OfficialRating = owned.OfficialRating
				}
				if len(members[i].Genres) == 0 {
					members[i].Genres = append([]string(nil), owned.Genres...)
				}
			}
		}
		name := strings.TrimSpace(sourceCollection.Name)
		if name == "" {
			name = strings.TrimSpace(refs[collectionID].Name)
		}
		if name == "" {
			resolution.Complete = false
			continue
		}
		resolution.Collections = append(resolution.Collections, Collection{
			TMDBID: collectionID, Name: name, Members: members,
		})
	}
	if len(collectionOrder) > 0 && detailResponses == 0 && firstErr != nil {
		return resolution, firstErr
	}
	return resolution, nil
}

func validMembers(candidates []catalog.Candidate) ([]catalog.Candidate, bool) {
	members := make([]catalog.Candidate, 0, len(candidates))
	seen := make(map[int]bool, len(candidates))
	complete := true
	for _, candidate := range candidates {
		if candidate.MediaType != provision.Movie || candidate.TMDBID <= 0 || strings.TrimSpace(candidate.Name) == "" || seen[candidate.TMDBID] {
			complete = false
			continue
		}
		seen[candidate.TMDBID] = true
		candidate.Source = catalog.ScopeTMDB
		members = append(members, candidate)
	}
	sort.SliceStable(members, func(i, j int) bool {
		left, right := members[i], members[j]
		switch {
		case left.Year == right.Year:
			return false // source order retains exact release dates within the year
		case left.Year == 0:
			return false
		case right.Year == 0:
			return true
		default:
			return left.Year < right.Year
		}
	})
	return members, complete
}
