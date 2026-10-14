package schedule

// DerivedFiller is the filler context a channel implies through its programming, for the
// fields its operator left unset (§10). It is computed live, never persisted: a stored copy
// would go stale the moment the lineup changes, and would be indistinguishable from an
// operator's answer.
type DerivedFiller struct {
	Audience string // "" = nothing derivable; else kids|family|general
	Era      *Range // nil = nothing derivable
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
		top, found := -1, Rating("")
		for _, e := range lineup {
			r := NormalizeRating(string(e.OfficialRating))
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
	return out
}
