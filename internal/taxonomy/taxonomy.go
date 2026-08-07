// Package taxonomy is the clip tag vocabulary (§10 V45a): a forest of taxa on independent AXES
// (product / format / seasonal / audience-cue), the graph that turns a leaf tag like `beer` into its
// rollups (`alcohol`, `drinks`), and the resolve-or-drop grounding that keeps a model's output on the
// vocabulary.
//
// It replaces the flat 12-value `category` string, which could not answer "is `cereal` a kind of
// food?" — the question a curation rule like "one food ad per break" needs. A clip carries a SET of
// tags, not one category; each tag is a taxon here.
//
// ⚠ **This is the SEED forest — the default an install starts with, not a closed set.** The taxonomy
// is operator-editable (stored in `taxa`, §5); this package defines the shipped default and the pure
// graph operations over ANY taxonomy (seed or operator-edited), so the store and the tagger share one
// implementation of "what is `beer`'s ancestor chain" rather than each re-deriving it.
package taxonomy

import (
	"sort"
	"strings"
)

// Axis is the independent dimension a taxon lives on (§10 V45a). A clip is tagged on several at once
// — a Christmas beer ad is a `product` (beer), on the `seasonal` axis (christmas). Keeping them
// separate is what stops `psa` (a format) and `beer` (a product) becoming false siblings in one tree.
type Axis string

const (
	// AxisProduct is what the clip SELLS — beer, cereal, cars. The deepest hierarchy.
	AxisProduct Axis = "product"
	// AxisFormat is what the clip IS, not what it sells — a psa, a movie trailer, a station ident.
	AxisFormat Axis = "format"
	// AxisSeasonal is when it belongs — christmas, back-to-school. Reuses the §10 holiday IDs.
	AxisSeasonal Axis = "seasonal"
	// AxisAudienceCue is a HINT about who it suits, kept separate from the clip's `audience` verdict
	// (a cue is a suggestion the tagger may read, not the authoritative audience enum).
	AxisAudienceCue Axis = "audience-cue"
)

// Taxon is one node in the taxonomy forest.
type Taxon struct {
	// Slug is the stable machine id — lowercase, hyphenless-or-hyphenated, the token the LLM emits and
	// every tag row references. Renaming a slug is a breaking change; use RetiredAliases instead.
	Slug string
	// Label is the human display form.
	Label string
	// Parent is the slug of this taxon's parent on the SAME axis, or "" for an axis root. `beer`'s
	// parent is `alcohol`, whose parent is `drinks`, an AxisProduct root.
	Parent string
	// Axis is which dimension this taxon lives on. A taxon's ancestor chain never crosses axes.
	Axis Axis
	// Synonyms are near-miss forms the grounding maps to this Slug — the LLM-friendliness lever. A
	// model that emits `brew` or `lager` resolves to `beer` rather than being dropped.
	Synonyms []string
	// RetiredAliases are former slugs that still resolve here after a rename, so an operator renaming a
	// taxon does not silently drop every clip tagged under the old name (§10 retired-identifier rule).
	RetiredAliases []string
}

// Forest is a taxonomy — a set of taxa keyed by slug, with the derived indexes the graph operations
// need. Build it with New so the synonym/alias resolution index and the child map are consistent.
type Forest struct {
	bySlug   map[string]Taxon
	resolver map[string]string   // synonym|alias|slug (all lowercased) → canonical slug
	children map[string][]string // slug → child slugs (for descendants queries)
}

// New builds a Forest from a taxa list, computing the resolve index (slug + every synonym + every
// retired alias → canonical) and the child map. Later duplicate slugs win, which lets an operator
// override a seeded taxon by re-declaring it.
func New(taxa []Taxon) *Forest {
	f := &Forest{
		bySlug:   make(map[string]Taxon, len(taxa)),
		resolver: make(map[string]string),
		children: make(map[string][]string),
	}
	for _, t := range taxa {
		f.bySlug[t.Slug] = t
	}
	for _, t := range f.bySlug {
		f.resolver[strings.ToLower(t.Slug)] = t.Slug
		for _, s := range t.Synonyms {
			f.resolver[strings.ToLower(s)] = t.Slug
		}
		for _, a := range t.RetiredAliases {
			f.resolver[strings.ToLower(a)] = t.Slug
		}
		if t.Parent != "" {
			f.children[t.Parent] = append(f.children[t.Parent], t.Slug)
		}
	}
	return f
}

// Resolve grounds a raw tag from the model (or an operator import) to a canonical slug (§10 V45a).
// It returns the slug and true when the raw value is a known slug, synonym, or retired alias
// (case-insensitive, trimmed); it returns "" and false for anything off the vocabulary — which the
// caller DROPS. This is the anti-fabrication gate: an unknown tag never becomes a new taxon (only an
// operator adds taxa), exactly as an ungrounded era or brand is dropped rather than persisted (§8).
func (f *Forest) Resolve(raw string) (string, bool) {
	slug, ok := f.resolver[strings.ToLower(strings.TrimSpace(raw))]
	return slug, ok
}

