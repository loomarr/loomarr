package watermark

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// CallsignProposer is the seam for the issue's LLM callsign pick: one small call after channel
// creation, off Add a channel's latency path, its answer stored as policy.watermark.callsign so
// airtime never waits on a model. It is deliberately not wired while the LLM is off; with no
// stored callsign, DeriveCallsign names every Plate.
type CallsignProposer interface {
	ProposeCallsign(ctx context.Context, channelName string) (string, error)
}

// callsignStopwords are skipped for initials: "The Sci-Fi Vault" is SV.
var callsignStopwords = map[string]bool{"a": true, "an": true, "and": true, "the": true, "of": true,
	"for": true, "to": true, "in": true, "on": true, "with": true, "n": true}

// DeriveCallsign is the Plate's text when the channel has none, broadcast-short and deterministic:
// an acronym the name already has (TGIF), else a decade or number (80s), else the initials of its
// significant words (Saturday Cartoons: SC, at most 4), else its one word upper-cased (at most 8),
// else "CH<number>". Words keep only letters and digits, so "Sci-Fi" is one word.
func DeriveCallsign(name string, number int) string {
	var words, significant []string
	for _, f := range strings.Fields(name) {
		w := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, f)
		if w == "" {
			continue
		}
		words = append(words, w)
		if !callsignStopwords[strings.ToLower(w)] {
			significant = append(significant, w)
		}
	}
	// An all-caps name is shouting, not a string of acronyms.
	if strings.ToUpper(name) != name {
		for _, w := range words {
			if r := []rune(w); len(r) >= 2 && len(r) <= 5 && unicode.IsLetter(r[0]) && w == strings.ToUpper(w) {
				return w
			}
		}
	}
	for _, w := range words {
		if r := []rune(w); unicode.IsDigit(r[0]) && len(r) <= 5 {
			return w
		}
	}
	if len(significant) >= 2 {
		var initials []rune
		for _, w := range significant[:min(len(significant), 4)] {
			initials = append(initials, unicode.ToUpper([]rune(w)[0]))
		}
		return string(initials)
	}
	if len(significant) == 0 {
		significant = words
	}
	if len(significant) > 0 {
		r := []rune(strings.ToUpper(significant[0]))
		return string(r[:min(len(r), 8)])
	}
	return fmt.Sprintf("CH%d", number)
}
