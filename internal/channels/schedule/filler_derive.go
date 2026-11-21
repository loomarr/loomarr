package schedule

import (
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/platform/rating"
)

// DerivedFiller is the filler context a channel implies through its programming, for the
// fields its operator left unset (§10). It is computed live, never persisted: a stored copy
// would go stale the moment the lineup changes, and would be indistinguishable from an
// operator's answer.
type DerivedFiller struct {
	Audience string // "" = nothing derivable; else kids|family|general
	Era      *Range // nil = nothing derivable
	// Network is the broadcast network most of the lineup's series premiered on, as a
	// NetworkKey (#4432): a channel airs only that network's branded filler. "" = none, which a
	// tie, an all-movie lineup and unstamped entries all give; such a channel airs no network's.
	Network string
}

// NetworkKey is a broadcast network's name reduced to what two sources agree on, so a clip's
// "CBS" meets a series' "CBS" and "The WB" meets "WB": upper case, without a leading "THE", letters
// and digits only. "" stays "".
func NetworkKey(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "THE ")
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// majorityNetwork is the network the most entries premiered on (their first network); "" when
// none has one or two networks tie for the most.
func majorityNetwork(lineup []LineupEntry) string {
	counts := map[string]int{}
	for _, e := range lineup {
		if len(e.Networks) > 0 {
			if k := NetworkKey(e.Networks[0]); k != "" {
				counts[k]++
			}
		}
	}
	best, tied := "", false
	for k, n := range counts {
		switch {
		case best == "" || n > counts[best]:
			best, tied = k, false
		case n == counts[best]:
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}

// DeriveFiller reads a channel's programming for the filler context it implies.
//
// Audience: the channel's audience ceiling when it has one, otherwise the HIGHEST-ranked
// content rating in its lineup — so a single adult title keeps a mostly-kids lineup out of
// `kids`, the safe direction. Unrated entries are ignored; a lineup with no ratings derives
// nothing (the channel then behaves as it always did, with no audience narrowing).
//
// Era: the span of the lineup's known release years. Scope-declared eras are resolved by the
// caller first and win over this; a year of 0 is unknown and never widens the span.
func DeriveFiller(policy ChannelPolicy, lineup []LineupEntry) DerivedFiller {
	var out DerivedFiller

	if policy.Audience.Ceiling != "" {
		out.Audience = fillerAudienceForCeiling(policy.Audience.Ceiling)
	} else {
		top, found := -1, rating.Rating("")
		for _, e := range lineup {
			r := rating.Normalize(string(e.OfficialRating))
			if rank, ok := r.Rank(); ok && rank > top {
				top, found = rank, r
			}
		}
		if top >= 0 {
			out.Audience = fillerAudienceForCeiling(found)
		}
	}

	var era Range
	for _, e := range lineup {
		if e.Year <= 0 {
			continue
		}
		if era.From == 0 || e.Year < era.From {
			era.From = e.Year
		}
		if e.Year > era.To {
			era.To = e.Year
		}
	}
	if era.From > 0 {
		out.Era = &era
	}
	out.Network = majorityNetwork(lineup)
	return out
}
