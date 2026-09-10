// Package textmatch owns deterministic, Unicode-aware whole-word phrase matching.
package textmatch

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// ContainsPhrase reports whether phrase occurs in text as a whole normalized
// word sequence. Punctuation and repeated whitespace are separators, so
// "start-to-finish" matches "start to finish", while "classic" does not match
// "classical".
func ContainsPhrase(text, phrase string) bool {
	text = normalize(text)
	phrase = normalize(phrase)
	return phrase != "" && strings.Contains(" "+text+" ", " "+phrase+" ")
}

func normalize(text string) string {
	// Compatibility decomposition can introduce cased letters (for example ᴬ -> A),
	// so normalize before folding. Normalize once more afterward to keep the folded
	// representation canonical for whole-phrase comparison.
	text = norm.NFKC.String(cases.Fold().String(norm.NFKC.String(text)))
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	}), " ")
}

// ContainsPhraseOutside matches a whole normalized phrase outside occurrences of
// excludedPhrase. Excluded spans remain separators, so masking never joins words
// into new matching phrases. It uses the same Unicode rules as ContainsPhrase.
func ContainsPhraseOutside(text, phrase, excludedPhrase string) bool {
	text = " " + normalize(text) + " "
	phrase = normalize(phrase)
	excludedPhrase = normalize(excludedPhrase)
	if excludedPhrase != "" {
		excluded := " " + excludedPhrase + " "
		for strings.Contains(text, excluded) {
			text = strings.ReplaceAll(text, excluded, " \x00 ")
		}
	}
	return phrase != "" && strings.Contains(text, " "+phrase+" ")
}
