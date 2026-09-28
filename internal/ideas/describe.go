package ideas

import (
	"fmt"
	"strconv"
	"strings"
)

// Describe names an idea and pitches it in one sentence (#1720), from its facet, the seasonal
// calendar's label and its counts alone, so a card reads the same whether or not the LLM is on.
// holidayLabel is the calendar's display name ("Halloween") and is read for holiday ideas only.
//
// The name says what plays: Movies when every title is a movie, Shows when every title is a
// series, Channel for a mix. The pitch says where the titles come from and how many there are;
// it never claims a schedule or a mood the data doesn't carry.
func Describe(idea Idea, holidayLabel string) (name, pitch string) {
	counts := countPhrase(idea.Movies, idea.Series)
	switch idea.Facet {
	case FacetHoliday:
		label := holidayLabel
		if label == "" {
			label = sentenceStart(idea.Value)
		}
		return label + " " + mixNoun(idea), fmt.Sprintf("Titles about %s from your library, for the season: %s.", label, counts)
	case FacetDecade:
		year, err := strconv.Atoi(idea.Value)
		if err != nil {
			break
		}
		return decadeLabel(year) + " " + mixNoun(idea),
			fmt.Sprintf("Titles from the %ds in your library that no channel plays yet: %s.", year, counts)
	case FacetGenre:
		return sentenceStart(idea.Value) + " " + mixNoun(idea),
			fmt.Sprintf("%s from your library that no channel plays yet: %s.", sentenceStart(idea.Value), counts)
	}
	return idea.Value, fmt.Sprintf("Titles from your library: %s.", counts)
}

func mixNoun(idea Idea) string {
	switch {
	case idea.Series == 0:
		return "Movies"
	case idea.Movies == 0:
		return "Shows"
	default:
		return "Channel"
	}
}

// decadeLabel is how people say a decade: "90s" in the last century, "2010s" in this one.
func decadeLabel(year int) string {
	if year >= 1900 && year < 2000 {
		return fmt.Sprintf("%02ds", year%100)
	}
	return fmt.Sprintf("%ds", year)
}

func countPhrase(movies, series int) string {
	var parts []string
	if movies > 0 {
		parts = append(parts, plural(movies, "movie", "movies"))
	}
	if series > 0 {
		parts = append(parts, plural(series, "show", "shows"))
	}
	return strings.Join(parts, " and ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// sentenceStart capitalises a library genre's first letter so it can open a sentence; the rest
// keeps the library's spelling ("Sci-Fi & Fantasy").
func sentenceStart(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
