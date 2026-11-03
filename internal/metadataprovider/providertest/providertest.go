// Package providertest is the conformance suite every metadata provider runs. It checks what the
// precedence rules rely on: a provider stays inside its declared capabilities, stamps every record
// with its own source, keeps an episode's ids apart from its series', and reports the contract's
// errors for what it cannot answer.
package providertest

import (
	"context"
	"errors"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// Case is what a provider's test supplies: items its fixture holds, and items it does not.
type Case struct {
	Held    []mp.Ref
	Missing []mp.Ref
}

// Run checks p against the contract.
func Run(t *testing.T, p mp.Provider, c Case) {
	t.Helper()
	if len(c.Held) == 0 {
		t.Fatal("providertest: a case with no held item checks no record")
	}
	caps := p.Capabilities()

	t.Run("registers", func(t *testing.T) {
		if _, err := mp.NewRegistry(p); err != nil {
			t.Fatalf("the registry refuses the provider: %v", err)
		}
		for _, r := range caps.Resolves {
			if !slices.Contains(caps.Kinds, r.Kind) {
				t.Errorf("resolves %s by %s but does not describe %s", r.Kind, r.Namespace, r.Kind)
			}
		}
	})

	t.Run("refuses what it cannot resolve", func(t *testing.T) {
		ref := mp.Ref{Kind: "conformance", ID: mp.ExternalID{Namespace: "conformance", Value: "1"}}
		if _, err := p.Fetch(context.Background(), ref); !errors.Is(err, mp.ErrUnsupported) {
			t.Fatalf("Fetch(%+v) = %v, want ErrUnsupported", ref, err)
		}
	})

	t.Run("stops on a cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := p.Fetch(ctx, c.Held[0]); err == nil {
			t.Fatal("Fetch succeeded on a cancelled context")
		}
	})

	t.Run("held items stay inside the capabilities", func(t *testing.T) {
		for _, ref := range c.Held {
			if !caps.CanResolve(ref) {
				t.Errorf("held %+v is outside the declared Resolves", ref)
				continue
			}
			rec, err := p.Fetch(context.Background(), ref)
			if err != nil {
				t.Errorf("Fetch(%+v): %v", ref, err)
				continue
			}
			if rec.Kind != ref.Kind {
				t.Errorf("Fetch(%+v) returned a %s", ref, rec.Kind)
			}
			checkRecord(t, p, rec)
		}
	})

	t.Run("missing items are not found", func(t *testing.T) {
		for _, ref := range c.Missing {
			if _, err := p.Fetch(context.Background(), ref); !errors.Is(err, mp.ErrNotFound) {
				t.Errorf("Fetch(%+v) = %v, want ErrNotFound", ref, err)
			}
		}
	})

	if lister, ok := p.(mp.Lister); ok {
		t.Run("listed items stay inside the capabilities", func(t *testing.T) {
			records, err := lister.List(context.Background())
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(records) == 0 {
				t.Fatal("List returned nothing from a fixture that holds items")
			}
			for _, rec := range records {
				checkRecord(t, p, rec)
			}
		})
	}
}

// checkRecord checks one record and its children.
func checkRecord(t *testing.T, p mp.Provider, rec mp.Record) {
	t.Helper()
	caps := p.Capabilities()
	want := mp.Source{Provider: p.ID(), Basis: caps.Basis, Catalog: rec.Source.Catalog}
	if caps.Catalog != "" {
		want.Catalog = caps.Catalog
	}
	if rec.Source != want {
		t.Errorf("record source = %+v, want %+v", rec.Source, want)
	}
	if !slices.Contains(caps.Kinds, rec.Kind) {
		t.Errorf("record kind %s is not declared", rec.Kind)
	}
	for _, f := range rec.Families() {
		if !caps.Supplies(f) {
			t.Errorf("%s record supplies undeclared family %s", rec.Kind, f)
		}
	}
	for _, id := range append(append([]mp.ExternalID(nil), rec.IDs...), rec.Parent...) {
		if id.Namespace == "" || id.Value == "" {
			t.Errorf("%s record carries an incomplete id %+v", rec.Kind, id)
		}
	}
	for _, own := range rec.IDs {
		if slices.Contains(rec.Parent, own) {
			t.Errorf("%s record gives its parent's id %+v as its own", rec.Kind, own)
		}
	}
	if rec.Numbering != nil && rec.Kind != mp.KindEpisode {
		t.Errorf("a %s record carries episode numbering", rec.Kind)
	}
	for _, more := range rec.More {
		if more.Kind != rec.Kind || len(more.More) > 0 {
			t.Errorf("a %s record has a further %s with %d more of its own", rec.Kind, more.Kind, len(more.More))
		}
		checkRecord(t, p, more)
	}
	for _, child := range rec.Children {
		if child.Kind != mp.KindEpisode || rec.Kind != mp.KindSeries {
			t.Errorf("a %s record has a %s child", rec.Kind, child.Kind)
		}
		checkRecord(t, p, child)
	}
}
