package taxonomy_test

import (
	"reflect"
	"testing"

	"github.com/mantonx/loomarr/internal/taxonomy"
)

func seed() *taxonomy.Forest { return taxonomy.New(taxonomy.SeedForest()) }

// ⚠ Seed integrity: every parent must exist, and no taxon may cycle. A malformed seed would seed a
// broken graph into every install's migration — this is the guard that keeps the shipped default sane.
func TestSeedForest_Integrity(t *testing.T) {
	f := seed()
	for _, tx := range taxonomy.SeedForest() {
		if tx.Slug == "" {
			t.Errorf("taxon with empty slug: %+v", tx)
		}
		if tx.Parent != "" {
			if _, ok := f.Get(tx.Parent); !ok {
				t.Errorf("taxon %q has parent %q which is not in the forest", tx.Slug, tx.Parent)
			}
		}
		// Ancestors terminates (its own cycle guard); assert it does not include the taxon itself.
		for _, a := range f.Ancestors(tx.Slug) {
			if a == tx.Slug {
				t.Errorf("taxon %q is its own ancestor — a cycle", tx.Slug)
			}
		}
	}
}

// Resolve grounds a raw model tag to a canonical slug, or drops it — the anti-fabrication gate.
func TestResolve(t *testing.T) {
	f := seed()
	cases := []struct {
		raw      string
		wantSlug string
		wantOK   bool
	}{
		{"beer", "beer", true},                // exact slug
		{"BEER", "beer", true},                // case-insensitive
		{"  beer  ", "beer", true},            // trimmed
		{"brew", "beer", true},                // synonym rescue — the LLM-friendly path
		{"lager", "beer", true},               // another synonym
		{"beverage", "drinks", true},          // synonym on a root
		{"soft-drink", "soda", true},          // synonym on a leaf
		{"nonsense-widget", "", false},        // off-vocabulary → DROPPED, never a new taxon
		{"", "", false},                       // empty → dropped
	}
	for _, c := range cases {
		gotSlug, gotOK := f.Resolve(c.raw)
		if gotSlug != c.wantSlug || gotOK != c.wantOK {
			t.Errorf("Resolve(%q) = (%q,%v), want (%q,%v)", c.raw, gotSlug, gotOK, c.wantSlug, c.wantOK)
		}
	}
}

// ⚠ THE grounding property, stated as a test: an off-vocabulary tag must be DROPPED, so a model can
// never mint a taxon. Only an operator adds taxa. Goes red if Resolve ever falls back to accepting raw.
func TestResolve_DropsUnknownNeverMintsTaxon(t *testing.T) {
	f := seed()
	if slug, ok := f.Resolve("cryptocurrency"); ok {
		t.Errorf("Resolve invented a taxon %q for an off-vocabulary tag — the model must not mint taxa", slug)
	}
}

// Ancestors / WithRollups derive the rollup tags a leaf implies — the denormalised set.
func TestRollups(t *testing.T) {
	f := seed()
	if got := f.Ancestors("beer"); !reflect.DeepEqual(got, []string{"alcohol", "drinks"}) {
		t.Errorf("Ancestors(beer) = %v, want [alcohol drinks]", got)
	}
	if got := f.Ancestors("cereal"); !reflect.DeepEqual(got, []string{"food"}) {
		t.Errorf("Ancestors(cereal) = %v, want [food]", got)
	}
	// A root has no ancestors.
	if got := f.Ancestors("drinks"); len(got) != 0 {
		t.Errorf("Ancestors(drinks) = %v, want none — it is a root", got)
	}
	// WithRollups flags leaf vs rollup: beer(leaf) + alcohol(rollup) + drinks(rollup).
	rows := f.WithRollups("beer")
	want := []taxonomy.TagRow{{Slug: "beer", Leaf: true}, {Slug: "alcohol", Leaf: false}, {Slug: "drinks", Leaf: false}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("WithRollups(beer) = %+v, want %+v", rows, want)
	}
}

// ⚠ Cycle guard: an operator-edited graph could contain a loop (a → b → a). Ancestors must terminate,
// not spin forever. Build a tiny malformed forest and assert it stops.
func TestAncestors_CycleGuard(t *testing.T) {
	f := taxonomy.New([]taxonomy.Taxon{
		{Slug: "a", Parent: "b", Axis: taxonomy.AxisProduct},
		{Slug: "b", Parent: "a", Axis: taxonomy.AxisProduct},
	})
	got := f.Ancestors("a") // must terminate; the exact contents matter less than "does not hang"
	if len(got) > 2 {
		t.Errorf("Ancestors on a cycle returned %v — the guard failed to stop", got)
	}
}

// RetiredAliases resolve so a rename does not drop clips tagged under the old slug.
func TestResolve_RetiredAlias(t *testing.T) {
	f := taxonomy.New([]taxonomy.Taxon{
		{Slug: "drinks", Label: "Drinks", Axis: taxonomy.AxisProduct, RetiredAliases: []string{"beverages-old"}},
	})
	if slug, ok := f.Resolve("beverages-old"); !ok || slug != "drinks" {
		t.Errorf("Resolve(retired alias) = (%q,%v), want (drinks,true)", slug, ok)
	}
}