// Ancestors returns slug's parent chain, nearest first, EXCLUDING slug itself — the rollup tags a
// leaf tag implies. `beer` → [alcohol, drinks]. An unknown or root slug returns nil.
//
// Two malformations of an operator-edited graph are guarded, and both matter for the rollup cache:
//   - A CYCLE (a malformed remap A→B→A) must not loop forever: it stops if it revisits a slug.
//   - A DANGLING PARENT (a child whose `parent` names a taxon NOT in the graph — the state
//     DeleteTaxon leaves an un-reparented child in, or a typo'd UpsertTaxon parent) must not emit
//     that dead slug as a phantom ancestor. The chain STOPS at the first parent that does not exist,
//     so a dangling reference contributes no rollup rather than a rollup to a node curation can never
//     match. Only REAL nodes are followed.
func (f *Forest) Ancestors(slug string) []string {
	var out []string
	seen := map[string]bool{slug: true}
	cur := f.bySlug[slug].Parent
	for cur != "" && !seen[cur] {
		parent, ok := f.bySlug[cur]
		if !ok {
			break // dangling parent — a slug not in the graph is not a real ancestor
		}
		out = append(out, cur)
		seen[cur] = true
		cur = parent.Parent
	}
	return out
}

// WithRollups returns the full tag set a leaf tag implies: the leaf plus all its ancestors, each
// flagged whether it is the LEAF (the model/operator asserted it) or a ROLLUP (derived). This is what
// the denormalised writer stores — the leaf flag is what lets a re-tag replace leaves while rollups
// are always recomputed from the current graph (§10 V45a).
func (f *Forest) WithRollups(leaf string) []TagRow {
	rows := []TagRow{{Slug: leaf, Leaf: true}}
	for _, a := range f.Ancestors(leaf) {
		rows = append(rows, TagRow{Slug: a, Leaf: false})
	}
	return rows
}

// TagRow is one denormalised clip↔taxon row: the slug and whether it was asserted (leaf) or derived
// (rollup). Stored in `clip_tags`; the leaf flag drives the reindex (§10 V45a).
type TagRow struct {
	Slug string
	Leaf bool
}

// Get returns a taxon by slug, and whether it exists.
func (f *Forest) Get(slug string) (Taxon, bool) {
	t, ok := f.bySlug[slug]
	return t, ok
}

// All returns every taxon, sorted by axis then slug — the stable order the seed migration and the
// served vocabulary use (a deterministic order keeps the migration and the API diff-stable).
func (f *Forest) All() []Taxon {
	out := make([]Taxon, 0, len(f.bySlug))
	for _, t := range f.bySlug {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Axis != out[j].Axis {
			return out[i].Axis < out[j].Axis
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// PrimaryProductLeaf picks the single product-axis slug to use as a clip's derived `category` shadow
// (§10 V45a) — the MOST SPECIFIC product tag among the given leaves. `category` is no longer an input;
// it is computed from the taxonomy tags so existing category readers keep working during the
// transition, with the taxonomy graph as the one source of truth.
//
// "Most specific" = the product-axis leaf with the longest ancestor chain (deepest in the tree); ties
// break on slug order for determinism. Returns "" when no leaf is on the product axis (a clip tagged
// only `psa` (format) or `christmas` (seasonal) has no product category — which is correct, and is a
// state the flat string could not represent honestly).
func (f *Forest) PrimaryProductLeaf(leaves []string) string {
	best, bestDepth := "", -1
	for _, slug := range leaves {
		t, ok := f.bySlug[slug]
		if !ok || t.Axis != AxisProduct {
			continue
		}
		depth := len(f.Ancestors(slug))
		if depth > bestDepth || (depth == bestDepth && slug < best) {
			best, bestDepth = slug, depth
		}
	}
	return best
}

// Vocab renders the taxonomy as a compact per-axis prompt fragment the tagger serves to the model
// (§10 V45a) — the same "BE is the single source of the vocabulary" discipline schedule.BuildVocabulary
// gives the suggester, so the model never guesses a slug blind and every slug it CAN emit is one the
// grounding will accept. One line per axis: `product: beer, cereal, cars, …` using slugs (the tokens
// Resolve grounds against), so the served list and the accepted list are the same list by construction.
func (f *Forest) Vocab() string {
	byAxis := map[Axis][]string{}
	var order []Axis
	for _, t := range f.All() { // All() is axis-then-slug sorted, so each axis's slugs come out stable
		if _, seen := byAxis[t.Axis]; !seen {
			order = append(order, t.Axis)
		}
		byAxis[t.Axis] = append(byAxis[t.Axis], t.Slug)
	}
	var b strings.Builder
	for _, ax := range order {
		b.WriteString(string(ax))
		b.WriteString(": ")
		b.WriteString(strings.Join(byAxis[ax], ", "))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
