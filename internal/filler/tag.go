package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mantonx/loomarr/internal/llm"
)

// AI-assisted tagging (§10): classify an ingested clip's TEXT SIGNALS ONLY —
// filename + the source title/description yt-dlp/Archive preserve — into
// era/audience/category, via the configured LLM. Transcript/vision tagging is
// future work (§20). Grounding here is enum-validation: the model classifies a
// clip we hand it (it can't invent a clip), and its era/audience/category are
// validated against the known sets before any write — a hallucinated audience is
// dropped, not persisted.

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
// isn't a real enum value is dropped (that tag stays empty). Never invents a
// clip — the caller hands it a real one from the catalog.
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
	return validateTags(out), nil
}

// validateTags is the grounding pass for tagging: every field must be a real enum
// value / plausible year, else it's dropped (§10 — no hallucinated tags persisted).
func validateTags(out tagOutput) TagSuggestion {
	var t TagSuggestion
	if out.Era >= 1930 && out.Era <= 2035 {
		t.Era = out.Era
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
Rules: use 0 for era if you can't tell; pick the closest audience and category from the lists ONLY; never invent values outside the lists.`

func tagUserPrompt(name, sourceText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Filename/title: %s\n", name)
	if sourceText != "" {
		fmt.Fprintf(&b, "Source description: %s\n", sourceText)
	}
	b.WriteString("Classify it.")
	return b.String()
}
