package librarystore

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SortKey is the key a title sorts by, made from its sort title or, with none, its name: lower
// case with the accents taken off, so "É" sorts with "E" and not after "Z", as bytes and the
// database's collation would put it.
func SortKey(title string) string {
	decomposed := norm.NFD.String(strings.ToLower(title))
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
		case sortLetters[r] != "":
			b.WriteString(sortLetters[r])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sortLetters are the lower-case letters with no decomposition that sort as other letters.
var sortLetters = map[rune]string{'ø': "o", 'æ': "ae", 'œ': "oe", 'ß': "ss", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th"}
