package metadataprovider

import "fmt"

// Registry holds the providers an installation runs. It is built once at startup and read
// afterwards, so it needs no lock.
type Registry struct {
	ordered []Provider
	byID    map[ID]Provider
}

// NewRegistry registers providers in the order given. A provider whose ID is not one of IDs, is
// repeated, or whose capabilities are incomplete is a programming error, reported here rather
// than discovered later as a provider that is silently never asked.
func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{byID: make(map[ID]Provider, len(providers))}
	for _, p := range providers {
		id := p.ID()
		if _, err := ParseID(string(id)); err != nil {
			return nil, err
		}
		if _, dup := r.byID[id]; dup {
			return nil, fmt.Errorf("metadataprovider: provider %q is registered twice", id)
		}
		if err := checkCapabilities(p.Capabilities()); err != nil {
			return nil, fmt.Errorf("metadataprovider: provider %q: %w", id, err)
		}
		r.byID[id] = p
		r.ordered = append(r.ordered, p)
	}
	return r, nil
}

func checkCapabilities(c Capabilities) error {
	switch {
	case c.Basis == "":
		return fmt.Errorf("declares no basis")
	case len(c.Kinds) == 0:
		return fmt.Errorf("describes no kind")
	case len(c.Families) == 0:
		return fmt.Errorf("supplies no field family")
	case len(c.Resolves) == 0:
		return fmt.Errorf("resolves no namespace")
	}
	return nil
}

// Get returns the provider registered under id.
func (r *Registry) Get(id ID) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byID[id]
	return p, ok
}

// Providers returns every registered provider in registration order.
func (r *Registry) Providers() []Provider {
	if r == nil {
		return nil
	}
	return append([]Provider(nil), r.ordered...)
}

// Resolving returns the providers that can fetch ref, in registration order.
func (r *Registry) Resolving(ref Ref) []Provider {
	var out []Provider
	for _, p := range r.Providers() {
		if p.Capabilities().CanResolve(ref) {
			out = append(out, p)
		}
	}
	return out
}

// Supplying returns the providers that supply family, in registration order.
func (r *Registry) Supplying(family Family) []Provider {
	var out []Provider
	for _, p := range r.Providers() {
		if p.Capabilities().Supplies(family) {
			out = append(out, p)
		}
	}
	return out
}
