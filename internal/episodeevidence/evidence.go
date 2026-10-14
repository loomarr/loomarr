// Package episodeevidence owns playable structure and bounded editorial facts used for episode curation.
package episodeevidence

import (
	"errors"
	"math"
	"strings"
)

const (
	MaxOverviewRunes = 2048
	MaxTags          = 16
	MaxTagRunes      = 64
)

// Evidence is sanitized episode quality and thematic metadata. Zero values are
// explicit unavailable evidence.
type Evidence struct {
	CommunityRating float64
	Overview        string
	Tags            []string
}

// ValidatePlayable rejects episode facts that cannot safely become a scheduled program.
func ValidatePlayable(libraryItemID string, durationMs int64, season, episode, episodeEnd int) error {
	switch {
	case strings.TrimSpace(libraryItemID) == "":
		return errors.New("episode has no library item id")
	case durationMs <= 0:
		return errors.New("episode runtime must be positive")
	case season < 0:
		return errors.New("episode season must be non-negative")
	case episode <= 0:
		return errors.New("episode number must be positive")
	case episodeEnd < 0:
		return errors.New("episode end must be non-negative")
	case episodeEnd > 0 && episodeEnd < episode:
		return errors.New("episode end precedes episode number")
	default:
		return nil
	}
}

// Sanitize validates and bounds provider- or cache-controlled editorial facts.
func Sanitize(rating float64, overview string, tags []string) Evidence {
	if math.IsNaN(rating) || math.IsInf(rating, 0) || rating <= 0 || rating > 10 {
		rating = 0
	}
	return Evidence{
		CommunityRating: rating,
		Overview:        boundedText(overview, MaxOverviewRunes),
		Tags:            boundedTags(tags),
	}
}

func boundedText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes])
	}
	return strings.TrimSpace(text)
}

func boundedTags(tags []string) []string {
	out := make([]string, 0, min(len(tags), MaxTags))
	seen := make(map[string]bool, cap(out))
	for _, tag := range tags {
		tag = boundedText(tag, MaxTagRunes)
		folded := strings.ToLower(tag)
		if tag == "" || seen[folded] {
			continue
		}
		seen[folded] = true
		out = append(out, tag)
		if len(out) == MaxTags {
			break
		}
	}
	return out
}
