// Package moviecollectionsfixture provides the shared deterministic source for
// movie-collection resolver tests. It records authoritative reference and
// roster lookups separately so tests can assert bounded, deduplicated access.
package moviecollectionsfixture

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/library/tmdb"
)

type Source struct {
	mu               sync.Mutex
	Refs             map[int]tmdb.CollectionRef
	Collections      map[int]tmdb.SourceCollection
	RefErrors        map[int]error
	CollectionErrors map[int]error
	refCalls         []int
	collectionCalls  []int
}

func (s *Source) CollectionForMovie(_ context.Context, movieID int) (tmdb.CollectionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refCalls = append(s.refCalls, movieID)
	if err := s.RefErrors[movieID]; err != nil {
		return tmdb.CollectionRef{}, err
	}
	return s.Refs[movieID], nil
}

func (s *Source) MovieCollection(_ context.Context, collectionID int) (tmdb.SourceCollection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collectionCalls = append(s.collectionCalls, collectionID)
	if err := s.CollectionErrors[collectionID]; err != nil {
		return tmdb.SourceCollection{}, err
	}
	return s.Collections[collectionID], nil
}

func (s *Source) CollectionCalls() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.collectionCalls...)
}

func (s *Source) ReferenceCalls() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.refCalls...)
}
