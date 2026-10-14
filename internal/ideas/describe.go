package ideas

import (
	"fmt"
	"strconv"
	"strings"
)

// Describe names an idea and pitches it in one sentence (#1720), from its facet and the seasonal
// calendar's label alone, so a card reads the same whether or not the LLM is on. holidayLabel is
// the calendar's display name ("Halloween") and is read for holiday ideas only.
//
// The name says what plays: Movies when every title is a movie, Shows when every title is a
// series, Channel for a mix. The pitch says what the channel would be. It carries no counts (the
// card shows those beside it) and never claims a schedule or a mood the data doesn't carry.
func Describe(idea Idea, holidayLabel string) (name, pitch string) {
	switch idea.Facet {
	case FacetHoliday:
		label := holidayLabel
		if label == "" {
			label = sentenceStart(idea.Value)
		}
		return label + " " + mixNoun(idea), fmt.Sprintf("Your library's %s titles, on one channel for the season.", label)
	case FacetDecade:
		year, err := strconv.Atoi(idea.Value)
		if err != nil {
			break
		}
		return decadeLabel(year) + " " + mixNoun(idea),
			fmt.Sprintf("Your library's %ds titles that no channel plays yet, on one channel.", year)
	case FacetGenre:
		return sentenceStart(idea.Value) + " " + mixNoun(idea),
			fmt.Sprintf("Every %s title in your library that no channel plays yet, on one channel.", strings.ToLower(idea.Value))
	}
	return idea.Value, "Titles from your library, on one channel."
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

// sentenceStart capitalises a library genre's first letter for a name; the rest keeps the
// library's spelling ("Sci-Fi & Fantasy").
func sentenceStart(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
