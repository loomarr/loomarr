package filler

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// OpenSearcher finds listings across a whole provider, outside every registered source (#2059
// decision 10). A short pass asks it only once the enabled sources have had their turn.
type OpenSearcher interface {
	OpenSearch(ctx context.Context, query string, limit int) ([]DiscoveredRef, error)
	// Gate is the pass's listing gate, read once per pass that searches.
	Gate(ctx context.Context) (ListingGate, error)
}

// OpenSearchSourceID is the source an open-search hit is acquired under. No row has it: an
// open-search hit belongs to no registered source, and "search:" is no source kind.
const OpenSearchSourceID = "search:archive"

// OpenSearchesPerPass bounds the call signs one pass searches the whole provider for, rotating
// hourly like the source searches.
const OpenSearchesPerPass = 2

// ListingGate decides from a listing's title alone whether an open-search hit may be downloaded
// (#2059 decision 11). It is deterministic, since a hit comes from nowhere vetted: no model is
// asked. A hit that passes still runs the whole pipeline, which decides its locality afresh.
type ListingGate struct {
	// Station returns the call sign of the nearby station the title names, held on the title's
	// date. False when the title names none, or a station outside every place's radius.
	Station func(title string) (callSign string, ok bool)
	// Languages are the languages the channels air (IntakeLanguages). Empty airs every language.
	Languages string
}

// Listing gate refusals, for the pass's log.
const (
	ListingUnsafe    = "unsafe"
	ListingLanguage  = "language"
	ListingUndated   = "undated"
	ListingOffEra    = "off_era"
	ListingNoStation = "no_station"
)

// Admit returns the call sign that admitted the listing, or why it was refused. The checks, all
// on the title:
//   - no word from the safety list;
//   - no language named, nor a script written, that no channel airs;
//   - a year, and every year it gives inside one of the eras;
//   - a nearby station's call sign, held in that year.
func (g ListingGate) Admit(ref DiscoveredRef, eras []EraRange) (callSign, refused string) {
	title := strings.TrimSpace(ref.Title)
	if unsafeListing(title) {
		return "", ListingUnsafe
	}
	if foreignListing(title, LanguageList(g.Languages)) {
		return "", ListingLanguage
	}
	years := listingYears.FindAllString(title, -1)
	if len(years) == 0 {
		return "", ListingUndated
	}
	for _, y := range years {
		year, _ := strconv.Atoi(y)
		if !slices.ContainsFunc(eras, func(r EraRange) bool {
			return (r.From == 0 || year >= r.From) && (r.To == 0 || year <= r.To)
		}) {
			return "", ListingOffEra
		}
	}
	if g.Station == nil {
		return "", ListingNoStation
	}
	sign, ok := g.Station(title)
	if !ok {
		return "", ListingNoStation
	}
	return sign, ""
}

var listingYears = regexp.MustCompile(`\b(19[3-9][0-9]|20[0-3][0-9])\b`)

// unsafeWords refuse a listing outright. Deliberately broad: refusing a harmless hit costs one
// clip of an open search, while airing an unsafe one cannot be taken back.
var unsafeWords = map[string]bool{
	"porn": true, "porno": true, "xxx": true, "nsfw": true, "nude": true, "nudity": true,
	"naked": true, "sex": true, "sexy": true, "erotic": true, "fetish": true, "hentai": true,
	"gore": true, "uncensored": true, "beheading": true, "execution": true, "murder": true,
	"suicide": true, "nazi": true, "hitler": true, "kkk": true, "adult": true, "explicit": true,
}

func unsafeListing(title string) bool {
	for _, word := range listingWords(title) {
		if unsafeWords[word] {
			return true
		}
	}
	return false
}

// listingLanguageWords are languages a title names in words ("Spanish commercial", "en español").
var listingLanguageWords = map[string]string{
	"english": "en", "spanish": "es", "espanol": "es", "español": "es", "french": "fr",
	"francais": "fr", "français": "fr", "german": "de", "deutsch": "de", "italian": "it",
	"italiano": "it", "portuguese": "pt", "português": "pt", "chinese": "zh", "mandarin": "zh",
	"cantonese": "zh", "japanese": "ja", "korean": "ko", "russian": "ru", "arabic": "ar",
	"hindi": "hi", "greek": "el", "hebrew": "he", "polish": "pl", "dutch": "nl",
}

// listingScripts are the scripts that each write one language (or, for Han, one of two).
var listingScripts = []struct {
	table *unicode.RangeTable
	langs []string
}{
	{unicode.Han, []string{"zh", "ja"}}, {unicode.Hiragana, []string{"ja"}},
	{unicode.Katakana, []string{"ja"}}, {unicode.Hangul, []string{"ko"}},
	{unicode.Cyrillic, []string{"ru", "uk", "bg", "sr"}}, {unicode.Arabic, []string{"ar", "fa", "ur"}},
	{unicode.Devanagari, []string{"hi"}}, {unicode.Greek, []string{"el"}},
	{unicode.Hebrew, []string{"he", "yi"}}, {unicode.Thai, []string{"th"}},
}

// foreignListing reports a title naming or written in a language no channel airs. An English word
// or a Latin-script title never refuses: most listings are English whatever they hold.
func foreignListing(title string, airs []string) bool {
	if len(airs) == 0 {
		return false
	}
	for _, word := range listingWords(title) {
		if lang, ok := listingLanguageWords[word]; ok && lang != "en" && !slices.Contains(airs, lang) {
			return true
		}
	}
	for _, r := range title {
		for _, script := range listingScripts {
			if unicode.Is(script.table, r) && !slices.ContainsFunc(script.langs, func(l string) bool {
				return slices.Contains(airs, l)
			}) {
				return true
			}
		}
	}
	return false
}

func listingWords(title string) []string {
	return strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
