package schedule

import "github.com/loomarr/loomarr/internal/holidayvocab"

// The authoring vocabulary served to the rules editor (programming-design §6.6, §8.1). Until
// now the FE hand-mirrored presets.go BYTE-FOR-BYTE (its presets.ts said "keep in sync") — a
// standing drift hazard. BuildVocabulary makes the BE the single source: it enumerates the
// canonical tokens + human labels and derives each token's LOWERED value from the same
// Lower* functions the suggester and the write path use, so the picker the editor renders and
// the rule it lowers to are identical to the BE's, by construction. Parametric tokens
// (series:<key>, genre:<name>, era:<range>) are not enumerable and stay client-composed; the
// BE still re-validates every write, so those remain fail-safe.

// Vocabulary is the closed WHEN/WHAT/HOW authoring vocabulary (§6.6).
type Vocabulary struct {
	When []WhenVocab `json:"when"`
	What []WhatVocab `json:"what"`
	How  []HowVocab  `json:"how"`
}

// WhenVocab is one WHEN preset: its token, labels, and the predicate + default priority the
// BE lowers it to (so the editor's preview matches the reconciler's evaluation).
type WhenVocab struct {
	Token      string        `json:"token"`
	Label      string        `json:"label"`
	ShortLabel string        `json:"shortLabel"`
	Predicate  WhenPredicate `json:"predicate"`
	Priority   int           `json:"priority"`
}

// WhatVocab is one non-parametric WHAT preset (parametric series:/genre:/era: are excluded —
// they're composed from the channel's lineup client-side). Scope is nil for "no narrowing".
type WhatVocab struct {
	Token string       `json:"token"`
	Label string       `json:"label"`
	Scope *ScopePolicy `json:"scope,omitempty"`
}

// HowVocab is one HOW preset: token, labels, the RuleOrdering it lowers to, and whether it
// pins the rolling window to the full run (a marathon binge — WindowFull).
type HowVocab struct {
	Token      string       `json:"token"`
	Label      string       `json:"label"`
	ShortLabel string       `json:"shortLabel"`
	Ordering   RuleOrdering `json:"ordering"`
	WindowFull bool         `json:"windowFull"`
}

// The canonical token + label tables. Tokens MUST be ones Lower* recognizes (a test asserts
// it) — the labels are the only genuinely new data here; the lowered values come from Lower*.
var (
	whenVocabTokens = []struct{ token, label, short string }{
		{"weekend", "Weekend", "Weekend"},
		{"weekday", "Weekday", "Weekday"},
		{"mornings", "Mornings (6–10)", "Mornings"},
		{"daytime", "Daytime (10–17)", "Daytime"},
		{"primetime", "Primetime (20–23)", "Primetime"},
		{"late-night", "Late night (23–2)", "Late night"},
		{"overnight", "Overnight (2–6)", "Overnight"},
	}
	whatVocabTokens = []struct{ token, label string }{
		{"all", "Anything (no extra narrowing)"},
		{"kids", "Kids-safe genres"},
		{"family", "Family genres"},
		{"holiday-matched", "Holiday-matched titles"},
	}
	howVocabTokens = []struct{ token, label, short string }{
		{"syndication", "Syndication (the deck order)", "Syndication"},
		{"shuffle", "Shuffle", "Shuffle"},
		{"marathon", "Marathon (binge one show, no breaks)", "Marathon"},
		{"feature", "Feature (in order)", "Feature"},
	}
)

// BuildVocabulary assembles the vocabulary, lowering each token through the same Lower*
// functions every other caller uses. A token that fails to lower is skipped (it can't be a
// real preset) — the parity test guarantees this never silently drops a shipped token.
func BuildVocabulary() Vocabulary {
	v := Vocabulary{}

	for _, w := range whenVocabTokens {
		if pred, prio, ok := LowerWhen(w.token); ok {
			v.When = append(v.When, WhenVocab{Token: w.token, Label: w.label, ShortLabel: w.short, Predicate: pred, Priority: prio})
		}
	}
	for _, h := range holidayvocab.Definitions() {
		token := "holiday:" + h.ID
		if pred, prio, ok := LowerWhen(token); ok {
			v.When = append(v.When, WhenVocab{Token: token, Label: h.Label, ShortLabel: h.Label, Predicate: pred, Priority: prio})
		}
	}

	for _, w := range whatVocabTokens {
		// LowerWhat("all") returns ok=false (nil scope, no narrowing) by design; treat the
		// static tokens as valid vocabulary regardless, carrying whatever scope they lower to.
		scope, _, _ := LowerWhat(w.token)
		v.What = append(v.What, WhatVocab{Token: w.token, Label: w.label, Scope: scope})
	}

	for _, h := range howVocabTokens {
		if ord, win, ok := LowerHow(h.token); ok {
			v.How = append(v.How, HowVocab{Token: h.token, Label: h.label, ShortLabel: h.short, Ordering: ord, WindowFull: win == WindowFull})
		}
	}

	return v
}
