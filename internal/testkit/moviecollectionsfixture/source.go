// Package moviecollectionsfixture provides the shared deterministic source for
// movie-collection resolver tests. It records authoritative reference and
// roster lookups separately so tests can assert bounded, deduplicated access.
package moviecollectionsfixture

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/moviecollections"
)

type Source struct {
	mu               sync.Mutex
	Refs             map[int]moviecollections.CollectionRef
	Collections      map[int]moviecollections.SourceCollection
	RefErrors        map[int]error
	CollectionErrors map[int]error
	refCalls         []int
	collectionCalls  []int
}

func (s *Source) CollectionForMovie(_ context.Context, movieID int) (moviecollections.CollectionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refCalls = append(s.refCalls, movieID)
	if err := s.RefErrors[movieID]; err != nil {
		return moviecollections.CollectionRef{}, err
	}
	return s.Refs[movieID], nil
}

func (s *Source) MovieCollection(_ context.Context, collectionID int) (moviecollections.SourceCollection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collectionCalls = append(s.collectionCalls, collectionID)
	if err := s.CollectionErrors[collectionID]; err != nil {
		return moviecollections.SourceCollection{}, err
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
