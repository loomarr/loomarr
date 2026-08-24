package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// AI-assisted tagging (§10): classify an ingested clip's TEXT SIGNALS —
// filename, the source title/description yt-dlp/Archive preserve, and (for clips
// produced by compilation splitting, V34) the segment transcript — into
// era/audience/a SET OF TAXONOMY TAGS, via the configured LLM. Grounding here is
// resolve-or-drop against the taxonomy graph PLUS the era rule: the model
// classifies a clip we hand it (it can't invent a clip), each tag it returns is
// grounded through taxonomy.Forest.Resolve (an exact slug or a synonym → kept and
// canonicalised; anything else → DROPPED, never a new taxon), and its era is
// accepted ONLY when the year appears literally in the text signals — a measured
// §8 hole (2 of 10 real transcripts got an era inferred from tone, plan §6.4).
// An ungrounded era is NOT persisted as fact: it comes back as SuggestedEra for
// the operator to confirm.
//
// ⚠ V45a: the flat closed `category` enum is GONE. It was one of THREE drifting
// copies of the same list (this file's map, the inline prompt, and
// schedule.fillerCategories mirrored to the FE); the taxonomy graph is now the one
// vocabulary, SERVED to the model (forest.Vocab()) so the served set and the
// grounded set are the same set by construction. `Category` survives only as a
// DERIVED shadow — the primary product leaf of the tag set — for readers not yet
// on the tag set (pod separation, the catalog filter).

// TagSuggestion is the validated classification for one clip. Fields are zero
// when the model gave an unusable/ungrounded value (that field stays untagged).
type TagSuggestion struct {
	Era      int
	Audience Audience
	// Tags is the GROUNDED taxonomy leaf set (§10 V45a) — every tag the model returned that resolved
	// against the graph, canonicalised to its slug, de-duplicated, in stable order. The empty slice is
	// the honest "nothing groundable" case. These are the LEAVES; the store expands them to rollups.
	Tags []string
	// Category is the DERIVED shadow of Tags — the primary product leaf (forest.PrimaryProductLeaf),
	// computed once in validateTags where the forest is in hand, so Score()/Complete() (which run
	// without a forest) can keep asking "has a product tag?" as `Category != ""`. It is never an input;
	// it always equals PrimaryProductLeaf(Tags). "" when the tag set has no product-axis leaf.
	Category string
	// Brand is the advertiser the clip is FOR — "Kellogg's", "Ford" (§10 V44). Free text, not an
	// enum: the set of advertisers is open in a way the tag vocabulary is not.
	//
	// ⚠ GROUNDED like era, and dropped for the same reason it is. validateTags keeps a brand ONLY
	// when it appears literally in the clip's text signals; a brand the model inferred from a
	// category or a tone is emptied here, never persisted. Unlike era there is no "suggested brand"
	// half — an ungrounded advertiser is not a question worth asking a human, it is a guess, so it
	// simply vanishes. "" is the honest common case.
	Brand string
	// SuggestedEra is an era the model proposed whose year does NOT appear in the
	// text signals (§10 era grounding, V34). It is never written to the clip's
	// Era — matching must not see it — and is recorded on the clip as a
	// suggestion for the operator to confirm (PATCH /v1/filler/{id} setting era
	// confirms and clears it). 0 = no suggestion.
	SuggestedEra int
	// Confidence (0-100) is the grounding-CAPPED score that decides auto-filing vs a human
	// (§10 V38). Computed by Score() — never taken from the model directly.
	Confidence int
}

// Complete reports whether all three tags resolved — only then is the clip fully
// matchable (§10).
func (t TagSuggestion) Complete() bool {
	return t.Era > 0 && t.Audience != "" && t.Category != ""
}

// Confidence ceilings (§10 V38). Each names what the GROUNDING could verify, never what the
// model claimed. They are ceilings rather than penalties so the worst fact about a clip decides
// its score — a clip cannot average its way past a fabricated era.
const (
	// Everything verified: the era's year appears literally in the text, and both audience and
	// category matched a known enum. This is the only state that may be auto-filed.
	confGrounded = 100
	// ⚠ THE load-bearing one. The model proposed an era that is NOT in the clip's text — the
	// exact failure 00024 and §10's grounding rule exist for. Capped BELOW any threshold an
	// operator can set, so an inferred era can never be filed without a human, whatever the
	// model reports and whatever the threshold is set to.
	confUngroundedEra = 40
	// Tags resolved but there was no era at all (the model returned 0). Honest, incomplete, and
	// not a fabrication — worth a human's glance, not an alarm.
	confNoEra = 60
	// A field the model returned was not a real enum value and got dropped, so the clip is
	// partially untagged.
	confPartial = 50
)

