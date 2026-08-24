package taxonomy_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/taxonomy"
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
		{"beer", "beer", true},         // exact slug
		{"BEER", "beer", true},         // case-insensitive
		{"  beer  ", "beer", true},     // trimmed
		{"brew", "beer", true},         // synonym rescue — the LLM-friendly path
		{"lager", "beer", true},        // another synonym
		{"beverage", "drinks", true},   // synonym on a root
		{"soft-drink", "soda", true},   // synonym on a leaf
		{"nonsense-widget", "", false}, // off-vocabulary → DROPPED, never a new taxon
		{"", "", false},                // empty → dropped
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

// ⚠ Dangling-parent guard (§10 V45a): a child whose `parent` names a taxon NOT in the graph — the
// state a deleted-but-not-reparented node leaves, or a typo'd Upsert parent. Ancestors must STOP at
// the missing parent, never emit it as a phantom rollup. This is the bug the reindex conformance
// surfaced: a phantom ancestor is a rollup to a node curation can never match. DeleteTaxon avoids
// creating this state (it reparents), but this is the source-level safety net for every other path.
func TestAncestors_DanglingParent(t *testing.T) {
	f := taxonomy.New([]taxonomy.Taxon{
		{Slug: "beer", Parent: "", Axis: taxonomy.AxisProduct},
		{Slug: "lager", Parent: "ghost", Axis: taxonomy.AxisProduct}, // parent 'ghost' is not in the graph
	})
	if got := f.Ancestors("lager"); len(got) != 0 {
		t.Errorf("Ancestors(lager) = %v, want none — a dangling parent is not a real ancestor", got)
	}
	// A real chain with a dangling END still yields the real prefix, then stops.
	f2 := taxonomy.New([]taxonomy.Taxon{
		{Slug: "lager", Parent: "beer", Axis: taxonomy.AxisProduct},
		{Slug: "beer", Parent: "ghost", Axis: taxonomy.AxisProduct}, // beer exists; its parent does not
	})
	if got := f2.Ancestors("lager"); !reflect.DeepEqual(got, []string{"beer"}) {
		t.Errorf("Ancestors(lager) = %v, want [beer] — real prefix kept, dangling tail dropped", got)
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

func TestDescendants_IsStableParentFirstForDeepTrees(t *testing.T) {
	forest := taxonomy.New([]taxonomy.Taxon{
		{Slug: "root", Label: "Root", Axis: taxonomy.AxisProduct},
		{Slug: "z-child", Label: "Z", Parent: "root", Axis: taxonomy.AxisProduct},
		{Slug: "a-child", Label: "A", Parent: "root", Axis: taxonomy.AxisProduct},
		{Slug: "grandchild", Label: "Grandchild", Parent: "a-child", Axis: taxonomy.AxisProduct},
	})
	got := forest.Descendants("root")
	want := []string{"a-child", "grandchild", "z-child"}
	if len(got) != len(want) {
		t.Fatalf("Descendants(root) = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].Slug != want[i] {
			t.Fatalf("Descendants(root)[%d] = %q, want %q", i, got[i].Slug, want[i])
		}
	}
	if got := forest.Descendants("missing"); len(got) != 0 {
		t.Fatalf("Descendants(missing) = %+v, want empty", got)
	}
}

func TestValidate_AcceptsSeedForest(t *testing.T) {
	if err := taxonomy.Validate(taxonomy.SeedForest()); err != nil {
		t.Fatalf("Validate(seed) = %v, want a valid shipped forest", err)
	}
}

func TestValidate_RejectsMalformedGraphs(t *testing.T) {
	base := []taxonomy.Taxon{
		{Slug: "food", Label: "Food", Axis: taxonomy.AxisProduct},
		{Slug: "cereal", Label: "Cereal", Parent: "food", Axis: taxonomy.AxisProduct},
		{Slug: "promo", Label: "Promo", Axis: taxonomy.AxisFormat},
	}
	tests := []struct {
		name string
		edit func([]taxonomy.Taxon) []taxonomy.Taxon
	}{
		{"malformed slug", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Slug = "Breakfast Food"; return ts }},
		{"blank label", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Label = "  "; return ts }},
		{"unknown axis", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Axis = "topic"; return ts }},
		{"missing parent", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Parent = "missing"; return ts }},
		{"cross-axis parent", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Parent = "promo"; return ts }},
		{"self parent", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Parent = "cereal"; return ts }},
		{"cycle", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[0].Parent = "cereal"; return ts }},
		{"resolver collision", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Synonyms = []string{"promo"}; return ts }},
		{"duplicate resolver spelling", func(ts []taxonomy.Taxon) []taxonomy.Taxon { ts[1].Synonyms = []string{"flakes", " FLAKES "}; return ts }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := append([]taxonomy.Taxon(nil), base...)
			if err := taxonomy.Validate(tt.edit(got)); !errors.Is(err, taxonomy.ErrInvalidForest) {
				t.Fatalf("Validate() = %v, want ErrInvalidForest", err)
			}
		})
	}
}

func TestCanonicalize_MatchesResolverWhitespaceRules(t *testing.T) {
	taxon := taxonomy.Canonicalize(taxonomy.Taxon{
		Slug: " cereal ", Label: " Breakfast cereal ", Parent: " food ",
		Axis: taxonomy.AxisProduct, Synonyms: []string{" cold cereal "}, RetiredAliases: []string{"breakfast-food "},
	})
	if taxon.Slug != "cereal" || taxon.Label != "Breakfast cereal" || taxon.Parent != "food" ||
		taxon.Synonyms[0] != "cold cereal" || taxon.RetiredAliases[0] != "breakfast-food" {
		t.Fatalf("Canonicalize = %+v", taxon)
	}
	forest := taxonomy.New([]taxonomy.Taxon{taxon})
	if got, ok := forest.Resolve("  COLD CEREAL "); !ok || got != "cereal" {
		t.Fatalf("Resolve spaced synonym = %q, %v, want cereal, true", got, ok)
	}
}
