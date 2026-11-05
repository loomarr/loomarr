package sourceprovider

import "fmt"

// Registry holds the providers an installation runs, by ID. It is built once at startup and read
// afterwards, so it needs no lock.
//
// A source row names its provider by ID. A row whose provider is not registered is inert: filler
// neither fetches nor deletes it, so the provider coming back brings its sources with it.
type Registry struct {
	byID  map[ID]Provider
	order []Provider
}

// NewRegistry registers providers. A provider with an empty or repeated ID is a programming error,
// reported here rather than discovered as one provider quietly shadowing another.
func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{byID: make(map[ID]Provider, len(providers))}
	for _, p := range providers {
		id := p.ID()
		if id == "" {
			return nil, fmt.Errorf("sourceprovider: a provider has no ID")
		}
		if _, dup := r.byID[id]; dup {
			return nil, fmt.Errorf("sourceprovider: provider %q is registered twice", id)
		}
		r.byID[id] = p
		r.order = append(r.order, p)
	}
	return r, nil
}

// All returns the providers in the order they were registered, which is the order a list shows
// them in.
func (r *Registry) All() []Provider {
	if r == nil {
		return nil
	}
	return append([]Provider(nil), r.order...)
}

// Get returns the provider registered under id.
func (r *Registry) Get(id ID) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byID[id]
	return p, ok
}

// Lister returns the registered provider's Lister: false when the provider is not registered or is
// scanned where it is rather than fetched.
func (r *Registry) Lister(id ID) (Lister, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	l, ok := p.(Lister)
	return l, ok
}

// Finder returns the registered provider's Finder: false when the provider is not registered or
// cannot suggest or verify a source.
func (r *Registry) Finder(id ID) (Finder, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	f, ok := p.(Finder)
	return f, ok
}

// Searcher returns the registered provider's Searcher: false when the provider is not registered
// or cannot search inside a source.
func (r *Registry) Searcher(id ID) (Searcher, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	s, ok := p.(Searcher)
	return s, ok
}

// Addresser returns the registered provider's Addresser: false when the provider is not
// registered or does not take sources by hand.
func (r *Registry) Addresser(id ID) (Addresser, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	a, ok := p.(Addresser)
	return a, ok
}

// Linker returns the registered provider's Linker: false when the provider is not registered or
// has no pages to link to.
func (r *Registry) Linker(id ID) (Linker, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	l, ok := p.(Linker)
	return l, ok
}

// Recognizer returns the registered provider's Recognizer: false when the provider is not
// registered or cannot tell an address from a search.
func (r *Registry) Recognizer(id ID) (Recognizer, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	rec, ok := p.(Recognizer)
	return rec, ok
}

// Fetched reports whether id names a registered provider whose sources are downloaded rather than
// scanned where they are.
func (r *Registry) Fetched(id ID) bool {
	p, ok := r.Get(id)
	return ok && p.Describe().Fetched
}