// MaxAutoFileConfidence is the highest threshold an operator may set. ⚠ It exists so that
// `confUngroundedEra` stays strictly below EVERY reachable threshold: a settings value above
// this would let an operator configure their way into auto-filing inferred eras, which is the
// one thing §10 says the cap must prevent. Enforced by the settings registry's range.
const MaxAutoFileConfidence = 95

// Score returns this suggestion's confidence (0-100) — the number that decides whether a clip is
// filed automatically or surfaced to a human (§10 V38).
//
// ⚠ **Two layers, and only the grounding layer can RAISE it.** The ceiling below comes entirely
// from what `validateTags` could verify against the clip's own text. `modelConfidence` (0-100,
// 0 = the model said nothing) may only lower that ceiling, never lift it.
//
// The asymmetry is the whole point. This tagger has a measured history of confident fabrication
// — it invented an era on 2 of 10 real clips, inferred from tone (§10, plan §6.4) — so a model
// that is *certain* about an era it made up must not be able to talk its way past the gate. A
// model that is *unsure* about a clip whose tags all verify is still worth surfacing, which is
// why lowering is allowed.
func (t TagSuggestion) Score(modelConfidence int) int {
	ceiling := confGrounded
	switch {
	case t.SuggestedEra > 0:
		// An ungrounded era. Checked FIRST: a clip can be both ungrounded and partial, and the
		// fabrication is the more serious fact, so it must win.
		ceiling = confUngroundedEra
	case t.Audience == "" || t.Category == "":
		ceiling = confPartial
	case t.Era == 0:
		ceiling = confNoEra
	}
	if modelConfidence > 0 && modelConfidence < ceiling {
		return modelConfidence
	}
	return ceiling
}

// tagOutput is the model's raw JSON classification (untrusted — validated before use).
type tagOutput struct {
	Era      int    `json:"era"`
	Audience string `json:"audience"`
	// The axis-shaped fields make the model answer the independent taxonomy questions independently:
	// what the clip sells, what it is, when it belongs, and which audience cues it carries. A flat
	// bag made `local` look plausible for political/news clips because the model lost the fact that
	// `local` is on the PRODUCT axis. Each value is still untrusted and resolve-or-dropped below.
	Product      string   `json:"product"`
	Format       string   `json:"format"`
	Seasonal     []string `json:"seasonal"`
	AudienceCues []string `json:"audienceCues"`
	// Tags is a compatibility input for providers/cached responses still returning the pre-axis
	// shape. The current prompt does not ask for it. It remains resolve-or-drop, never authoritative.
	Tags []string `json:"tags,omitempty"`
	// Brand is the advertiser the model read off the text (§10 V44). Untrusted like every other
	// field here: validateTags keeps it only if it appears literally in the clip's text signals,
	// otherwise it is dropped — a brand is never allowed to raise Confidence (it rides the existing
	// grounded ceiling), so it does not appear in Score at all.
	Brand string `json:"brand"`
	// Confidence is the model's own 0-100 self-assessment (V38). ⚠ **Advisory only, and it can
	// only LOWER the grounded ceiling** — see TagSuggestion.Score. It is read from the same
	// untrusted JSON as everything else here, so it is clamped before use: a model returning 150
	// must not become a score of 150.
	Confidence int `json:"confidence"`
}

// Classify asks the LLM to tag a clip from its text signals and returns the
// VALIDATED result (§10). It uses JSON mode; every tag the model returns is
// grounded against `forest` (dropped if it does not resolve), and an era whose
// year is absent from the text comes back as SuggestedEra, never Era. Never
// invents a clip — the caller hands it a real one from the catalog.
//
// ⚠ `forest` is REQUIRED (§10 V45a): it is both the vocabulary SERVED to the model
// (so it never guesses a slug blind) and the grounding gate the answer is resolved
// against — the served list and the accepted list are the same list by
// construction, the same "BE is the single source of the vocabulary" discipline
// the suggester uses. A nil forest tags nothing (every slug fails to resolve),
// which is the safe direction.
func Classify(ctx context.Context, provider llm.Provider, forest *taxonomy.Forest, name, sourceText string) (TagSuggestion, error) {
	messages := []llm.Message{
		{Role: llm.System, Content: tagSystemPrompt(forest)},
		{Role: llm.User, Content: tagUserPrompt(name, sourceText)},
	}
	resp, err := provider.Chat(ctx, messages, llm.ChatOptions{JSONMode: true})
	if err != nil {
		return TagSuggestion{}, fmt.Errorf("classify clip: %w", err)
	}
	var out tagOutput
	// Unwrap a code fence before parsing, the same robustness the vision tier needs (§10 V44):
	// JSONMode keeps qwen3's output bare, but a model that wraps its answer in ```json … ``` would
	// otherwise fail on the leading backtick. llm.ExtractJSONObject is a no-op on already-bare JSON.
	if err := json.Unmarshal([]byte(llm.ExtractJSONObject(resp.Content)), &out); err != nil {
		return TagSuggestion{}, fmt.Errorf("classify clip: model output not JSON: %w", err)
	}
	return validateTags(out, forest, name+"\n"+sourceText), nil
}

