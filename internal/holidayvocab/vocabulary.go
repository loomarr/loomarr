// Package holidayvocab owns Loomarr's immutable built-in holiday identities and the judgment of
// whether a title or episode is about one (programming-design §6).
package holidayvocab

import (
	"strings"

	"github.com/loomarr/loomarr/internal/textmatch"
)

type definition struct {
	id            string
	label         string
	intentAliases []string
	// nameAliases name the holiday itself. Any one, outside its look-alikes, is proof.
	nameAliases []string
	// lookAlikes are phrases that contain a name alias without naming the holiday.
	lookAlikes []string
	// moodAliases fit the holiday's mood. One counts only when the genres corroborate it.
	moodAliases []string
	// moodGenres corroborates a mood alias: any one inner set fully present in the genres.
	moodGenres [][]string
}

var familyFantasy = func() [][]string {
	var out [][]string
	for _, family := range []string{"family", "children", "kids", "animation"} {
		out = append(out, []string{family, "fantasy"})
	}
	return out
}()

var builtins = [...]definition{
	{
		id: "halloween", label: "Halloween",
		intentAliases: []string{"halloween"},
		nameAliases:   []string{"halloween"},
		moodAliases:   []string{"spooky", "haunted", "ghost", "ghosts", "witch", "witches", "horror"},
		moodGenres:    append([][]string{{"horror"}}, familyFantasy...),
	},
	{
		id: "thanksgiving", label: "Thanksgiving",
		intentAliases: []string{"thanksgiving", "turkey day"},
		nameAliases:   []string{"thanksgiving", "turkey day"},
	},
	{
		id: "christmas", label: "Christmas",
		intentAliases: []string{"christmas", "xmas", "santa", "noel", "yuletide"},
		nameAliases:   []string{"christmas", "xmas", "santa", "noel", "yuletide"},
		// A character's pet name and a city are not Christmas proof.
		lookAlikes: []string{"santa's little helper", "santa clarita", "santa monica", "santa barbara", "santa fe", "santa cruz",
			"santa ana", "santa rosa", "santa maria", "santa catalina"},
	},
	{
		id: "newyear", label: "New Year",
		intentAliases: []string{"new year's", "new years", "new year", "nye", "hogmanay"},
		nameAliases:   []string{"new year's", "new years", "new year", "nye", "hogmanay"},
		lookAlikes:    []string{"bill nye"},
	},
	{
		id: "valentines", label: "Valentine's Day",
		intentAliases: []string{"valentine's", "valentines", "valentine"},
		nameAliases:   []string{"valentine's", "valentines", "valentine"},
	},
}

// Definition is one immutable built-in identity exposed without its private alias slices.
type Definition struct {
	ID    string
	Label string
}

// Definitions returns the closed built-in vocabulary in stable display order.
func Definitions() []Definition {
	out := make([]Definition, len(builtins))
	for i, builtin := range builtins {
		out[i] = Definition{ID: builtin.id, Label: builtin.label}
	}
	return out
}

// MatchIntent returns holiday ids named by whole-phrase affirmative intent text.
func MatchIntent(text string) []string {
	var out []string
	for _, builtin := range builtins {
		for _, alias := range builtin.intentAliases {
			if textmatch.ContainsPhrase(text, alias) {
				out = append(out, builtin.id)
				break
			}
		}
	}
	return out
}

// Verdict is the closed answer to "is this about the holiday?".
type Verdict string

const (
	// VerdictAbout: the holiday is the subject or setting. Only this benches or boosts.
	VerdictAbout Verdict = "about"
	// VerdictAdjacent: it fits the holiday's mood without being about it.
	VerdictAdjacent Verdict = "adjacent"
	VerdictNot      Verdict = "not"
)

// Reason says which evidence decided a verdict, for clients to word.
type Reason string

const (
	ReasonNameInTitle   Reason = "name_in_title"   // "Halloween in the title"
	ReasonNameInText    Reason = "name_in_text"    // "mentions Halloween"
	ReasonHolidayTag    Reason = "holiday_keyword" // "Halloween keyword"
	ReasonMoodWithGenre Reason = "mood_and_genre"  // "a ghost story and a horror film"
	ReasonNone          Reason = ""
)

// Evidence is what is known about one title or episode. Text holds free prose (an overview);
// Keywords holds curated tags (TMDB keywords, media-server tags).
type Evidence struct {
	Title    string
	Text     []string
	Keywords []string
	Genres   []string
}

// Judgment is one floor verdict with its reason and the alias that decided it.
type Judgment struct {
	Verdict Verdict
	Reason  Reason
	Cue     string
}

// Fits reports whether the judgment admits a title to the holiday's own programming.
func (j Judgment) Fits() bool { return j.Verdict == VerdictAbout || j.Verdict == VerdictAdjacent }

// Judge is the deterministic floor (§6): a name alias anywhere is about the holiday; a mood alias
// in the title or keywords is adjacent only when the genres corroborate it; nothing else counts.
// An unknown id is never about anything.
func Judge(id string, ev Evidence) Judgment {
	d, ok := lookup(id)
	if !ok {
		return Judgment{Verdict: VerdictNot}
	}
	if cue := d.nameIn(ev.Title); cue != "" {
		return Judgment{Verdict: VerdictAbout, Reason: ReasonNameInTitle, Cue: cue}
	}
	for _, kw := range ev.Keywords {
		if cue := d.nameIn(kw); cue != "" {
			return Judgment{Verdict: VerdictAbout, Reason: ReasonHolidayTag, Cue: cue}
		}
	}
	for _, text := range ev.Text {
		if cue := d.nameIn(text); cue != "" {
			return Judgment{Verdict: VerdictAbout, Reason: ReasonNameInText, Cue: cue}
		}
	}
	if !d.genresCorroborate(ev.Genres) {
		return Judgment{Verdict: VerdictNot}
	}
	for _, field := range append([]string{ev.Title}, ev.Keywords...) {
		for _, alias := range d.moodAliases {
			if textmatch.ContainsPhrase(field, alias) {
				return Judgment{Verdict: VerdictAdjacent, Reason: ReasonMoodWithGenre, Cue: alias}
			}
		}
	}
	return Judgment{Verdict: VerdictNot}
}

func lookup(id string) (definition, bool) {
	for _, builtin := range builtins {
		if builtin.id == id {
			return builtin, true
		}
	}
	return definition{}, false
}

func (d definition) nameIn(text string) string {
	for _, alias := range d.nameAliases {
		if textmatch.ContainsPhraseOutside(text, alias, d.lookAlikes...) {
			return alias
		}
	}
	return ""
}

// genresCorroborate matches genre words, so a TV library's "Sci-Fi & Fantasy" carries "fantasy".
func (d definition) genresCorroborate(genres []string) bool {
	joined := strings.Join(genres, " | ")
	for _, set := range d.moodGenres {
		all := true
		for _, g := range set {
			all = all && textmatch.ContainsPhrase(joined, g)
		}
		if all {
			return true
		}
	}
	return false
}
