package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/llm"
)

// AI-assisted tagging (§10): classify an ingested clip's TEXT SIGNALS —
// filename, the source title/description yt-dlp/Archive preserve, and (for clips
// produced by compilation splitting, V34) the segment transcript — into
// era/audience/category, via the configured LLM. Grounding here is
// enum-validation PLUS the era rule: the model classifies a clip we hand it (it
// can't invent a clip), its audience/category are validated against the known
// sets, and its era is accepted ONLY when the year appears literally in the
// text signals — a measured §8 hole (2 of 10 real transcripts got an era
// inferred from tone, plan §6.4). An ungrounded era is NOT persisted as fact:
// it comes back as SuggestedEra for the operator to confirm.

// knownCategories is the closed set the classifier may assign (§10). An
// out-of-set category is rejected (kept untagged rather than persisting garbage).
var knownCategories = map[string]bool{
	"toys": true, "cereal": true, "cars": true, "tech": true, "fast_food": true,
	"movie_trailer": true, "candy": true, "games": true, "psa": true, "ident": true,
	"bumper": true, "general": true,
}

// TagSuggestion is the validated classification for one clip. Fields are zero
// when the model gave an unusable/out-of-enum value (that field stays untagged).
type TagSuggestion struct {
	Era      int
	Audience Audience
	Category string
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
	Category string `json:"category"`
	// Confidence is the model's own 0-100 self-assessment (V38). ⚠ **Advisory only, and it can
	// only LOWER the grounded ceiling** — see TagSuggestion.Score. It is read from the same
	// untrusted JSON as everything else here, so it is clamped before use: a model returning 150
	// must not become a score of 150.
	Confidence int `json:"confidence"`
}

// Classify asks the LLM to tag a clip from its text signals and returns the
// VALIDATED result (§10). It uses JSON mode; any field the model returns that
// isn't a real enum value is dropped (that tag stays empty), and an era whose
// year is absent from the text comes back as SuggestedEra, never Era. Never
// invents a clip — the caller hands it a real one from the catalog.
func Classify(ctx context.Context, provider llm.Provider, name, sourceText string) (TagSuggestion, error) {
	messages := []llm.Message{
		{Role: llm.System, Content: tagSystemPrompt},
		{Role: llm.User, Content: tagUserPrompt(name, sourceText)},
	}
	resp, err := provider.Chat(ctx, messages, llm.ChatOptions{JSONMode: true})
	if err != nil {
		return TagSuggestion{}, fmt.Errorf("classify clip: %w", err)
	}
	var out tagOutput
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return TagSuggestion{}, fmt.Errorf("classify clip: model output not JSON: %w", err)
	}
	return validateTags(out, name+"\n"+sourceText), nil
}

// validateTags is the grounding pass for tagging: every field must be a real enum
// value / plausible year, else it's dropped (§10 — no hallucinated tags persisted).
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
func validateTags(out tagOutput, text string) TagSuggestion {
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
	if cat := strings.ToLower(strings.TrimSpace(out.Category)); knownCategories[cat] {
		t.Category = cat
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

const tagSystemPrompt = `You classify a short TV filler clip (a commercial/bumper/PSA) from its text only.
Return ONLY this JSON, no prose:
{"era":<4-digit year or 0 if unknown>,"audience":"kids|family|general|late_night","category":"toys|cereal|cars|tech|fast_food|movie_trailer|candy|games|psa|ident|bumper|general","confidence":<0-100>}
Rules: give era ONLY when a 4-digit year literally appears in the text — never infer a decade from tone, style, or products; use 0 for era if no year appears; pick the closest audience and category from the lists ONLY; never invent values outside the lists; set confidence to how certain you are that these tags are right, where 100 means the text states them outright and 0 means you are guessing.`

func tagUserPrompt(name, sourceText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Filename/title: %s\n", name)
	if sourceText != "" {
		fmt.Fprintf(&b, "Source description: %s\n", sourceText)
	}
	b.WriteString("Classify it.")
	return b.String()
}
