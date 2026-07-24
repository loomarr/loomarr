package schedule

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mantonx/loomarr/internal/provision"
)

// franchiseTag is a movie entry's franchise-group assignment (§5): the shared PartGroup id
// and the film's 1-based release-order index within it.
type franchiseTag struct {
	group string
	index int
}

// assignFranchiseGroups groups MOVIE entries that share a TMDB collection (CollectionID > 0)
// into a franchise, so a franchise's films play together in release-year order as an atomic
// block (§5 — the movie analogue of the multi-part episode floor). Returns a map from each
// grouped movie's Key to its (group, index); an ungrouped movie / any series is absent from
// the map (no franchise constraint). Only a collection with ≥2 present films is a group — a
// lone film of a franchise isn't "kept together" with anything. Deterministic: films are
// ordered by Year (ties broken by Key) so the same lineup always yields the same order.
func assignFranchiseGroups(entries []LineupEntry) map[provision.Key]franchiseTag {
	// Collect the movie entries per collection id.
	byCollection := map[int][]LineupEntry{}
	for _, e := range entries {
		if e.CollectionID > 0 && !e.Key.IsSeries() {
			byCollection[e.CollectionID] = append(byCollection[e.CollectionID], e)
		}
	}
	tags := map[provision.Key]franchiseTag{}
	for cid, films := range byCollection {
		if len(films) < 2 {
			continue // a single film of a franchise has nothing to be kept together with
		}
		// Release order: Year ascending, Key as a stable tiebreaker (same-year films / a
		// missing year still get a deterministic order).
		sort.Slice(films, func(i, j int) bool {
			if films[i].Year != films[j].Year {
				return films[i].Year < films[j].Year
			}
			return films[i].Key < films[j].Key
		})
		group := fmt.Sprintf("collection:%d", cid)
		for idx, f := range films {
			tags[f.Key] = franchiseTag{group: group, index: idx + 1}
		}
	}
	return tags
}

// Multi-part episode detection (§5 multi-part adjacency floor): a two-parter must air
// as an atomic, in-order block. Detection is deterministic and runs at episode-resolution
// time, from EITHER signal:
//
//   - IndexNumberEnd: a SINGLE file spanning e.g. episodes 25–26 (EpisodeEnd > Episode).
//     It's already one playable item, so it needs no grouping with a sibling — but we tag
//     it so the window/ordering treat its (longer) runtime as the atomic unit it is.
//   - Title suffix + consecutive: two SEPARATE files whose titles share a base name and
//     carry a "(1)/(2)" or "Part N" marker, on consecutive episode numbers of one season.
//     This is the common case (Star Trek two-parters). Both parts get one shared PartGroup
//     and their PartIndex from the marker.
//
// The grouping is applied in place: parts of one story get a shared, deterministic
// PartGroup and a 1-based PartIndex; standalone programs are left untouched (PartGroup "").
// A group id is derived from the series key + season + base title so it is stable across
// reconciles (no clock, no counter) — the §19 reproducibility requirement.

// partSuffix matches a trailing part marker: "(1)", "(2)", "Part 1", "Part II", "Pt. 2".
// Captured group 1 is an arabic number; group 2 is a roman numeral (I/II/III).
var partSuffix = regexp.MustCompile(`(?i)\s*(?:\((\d+)\)|part\s+(\d+|[ivx]+)|pt\.?\s+(\d+))\s*$`)

// romanToInt maps the small roman numerals a part marker realistically uses.
var romanToInt = map[string]int{"i": 1, "ii": 2, "iii": 3, "iv": 4, "v": 5, "vi": 6}

