package testkit

import (
	"context"
	"sync"
)

// SearchService is the shared in-memory double for a typed catalog search seam.
// Generic request and result types keep testkit independent of the package that
// owns the public search DTOs while still satisfying its Search method structurally.
type SearchService[Q, T any] struct {
	mu       sync.Mutex
	Results  []T
	Err      error
	requests []Q
}

func (s *SearchService[Q, T]) Search(_ context.Context, request Q) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	return append([]T(nil), s.Results...), s.Err
}

func (s *SearchService[Q, T]) Requests() []Q {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Q(nil), s.requests...)
}

// MovieCollectionService is the shared in-memory double for resolving the
// authoritative movie collections represented by a bounded set of Title Keys.
// Generic request/result types keep testkit independent of the API package.
type MovieCollectionService[Q, T any] struct {
	mu       sync.Mutex
	Result   T
	Err      error
	requests []Q
}

func (s *MovieCollectionService[Q, T]) ResolveMovieCollections(_ context.Context, request Q) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	return s.Result, s.Err
}

func (s *MovieCollectionService[Q, T]) Requests() []Q {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Q(nil), s.requests...)
}

// ApprovalAdditionResolution is one authoritative result returned by the
// shared ordered double for the approval gate's title-presence resolver.
type ApprovalAdditionResolution[T any] struct {
	Item  T
	Owned bool
	Err   error
}

type ApprovalAdditionResolver[T any] struct {
	mu      sync.Mutex
	Results []ApprovalAdditionResolution[T]
	calls   []T
}

func (r *ApprovalAdditionResolver[T]) ResolveApprovalAddition(_ context.Context, item T) (T, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, item)
	if len(r.Results) == 0 {
		var zero T
		return zero, false, nil
	}
	result := r.Results[0]
	r.Results = r.Results[1:]
	return result.Item, result.Owned, result.Err
}

func (r *ApprovalAdditionResolver[T]) Calls() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]T(nil), r.calls...)
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
