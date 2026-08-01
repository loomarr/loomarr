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
}

// Complete reports whether all three tags resolved — only then is the clip fully
// matchable (§10).
func (t TagSuggestion) Complete() bool {
	return t.Era > 0 && t.Audience != "" && t.Category != ""
}

// tagOutput is the model's raw JSON classification (untrusted — validated before use).
type tagOutput struct {
	Era      int    `json:"era"`
	Audience string `json:"audience"`
	Category string `json:"category"`
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
	return t
}

const tagSystemPrompt = `You classify a short TV filler clip (a commercial/bumper/PSA) from its text only.
Return ONLY this JSON, no prose:
{"era":<4-digit year or 0 if unknown>,"audience":"kids|family|general|late_night","category":"toys|cereal|cars|tech|fast_food|movie_trailer|candy|games|psa|ident|bumper|general"}
Rules: give era ONLY when a 4-digit year literally appears in the text — never infer a decade from tone, style, or products; use 0 for era if no year appears; pick the closest audience and category from the lists ONLY; never invent values outside the lists.`

func tagUserPrompt(name, sourceText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Filename/title: %s\n", name)
	if sourceText != "" {
		fmt.Fprintf(&b, "Source description: %s\n", sourceText)
	}
	b.WriteString("Classify it.")
	return b.String()
}
