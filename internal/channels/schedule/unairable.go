package schedule

import "github.com/loomarr/loomarr/internal/library/provision"

// UnairableTitle is one lineup title that contributes nothing to the channel's cycle, with why
// (#4140 step 1). It answers "why has this never aired" on the channel page. A title the active
// rule or the seasonal bench rests is not here: it airs at its time.
type UnairableTitle struct {
	Key    provision.Key   `json:"key"`
	Title  string          `json:"title"`
	Reason UnairableReason `json:"reason" enum:"not_in_library,no_episodes_in_seasons,over_ceiling,unrated,out_of_scope,no_episodes_selected"`
}

// UnairableReason is why a lineup title cannot air.
type UnairableReason string

const (
	// UnairableNotInLibrary: the library holds no file for the title (never downloaded, or the
	// lineup points at an item the library no longer has).
	UnairableNotInLibrary UnairableReason = "not_in_library"
	// UnairableNoEpisodesInSeasons: the library holds episodes, but none in the seasons the
	// lineup asks for.
	UnairableNoEpisodesInSeasons UnairableReason = "no_episodes_in_seasons"
	// UnairableOverCeiling: the title, or every episode of it, is rated above the audience ceiling.
	UnairableOverCeiling UnairableReason = "over_ceiling"
	// UnairableUnrated: the title has no rating and the channel's ceiling refuses unrated titles.
	UnairableUnrated UnairableReason = "unrated"
	// UnairableOutOfScope: the title, or every episode of it, falls outside the channel's dates,
	// genres or runtime.
	UnairableOutOfScope UnairableReason = "out_of_scope"
	// UnairableNoEpisodesSelected: the lineup's episode selection left nothing to air.
	UnairableNoEpisodesSelected UnairableReason = "no_episodes_selected"
)

// unairableTitles reports, in lineup order, each entry that put no programme into the cycle.
//
//   - entries is the approved lineup; channelEligible is what the audience and scope filters let
//     through, and rested what the active rule or the seasonal bench took out of that for now;
//   - resolved maps each resolved entry to the slots it resolved to;
//   - report is the exclusion report, which names the filter that refused a title or its episodes.
func unairableTitles(entries, channelEligible []LineupEntry, rested map[provision.Key]bool, resolved map[provision.Key][]Slot, report ExclusionReport, avail Availability) []UnairableTitle {
	passed := make(map[provision.Key]bool, len(channelEligible))
	for _, e := range channelEligible {
		passed[e.Key] = true
	}
	refused := map[provision.Key]string{}
	for _, it := range report.Items {
		if _, ok := refused[it.Key]; !ok {
			refused[it.Key] = it.Reason
		}
	}
	var out []UnairableTitle
	for _, e := range entries {
		if rested[e.Key] {
			continue
		}
		var reason UnairableReason
		switch slots, ok := resolved[e.Key]; {
		case !passed[e.Key]:
			reason = filterReason(refused[e.Key])
		case !ok:
			continue // not resolved this computation (the rule's scope took it out): it airs at its time
		case hasProgram(slots):
			continue
		case len(slots) > 0: // a placeholder: nothing the lineup asks for is in the library
			reason = UnairableNotInLibrary
			if e.Key.IsSeries() && len(avail.ResolveEpisodes(e.Key).Programs) > 0 {
				reason = UnairableNoEpisodesInSeasons
			}
		default: // resolved to nothing: the filters refused every episode, or the selection did
			reason = UnairableNoEpisodesSelected
			if r, ok := refused[e.Key]; ok {
				reason = filterReason(r)
			}
		}
		out = append(out, UnairableTitle{Key: e.Key, Title: e.Title, Reason: reason})
	}
	return out
}

func hasProgram(slots []Slot) bool {
	for _, s := range slots {
		if s.IsProgram() {
			return true
		}
	}
	return false
}

// filterReason maps an exclusion-report reason onto the reason a title cannot air.
func filterReason(r string) UnairableReason {
	switch r {
	case "over_ceiling":
		return UnairableOverCeiling
	case "unrated":
		return UnairableUnrated
	default:
		return UnairableOutOfScope
	}
}