// validateTags is the grounding pass for tagging: every field must be a real enum
// value / plausible year / literally-present brand, else it's dropped (§10 — no
// hallucinated tags persisted).
//
// ⚠ ERA IS SPECIAL (V34): a plausible year is not enough. Measured on real
// transcripts, the model inferred a decade from TONE on 2 of 10 clips (era 1980
// with no year anywhere in the text) and this validator had no way to tell an
// inferred year from a read one — both would have persisted as fact, violating
// §8. So an era is accepted only when the year appears LITERALLY in the text
// signals (filename, sidecar text, or transcript); otherwise it is demoted to
// SuggestedEra for operator confirmation. The asymmetry is safe: dropping a
// true positive leaves the clip untagged (a human or a later run can tag it),
// while accepting a fabrication corrupts era-matching silently.
func validateTags(out tagOutput, forest *taxonomy.Forest, text string) TagSuggestion {
	var t TagSuggestion
	if out.Era >= 1930 && out.Era <= 2035 {
		if strings.Contains(text, strconv.Itoa(out.Era)) {
			t.Era = out.Era
		} else {
			t.SuggestedEra = out.Era
		}
	}
	if aud := AudienceFromString(out.Audience); aud != "" {
		t.Audience = aud
	}
	// ⚠ TAGS ARE GROUNDED resolve-or-drop against the taxonomy (§10 V45a) — the anti-fabrication gate,
	// the same discipline era and brand get. Each returned slug is run through forest.Resolve: an exact
	// slug or a known synonym/retired alias is kept and canonicalised (`brew` → `beer`); anything else
	// is DROPPED, never minted as a new taxon (only an operator adds taxa). De-duplicated and kept in
	// stable order so a re-tag of the same clip is reproducible. A nil forest resolves nothing, so an
	// install with no taxonomy tags nothing rather than persisting raw model output.
	if forest != nil {
		seen := map[string]bool{}
		add := func(raw string, axis taxonomy.Axis) {
			if slug, ok := forest.Resolve(raw); ok && !seen[slug] {
				taxon, exists := forest.Get(slug)
				if axis != "" && (!exists || taxon.Axis != axis) {
					return
				}
				seen[slug] = true
				t.Tags = append(t.Tags, slug)
			}
		}
		for _, raw := range out.Tags {
			add(raw, "") // compatibility bag: the slug itself still declares its axis
		}
		add(out.Product, taxonomy.AxisProduct)
		add(out.Format, taxonomy.AxisFormat)
		for _, raw := range out.Seasonal {
			add(raw, taxonomy.AxisSeasonal)
		}
		for _, raw := range out.AudienceCues {
			add(raw, taxonomy.AxisAudienceCue)
		}
		sort.Strings(t.Tags)
		// Category is the DERIVED shadow: the primary product leaf of the grounded set. Computed here,
		// where the forest is in hand, so Score()/Complete() stay forest-free (see TagSuggestion.Category).
		t.Category = forest.PrimaryProductLeaf(t.Tags)
	}
	// ⚠ BRAND IS GROUNDED, exactly like era and for the same §8 reason (V44). The advertiser set is
	// open, so there is no enum to validate against — the only defence against a fabricated brand
	// ("this FEELS like a Coke ad") is the text itself. A brand is kept ONLY when it appears
	// literally in the clip's text signals, and dropped otherwise. There is deliberately no
	// SuggestedBrand counterpart — an ungrounded era is a question a human can confirm from the
	// year, an ungrounded brand is just a guess, so it vanishes rather than riding along.
	//
	// ⚠ Matched on a PUNCTUATION-FOLDED form, not a raw substring. A live test found the raw check
	// too brittle: the model answers "Campbell's" while the archive.org identifier is
	// "CampbellsSoupAdvert" — no apostrophe — so `Contains` rejected a CORRECT brand. Folding
	// apostrophes/punctuation and collapsing spaces on BOTH sides lets "Campbell's" ground against
	// "campbellssoupadvert" while still requiring the brand's letters to appear in order — the
	// anti-fabrication guarantee is unchanged (a brand the text does not contain still cannot pass;
	// "Coke" does not fold into a Campbell's filename), only the punctuation noise is tolerated.
	if brand := strings.TrimSpace(out.Brand); brand != "" &&
		strings.Contains(foldForGrounding(text), foldForGrounding(brand)) {
		t.Brand = brand
	}
	// ⚠ Clamped before it reaches Score, because this arrives in the same untrusted JSON as the
	// tags: a model returning 150 (or -1) must not become a score. Out-of-range reads as "the
	// model said nothing", which lets the grounding ceiling stand on its own.
	model := out.Confidence
	if model < 0 || model > 100 {
		model = 0
	}
	t.Confidence = t.Score(model)
	return t
}

