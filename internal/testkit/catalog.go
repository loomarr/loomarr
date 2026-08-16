package testkit

import (
	"context"
	"sync"
)

// SearchRequest records one call through a SearchService test double.
type SearchRequest struct {
	Query string
	Scope string
	Limit int
}

// SearchService is the shared in-memory double for a typed catalog search seam.
// The generic result keeps testkit independent of the package that owns the
// candidate DTO while still satisfying its Search method structurally.
type SearchService[T any] struct {
	mu       sync.Mutex
	Results  []T
	Err      error
	requests []SearchRequest
}

func (s *SearchService[T]) Search(_ context.Context, query, scope string, limit int) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, SearchRequest{Query: query, Scope: scope, Limit: limit})
	return append([]T(nil), s.Results...), s.Err
}

func (s *SearchService[T]) Requests() []SearchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SearchRequest(nil), s.requests...)
}

// IconService is the shared in-memory double for a typed channel-icon suggestion
// seam. Like SearchService, its generic result avoids coupling testkit to the API
// package's presentation type.
type IconService[T any] struct {
	mu         sync.Mutex
	Results    []T
	Err        error
	channelIDs []string
}

func (s *IconService[T]) IconSuggestions(_ context.Context, channelID string) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelIDs = append(s.channelIDs, channelID)
	return append([]T(nil), s.Results...), s.Err
}

func (s *IconService[T]) ChannelIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.channelIDs...)
}
