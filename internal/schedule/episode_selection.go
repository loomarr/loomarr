package schedule

import (
	"cmp"
	"slices"
	"strings"
)

// EpisodeSelection is the approved editorial policy for one series entry.
// It shapes only the already-safe episode pool; empty means the complete run.
type EpisodeSelection struct {
	Mode     EpisodeSelectionMode `json:"mode,omitempty"`
	Holidays []string             `json:"holidays,omitempty"`
}

type EpisodeSelectionMode string

const (
	EpisodeHighlights EpisodeSelectionMode = "highlights"
	EpisodeHoliday    EpisodeSelectionMode = "holiday"
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
	var rated []int
	for i, episode := range episodes {
		if episode.CommunityRating > 0 {
			rated = append(rated, i)
		}
	}
	// Fewer than four independent ratings is not enough evidence to call a
	// subset "highlights". Preserve the safe full run instead of guessing.
	if len(rated) < 4 {
		return episodes
	}
	target := (len(episodes) + 3) / 4
	if target < 4 {
		target = 4
	}
	if target > 48 {
		target = 48
	}
	if target >= len(episodes) || target > len(rated) {
		return episodes
	}
	slices.SortFunc(rated, func(a, b int) int {
		if byRating := cmp.Compare(episodes[b].CommunityRating, episodes[a].CommunityRating); byRating != 0 {
			return byRating
		}
		return cmp.Compare(a, b)
	})
	selected := make(map[int]bool, target)
	groups := make(map[string]bool)
	for _, index := range rated[:target] {
		selected[index] = true
		if group := episodes[index].PartGroup; group != "" {
			groups[group] = true
		}
	}
	out := make([]ResolvedProgram, 0, target)
	for i, episode := range episodes {
		if selected[i] || (episode.PartGroup != "" && groups[episode.PartGroup]) {
			out = append(out, episode)
		}
	}
	return out
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
	haystack := strings.ToLower(strings.Join(append([]string{episode.Title, episode.Overview}, episode.Tags...), " "))
	for _, holiday := range builtinCalendar {
		if len(selected) > 0 && !selected[holiday.id] {
			continue
		}
		for _, keyword := range holiday.keywords {
			if strings.Contains(haystack, keyword) {
				return true
			}
		}
	}
	return false
}