// foldForGrounding normalises a string for the brand containment check: lowercased, with every
// character that is not a letter or digit removed (apostrophes, spaces, punctuation, hyphens).
//
// ⚠ This is what makes brand grounding survive real filenames without weakening it (§10 V44, found
// by a live test). The model answers "Campbell's"; archive.org's identifier is "CampbellsSoupAdvert"
// — folding both to "campbells…" lets the correct brand ground, while a brand the text genuinely
// does NOT contain still fails: the letters must appear IN ORDER, so "coke" cannot fold into a
// Campbell's filename. It tolerates punctuation noise, not fabrication. Deliberately NOT applied to
// era — a year is digits only, has no punctuation to fold, and its literal-substring rule is the
// exact §8 guarantee that must stay strict.
func foldForGrounding(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tagSystemPrompt builds the classifier prompt, SERVING the taxonomy vocabulary (§10 V45a) so the
// model picks tags from the same list the grounding will accept — the served set and the accepted set
// are one set by construction (mirrors how the suggester serves schedule.BuildVocabulary). The vocab is
// grouped by axis (product / format / seasonal / audience-cue); the model may return several tags
// across axes (a Christmas beer ad is `beer` + `christmas`). A nil forest serves an empty vocabulary,
// which yields no groundable tags — the safe direction for an install with no taxonomy.
func tagSystemPrompt(forest *taxonomy.Forest) string {
	vocab := ""
	if forest != nil {
		vocab = forest.Vocab()
	}
	return `You classify a short TV filler clip (a commercial/bumper/PSA) from its text only.
Return ONLY this JSON, no prose:
{"era":<4-digit year or 0 if unknown>,"audience":"kids|family|general|late_night","product":"<one product slug or empty>","format":"<one format slug or empty>","seasonal":["<seasonal slug>", ...],"audienceCues":["<audience-cue slug>", ...],"brand":"<advertiser name or empty>","confidence":<0-100>}
Choose values ONLY from this vocabulary, grouped by axis. Entries written as "child (under parent)"
describe hierarchy; emit only the slug, never the annotation. Classify every identifiable axis
independently. On each axis emit only the most-specific applicable taxon, never its ancestors:
` + vocab + `
Rules: give era ONLY when a 4-digit year literally appears in the text — never infer a decade from
tone, style, or products; use 0 for era if no year appears. Pick the closest audience from the list.
Product asks WHAT IS BEING SOLD. For a product advertisement, include its most-specific product
whenever the advertised product is identifiable. Reliable common knowledge about an explicitly
named product or brand is allowed even when the generic category word is absent: Coca-Cola implies
soda, Ford Mustang implies cars, McDonald's implies fast_food, and a department-store advert implies
retail. This product rule does NOT relax era or brand grounding. For a station ID, newsbreak,
political campaign ad, commercial-break compilation, or unknown clip with no identifiable advertised
product, use an empty product rather than forcing one. The product slug local means a local BUSINESS,
never merely local geography, politics, news, or a station. Format asks WHAT THE CLIP IS; a paid
political campaign ad is commercial, not psa. Use ONLY slugs from the vocabulary — never invent one.
Give brand ONLY when the advertiser's name
literally appears in the text (e.g. "Kellogg's", "Ford") — never guess it from the products, and use
"" if no name appears. Set confidence to certainty in the complete classification, where 100 means
the evidence is explicit and 0 means you are guessing.`
}

func tagUserPrompt(name, sourceText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Filename/title: %s\n", name)
	if sourceText != "" {
		fmt.Fprintf(&b, "Source description: %s\n", sourceText)
	}
	b.WriteString("Classify it.")
	return b.String()
}
