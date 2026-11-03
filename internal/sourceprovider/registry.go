package sourceprovider

import "fmt"

// Registry holds the providers an installation runs, by ID. It is built once at startup and read
// afterwards, so it needs no lock.
//
// A source row names its provider by ID. A row whose provider is not registered is inert: filler
// neither fetches nor deletes it, so the provider coming back brings its sources with it.
type Registry struct {
	byID map[ID]Provider
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
	}
	return r, nil
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

// Scanned is a provider whose sources are read where they are, such as a folder or the media
// library, and never fetched. It has no Lister, so filler never checks its sources.
func Scanned(id ID, label, addressHint string) Provider {
	return scanned{id: id, description: Description{Label: label, AddressHint: addressHint}}
}

type scanned struct {
	id          ID
	description Description
}

func (s scanned) ID() ID                { return s.id }
func (s scanned) Describe() Description { return s.description }