// partMarker returns (baseTitle, partNumber, true) when title ends in a part marker,
// else ("", 0, false). The base is the title with the marker stripped, so "All Good
// Things... (2)" → ("All Good Things...", 2).
func partMarker(title string) (string, int, bool) {
	loc := partSuffix.FindStringSubmatchIndex(title)
	if loc == nil {
		return "", 0, false
	}
	// group() returns capture group g's text, or "" if it didn't participate.
	group := func(g int) string {
		lo, hi := loc[2*g], loc[2*g+1]
		if lo < 0 {
			return ""
		}
		return title[lo:hi]
	}
	var n int
	switch {
	case group(1) != "": // "(N)"
		n = atoiSafe(group(1))
	case group(2) != "": // "Part N" — arabic or roman
		if v := atoiSafe(group(2)); v > 0 {
			n = v
		} else {
			n = romanToInt[strings.ToLower(group(2))]
		}
	case group(3) != "": // "Pt. N"
		n = atoiSafe(group(3))
	}
	if n <= 0 {
		return "", 0, false
	}
	base := strings.TrimSpace(title[:loc[0]]) // loc[0] = start of the whole match
	return base, n, true
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// collapseGroups makes multi-part groups atomic for ordering + windowing (§5). Each run
// of same-PartGroup program slots is replaced by ONE representative super-slot whose
// DurationMs is the group's total runtime and whose Key/PartGroup identify it; the group's
// real slots (in PartIndex order) are stashed in `expand` keyed by PartGroup. Standalone
// slots (PartGroup "") pass through untouched. The ordering engine then permutes super-
// slots without seeing — or being able to split — a group's internals, and truncateToWindow
// counts a group's full runtime as one indivisible unit. Order-preserving: a group collapses
// at the position of its first part.
//
// A group's parts are assumed contiguous in `slots` here because resolveEntry emits them in
// order and nothing between resolution and ordering reorders within a series' expansion.
func collapseGroups(slots []Slot) (collapsed []Slot, expand map[string][]Slot) {
	expand = map[string][]Slot{}
	collapsed = make([]Slot, 0, len(slots))
	i := 0
	for i < len(slots) {
		g := slots[i].PartGroup
		if g == "" || !slots[i].IsProgram() {
			collapsed = append(collapsed, slots[i])
			i++
			continue
		}
		// Gather the contiguous run of this group.
		j := i
		var total int64
		run := []Slot{}
		for j < len(slots) && slots[j].PartGroup == g {
			total += slots[j].DurationMs
			run = append(run, slots[j])
			j++
		}
		sortByPartIndex(run)
		expand[g] = run
		// The super-slot carries the group's identity + total runtime for windowing.
		rep := run[0]
		rep.DurationMs = total
		collapsed = append(collapsed, rep)
		i = j
	}
	return collapsed, expand
}

// expandGroups reverses collapseGroups: each super-slot is replaced by its group's real
// parts (in PartIndex order), so the final deck has every two-parter adjacent + in-order.
func expandGroups(collapsed []Slot, expand map[string][]Slot) []Slot {
	if len(expand) == 0 {
		return collapsed
	}
	out := make([]Slot, 0, len(collapsed))
	for _, s := range collapsed {
		if parts, ok := expand[s.PartGroup]; ok && s.PartGroup != "" {
			out = append(out, parts...)
			continue
		}
		out = append(out, s)
	}
	return out
}

// sortByPartIndex orders a group's parts by their 1-based PartIndex (stable; a group is
// tiny). Guards the invariant that Part 1 airs before Part 2 regardless of resolution order.
func sortByPartIndex(run []Slot) {
	for a := 1; a < len(run); a++ {
		for b := a; b > 0 && run[b].PartIndex < run[b-1].PartIndex; b-- {
			run[b], run[b-1] = run[b-1], run[b]
		}
	}
}

// assignPartGroups tags multi-part episodes of ONE series (episodes in season/episode
// order, as ListEpisodes returns them) with a shared PartGroup + 1-based PartIndex, in
// place. seriesKey scopes the group id so two series can't collide. Standalone episodes
// are left with PartGroup "". Deterministic: same input → same tags, no clock/counter.
func assignPartGroups(seriesKey string, progs []ResolvedProgram) {
	for i := range progs {
		// Signal (a): a single-file span (IndexNumberEnd > IndexNumber). One item, but we
		// mark it so its runtime is treated as the atomic unit; PartIndex 1 (it's whole).
		if progs[i].EpisodeEnd > progs[i].Episode && progs[i].Episode > 0 {
			progs[i].PartGroup = fmt.Sprintf("%s|s%d|span%d-%d", seriesKey, progs[i].Season, progs[i].Episode, progs[i].EpisodeEnd)
			progs[i].PartIndex = 1
		}
	}

	// Signal (b): consecutive episodes sharing a title base + part markers. Walk runs of
	// same-season episodes whose base titles match and whose episode numbers are contiguous.
	i := 0
	for i < len(progs) {
		base, part, ok := partMarker(progs[i].Title)
		if !ok || progs[i].PartGroup != "" {
			i++
			continue
		}
		// Collect the run: subsequent episodes with the SAME base, same season, consecutive
		// episode numbers, ascending part numbers.
		j := i + 1
		run := []int{i}
		lastEp, lastPart := progs[i].Episode, part
		for j < len(progs) {
			b2, p2, ok2 := partMarker(progs[j].Title)
			if !ok2 || b2 != base || progs[j].Season != progs[i].Season ||
				progs[j].Episode != lastEp+1 || p2 != lastPart+1 {
				break
			}
			run = append(run, j)
			lastEp, lastPart = progs[j].Episode, p2
			j++
		}
		if len(run) >= 2 {
			group := fmt.Sprintf("%s|s%d|%s", seriesKey, progs[i].Season, strings.ToLower(base))
			for idx, pos := range run {
				progs[pos].PartGroup = group
				progs[pos].PartIndex = idx + 1
			}
			i = j
			continue
		}
		i++
	}
}
