// Package holidayvocab owns Loomarr's immutable built-in holiday identities and aliases.
package holidayvocab

import (
	"slices"

	"github.com/loomarr/loomarr/internal/textmatch"
)

type definition struct {
	id              string
	label           string
	intentAliases   []string
	evidenceAliases []string
}

var builtins = [...]definition{
	{
		id: "halloween", label: "Halloween",
		intentAliases:   []string{"halloween"},
		evidenceAliases: []string{"halloween", "spooky", "haunted", "ghost", "witch", "horror"},
	},
	{
		id: "thanksgiving", label: "Thanksgiving",
		intentAliases:   []string{"thanksgiving", "turkey day"},
		evidenceAliases: []string{"thanksgiving", "turkey day"},
	},
	{
		id: "christmas", label: "Christmas",
		intentAliases:   []string{"christmas", "xmas", "santa", "noel", "yuletide"},
		evidenceAliases: []string{"christmas", "xmas", "santa", "holiday", "noel", "yuletide"},
	},
	{
		id: "newyear", label: "New Year",
		intentAliases:   []string{"new year's", "new years", "new year", "nye", "hogmanay"},
		evidenceAliases: []string{"new year's", "new years", "new year", "nye", "hogmanay"},
	},
	{
		id: "valentines", label: "Valentine's Day",
		intentAliases:   []string{"valentine's", "valentines", "valentine"},
		evidenceAliases: []string{"valentine's", "valentines", "valentine"},
	},
}

// Definition is one immutable built-in identity exposed without its private alias slices.
type Definition struct {
	ID    string
	Label string
}

// Definitions returns the closed built-in vocabulary in stable display order.
func Definitions() []Definition {
	out := make([]Definition, len(builtins))
	for i, builtin := range builtins {
		out[i] = Definition{ID: builtin.id, Label: builtin.label}
	}
	return out
}

// MatchIntent returns holiday ids named by whole-phrase affirmative intent text.
func MatchIntent(text string) []string {
	var out []string
	for _, builtin := range builtins {
		for _, alias := range builtin.intentAliases {
			if textmatch.ContainsPhrase(text, alias) {
				out = append(out, builtin.id)
				break
			}
		}
	}
	return out
}

// EvidenceAliases returns a copy of the thematic aliases for one built-in id.
func EvidenceAliases(id string) []string {
	for _, builtin := range builtins {
		if builtin.id == id {
			return slices.Clone(builtin.evidenceAliases)
		}
	}
	return nil
}

// Known reports whether id belongs to the closed built-in vocabulary.
func Known(id string) bool {
	return len(EvidenceAliases(id)) > 0
}
