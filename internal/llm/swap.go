package llm

import (
	"context"
	"sync/atomic"
)

// Swappable is a Provider whose underlying MODEL can change at runtime without
// rebuilding the suggester (§8.1 in-app model selection). It wraps a factory that
// builds a concrete provider for a given model tag, and holds the active tag in an
// atomic pointer — so a Chat call always uses the currently-selected model, and a
// Select from the API takes effect on the very next job with no restart.
//
// This is the ONE runtime-mutable provider seam. It's used for local Ollama, where
// swapping models is just naming a different tag on the same host; a hosted openai
// provider names its model in config and isn't wrapped (nothing local to swap).
type Swappable struct {
	// build returns a fresh concrete Provider for a model tag. Building is cheap
	// (a struct + shared http client), so we rebuild on each swap rather than
	// mutating a live provider's fields under a lock.
	build   func(model string) Provider
	current atomic.Pointer[Provider]
	model   atomic.Pointer[string]
}

// NewSwappable builds a Swappable around a provider factory, starting on
// initialModel. build is typically a closure over the provider's base URL + key.
func NewSwappable(build func(model string) Provider, initialModel string) *Swappable {
	s := &Swappable{build: build}
	s.set(initialModel)
	return s
}

// set atomically swaps the active provider + model. Safe for concurrent callers;
// the last writer wins (model selection is admin-only + rare, so no CAS loop needed).
func (s *Swappable) set(model string) {
	p := s.build(model)
	s.current.Store(&p)
	s.model.Store(&model)
}

// SetModel changes the active model at runtime. Returns the model now in effect.
func (s *Swappable) SetModel(model string) string {
	s.set(model)
	return model
}

// Model returns the currently-active model tag.
func (s *Swappable) Model() string {
	if m := s.model.Load(); m != nil {
		return *m
	}
	return ""
}

// Chat delegates to the currently-active provider.
func (s *Swappable) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	p := *s.current.Load()
	return p.Chat(ctx, messages, opts)
}

// Name delegates to the active provider (stable across swaps for a given provider
// kind — "ollama").
func (s *Swappable) Name() string {
	p := *s.current.Load()
	return p.Name()
}

var _ Provider = (*Swappable)(nil)
