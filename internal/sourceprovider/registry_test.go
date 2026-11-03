package sourceprovider_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/sourceprovider"
)

type fetchedProvider struct{ id sourceprovider.ID }

func (p fetchedProvider) ID() sourceprovider.ID { return p.id }
func (fetchedProvider) Describe() sourceprovider.Description {
	return sourceprovider.Description{Label: "Fetched", Fetched: true}
}
func (fetchedProvider) List(context.Context, sourceprovider.Source, sourceprovider.ListRequest) (sourceprovider.Listing, error) {
	return sourceprovider.Listing{}, nil
}

func TestRegistry_FindsAProviderByIDAndOnlyListsAFetchedOne(t *testing.T) {
	r, err := sourceprovider.NewRegistry(fetchedProvider{id: "fetched"}, sourceprovider.Scanned("folder", "Folder", "/media"))
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := r.Get("folder"); !ok || p.Describe() != (sourceprovider.Description{Label: "Folder", AddressHint: "/media"}) {
		t.Fatalf("folder = %+v, %v, want the scanned provider as registered", p, ok)
	}
	if _, ok := r.Lister("fetched"); !ok {
		t.Fatal("a fetched provider has no Lister")
	}
	if _, ok := r.Lister("folder"); ok {
		t.Fatal("a scanned provider has a Lister: it would be fetched")
	}
}

func TestRegistry_AnUnregisteredProviderIsAbsentNotAnError(t *testing.T) {
	r, err := sourceprovider.NewRegistry(fetchedProvider{id: "fetched"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("vimeo"); ok {
		t.Fatal("an unregistered provider was found")
	}
	if _, ok := r.Lister("vimeo"); ok {
		t.Fatal("an unregistered provider has a Lister")
	}
	var nilRegistry *sourceprovider.Registry
	if _, ok := nilRegistry.Lister("fetched"); ok {
		t.Fatal("a nil registry has a Lister")
	}
}

func TestNewRegistry_RefusesAnEmptyOrRepeatedID(t *testing.T) {
	if _, err := sourceprovider.NewRegistry(fetchedProvider{id: ""}); err == nil {
		t.Error("registered a provider with no ID")
	}
	if _, err := sourceprovider.NewRegistry(fetchedProvider{id: "a"}, sourceprovider.Scanned("a", "A", "")); err == nil {
		t.Error("registered one ID twice: the second would shadow the first")
	}
}
