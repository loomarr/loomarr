package llm

import (
	"context"
	"sync/atomic"
)

// Selection is the full runtime choice of LLM backend (§8.1): which provider, at
// what base URL, which model, and (for hosted) which API key. A Swappable rebuilds
// its concrete provider from this on every change — local Ollama and any hosted
// OpenAI-compatible endpoint are both just a Selection.
type Selection struct {
	Provider string // "ollama" | "openai"
	URL      string // Ollama base, or the OpenAI-compatible base URL
	Model    string // model tag / id
	APIKey   string // hosted key; empty for local Ollama
}

// Swappable is a Provider whose underlying backend can change at runtime without
// rebuilding the suggester (§8.1 in-app selection). It holds the active Selection
// in an atomic pointer and rebuilds the concrete provider on each swap — so a Chat
// call always uses the currently-selected backend, and a Select from the API takes
// effect on the very next job with no restart.
//
// It covers BOTH a local Ollama host (swap the model tag) and a hosted provider
// (swap provider + base URL + model + key) — the two cases the model picker exposes.
type Swappable struct {
	// build returns a fresh concrete Provider for a Selection. Building is cheap (a
	// struct + shared http client), so we rebuild on each swap rather than mutating
	// a live provider's fields under a lock.
	build   func(Selection) Provider
	current atomic.Pointer[Provider]
	sel     atomic.Pointer[Selection]
}

// NewSwappable builds a Swappable around a provider factory, starting on the given
// initial Selection.
func NewSwappable(build func(Selection) Provider, initial Selection) *Swappable {
	s := &Swappable{build: build}
	s.Set(initial)
	return s
}

// Set atomically swaps the active provider + selection. Safe for concurrent
// callers; the last writer wins (selection is admin-only + rare, so no CAS loop).
func (s *Swappable) Set(sel Selection) {
	p := s.build(sel)
	s.current.Store(&p)
	s.sel.Store(&sel)
}

// SetModel swaps only the model, keeping the current provider/URL/key — the common
// local case (pick a different Ollama tag on the same host).
func (s *Swappable) SetModel(model string) {
	cur := s.Selection()
	cur.Model = model
	s.Set(cur)
}

// Selection returns a copy of the currently-active selection.
func (s *Swappable) Selection() Selection {
	if sel := s.sel.Load(); sel != nil {
		return *sel
	}
	return Selection{}
}

// Model returns the currently-active model tag/id.
func (s *Swappable) Model() string { return s.Selection().Model }

// Provider returns the currently-active provider kind ("ollama"|"openai").
func (s *Swappable) Provider() string { return s.Selection().Provider }

// Chat delegates to the currently-active provider.
func (s *Swappable) Chat(ctx context.Context, messages []Message, opts ChatOptions) (Response, error) {
	p := *s.current.Load()
	return p.Chat(ctx, messages, opts)
}

// Name delegates to the active provider ("ollama"|"openai"), and thus changes when
// the provider kind is swapped.
func (s *Swappable) Name() string {
	p := *s.current.Load()
	return p.Name()
}

var _ Provider = (*Swappable)(nil)
