package schedule

import (
	"cmp"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/textmatch"
)

// EpisodeSelection is the approved editorial policy for one series entry.
// It shapes only the already-safe episode pool; empty means the complete run.
type EpisodeSelection struct {
	Mode     EpisodeSelectionMode `json:"mode,omitempty"`
	Holidays []string             `json:"holidays,omitempty"`
}

type EpisodeSelectionMode string

const (
	EpisodeComplete   EpisodeSelectionMode = "complete"
	EpisodeHighlights EpisodeSelectionMode = "highlights"
	EpisodeHoliday    EpisodeSelectionMode = "holiday"

	minHighlightUnits = 8
)

func selectEpisodes(episodes []ResolvedProgram, policy EpisodeSelection) []ResolvedProgram {
	switch policy.Mode {
	case EpisodeHighlights:
		return selectHighlights(episodes)
	case EpisodeHoliday:
		return selectHolidayEpisodes(episodes, policy.Holidays)
	default:
		return episodes
	}
}

func selectHighlights(episodes []ResolvedProgram) []ResolvedProgram {
	type selectionUnit struct {
		episodes []int
		rating   float64
		rated    bool
	}
	units := make([]selectionUnit, 0, len(episodes))
	groupUnit := make(map[string]int)
	for i, episode := range episodes {
		unitIndex, grouped := groupUnit[episode.PartGroup]
		if episode.PartGroup == "" || !grouped {
			unitIndex = len(units)
			units = append(units, selectionUnit{rated: true})
			if episode.PartGroup != "" {
				groupUnit[episode.PartGroup] = unitIndex
			}
		}
		unit := &units[unitIndex]
		unit.episodes = append(unit.episodes, i)
		if validCommunityRating(episode.CommunityRating) {
			unit.rating += episode.CommunityRating
		} else {
			unit.rated = false
		}
	}
	if len(units) < minHighlightUnits {
		return episodes
	}
	var rated []int
	for i := range units {
		if units[i].rated {
			units[i].rating /= float64(len(units[i].episodes))
			rated = append(rated, i)
		}
	}
	// A sparse rated minority cannot define "best" for the whole eligible run.
	// Require ratings on at least three quarters of the selection units.
	if len(rated)*4 < len(units)*3 {
		return episodes
	}
	target := (len(rated) + 3) / 4
	if target < 4 {
		target = 4
	}
	if target > 48 {
		target = 48
	}
	if target >= len(rated) || target > 48 {
		return episodes
	}
	slices.SortFunc(rated, func(a, b int) int {
		if byRating := cmp.Compare(units[b].rating, units[a].rating); byRating != 0 {
			return byRating
		}
		return cmp.Compare(a, b)
	})
	cutoff := units[rated[target-1]].rating
	for target < len(rated) && units[rated[target]].rating == cutoff {
		target++
	}
	if target >= len(rated) || target > 48 {
		return episodes
	}
	selected := make(map[int]bool, target)
	for _, unitIndex := range rated[:target] {
		for _, episodeIndex := range units[unitIndex].episodes {
			selected[episodeIndex] = true
		}
	}
	out := make([]ResolvedProgram, 0, len(selected))
	for i, episode := range episodes {
		if selected[i] {
			out = append(out, episode)
		}
	}
	return out
}

func validCommunityRating(rating float64) bool {
	// Comparisons also reject NaN; +Inf is above the closed upper bound.
	return rating > 0 && rating <= 10
}

func selectHolidayEpisodes(episodes []ResolvedProgram, holidayIDs []string) []ResolvedProgram {
	selectedHolidays := make(map[string]bool, len(holidayIDs))
	for _, id := range holidayIDs {
		selectedHolidays[strings.ToLower(id)] = true
	}
	groups := make(map[string]bool)
	selected := make(map[int]bool)
	for i, episode := range episodes {
		if episodeMatchesHoliday(episode, selectedHolidays) {
			selected[i] = true
			if episode.PartGroup != "" {
				groups[episode.PartGroup] = true
			}
		}
	}
	if len(selected) == 0 {
		return episodes
	}
	out := make([]ResolvedProgram, 0, len(selected))
	for i, episode := range episodes {
		if selected[i] || (episode.PartGroup != "" && groups[episode.PartGroup]) {
			out = append(out, episode)
		}
	}
	return out
}

func episodeMatchesHoliday(episode ResolvedProgram, selected map[string]bool) bool {
	haystack := strings.Join(append([]string{episode.Title, episode.Overview}, episode.Tags...), " ")
	for _, holiday := range builtinCalendar {
		if len(selected) > 0 && !selected[holiday.id] {
			continue
		}
		for _, keyword := range holidayvocab.EvidenceAliases(holiday.id) {
			if textmatch.ContainsPhrase(haystack, keyword) {
				return true
			}
		}
	}
	return false
}
