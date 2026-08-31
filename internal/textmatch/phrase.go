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
