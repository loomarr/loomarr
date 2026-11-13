package scan

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// editionTag matches the edition a file names, "{edition-Director's Cut}" (decision 54).
var editionTag = regexp.MustCompile(`(?i)\{edition-([^{}]+)\}`)

// editionIn is the edition a file's name gives; "" for none.
func editionIn(name string) string {
	if m := editionTag.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// fileEdition is the edition a file's own records give it, labelled: its arr's, else its name's,
// else its NFO's. It is the file's, not the title's, so no catalog has a say.
func fileEdition(records []mp.Record) string {
	for _, basis := range []mp.Basis{mp.BasisManager, mp.BasisEmbeddedID, mp.BasisSidecar} {
		for _, rec := range records {
			if rec.Source.Basis == basis && rec.Descriptive != nil && rec.Descriptive.Edition != "" {
				return editionLabel(rec.Descriptive.Edition)
			}
		}
	}
	return ""
}

// editionLabel is how an edition is shown (decision 54): as its source gave it, with each word
// capitalised when it came in one case ("EXTENDED", "directors cut"). The theatrical version is
// the default and has no label.
func editionLabel(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || editionKey(s) == "theatrical" {
		return ""
	}
	if s != strings.ToUpper(s) && s != strings.ToLower(s) {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r, n := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[n:]
	}
	return strings.Join(words, " ")
}

// editionKey is what two labels of one edition share: "Extended" and "EXTENDED Cut", or
// "Director's Cut" and "Directors Cut", are the same edition.
func editionKey(label string) string {
	k := strings.ToLower(strings.Join(strings.Fields(label), " "))
	k = strings.NewReplacer("'", "", "’", "").Replace(k)
	for _, suffix := range []string{" cut", " edition", " version"} {
		k = strings.TrimSuffix(k, suffix)
	}
	return k
}

// playingCopies is which of a title's n files play: one in each edition (decision 54), or each
// part of an edition in parts, picked among the copies of it as playingCopy picks (decision 37).
// An edition with a whole file plays that, and its parts are extra copies.
func playingCopies(n int, edition func(int) string, part func(int) int, managed func(int) bool, pixels func(int) int, size, length func(int) int64) []bool {
	groups := map[string][]int{}
	whole := map[string]bool{}
	for i := range n {
		e := editionKey(edition(i))
		k := e + "\x00" + strconv.Itoa(part(i))
		groups[k] = append(groups[k], i)
		whole[e] = whole[e] || part(i) == 0
	}
	plays := make([]bool, n)
	for _, g := range groups {
		if part(g[0]) != 0 && whole[editionKey(edition(g[0]))] {
			continue
		}
		best := playingCopy(len(g), func(j int) bool { return managed(g[j]) },
			func(j int) int { return pixels(g[j]) }, func(j int) int64 { return size(g[j]) }, func(j int) int64 { return length(g[j]) })
		plays[g[best]] = true
	}
	return plays
}
