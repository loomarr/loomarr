// Package holidayvocab owns Loomarr's holiday definitions — what each holiday is called, the words
// that show a title is about it, and when each country keeps it — and the judgment of whether a
// title or episode is about one (programming-design §6, #2030). Definitions are data: Seed is the
// shipped set, and a Vocabulary is one validated snapshot of them that callers are handed.
package holidayvocab

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/textmatch"
)

// Holiday is one holiday definition.
type Holiday struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Words Words  `json:"words"`
	// Kept is when each country keeps the holiday. Empty = undated: no season anywhere yet.
	Kept []Keeping `json:"kept,omitempty"`
	// AirWindow is the broadcast window of the year's holiday: a TV episode that first aired
	// inside it was scheduled for the holiday. Nil = no air-date evidence.
	AirWindow *Rule `json:"airWindow,omitempty"`
	// Members makes a group holiday ("happy holidays"): in season whenever any member is. A group
	// has no dates or words of its own and is never on a programming calendar.
	Members []string `json:"members,omitempty"`
	// NameNeedsCorroboration makes a name alone boost but not bench (Judgment.Benches): for a
	// holiday whose name is also an ordinary title.
	NameNeedsCorroboration bool `json:"nameNeedsCorroboration,omitempty"`
}

// Words are the phrases that tie a title to a holiday.
type Words struct {
	// Intents name the holiday in a channel request ("a Halloween channel").
	Intents []string `json:"intents,omitempty"`
	// Names name the holiday itself. Any one, outside the look-alikes, is proof.
	Names []string `json:"names,omitempty"`
	// LookAlikes contain a name without naming the holiday ("santa monica").
	LookAlikes []string `json:"lookAlikes,omitempty"`
	// Moods fit the holiday's mood. One counts only when the genres or the air date corroborate it.
	Moods []string `json:"moods,omitempty"`
	// MoodGenres corroborate a mood: any one inner set fully present in the genres.
	MoodGenres [][]string `json:"moodGenres,omitempty"`
}

func (w Words) none() bool {
	return len(w.Intents)+len(w.Names)+len(w.LookAlikes)+len(w.Moods)+len(w.MoodGenres) == 0
}

// Vocabulary is one validated snapshot of the holiday definitions, in display order.
type Vocabulary struct{ holidays []Holiday }

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// New validates holidays into a Vocabulary.
func New(holidays []Holiday) (Vocabulary, error) {
	ids := map[string]bool{}
	for _, h := range holidays {
		if !slugPattern.MatchString(h.ID) {
			return Vocabulary{}, fmt.Errorf("holiday id %q is not a slug", h.ID)
		}
		if ids[h.ID] {
			return Vocabulary{}, fmt.Errorf("holiday %q is defined twice", h.ID)
		}
		ids[h.ID] = true
		if err := h.validate(); err != nil {
			return Vocabulary{}, fmt.Errorf("holiday %q: %w", h.ID, err)
		}
	}
	// A member is a holiday with a date of its own, never a group: groups of groups could cycle.
	dated := map[string]bool{}
	for _, h := range holidays {
		dated[h.ID] = len(h.Members) == 0
	}
	for _, h := range holidays {
		for _, m := range h.Members {
			if !dated[m] {
				return Vocabulary{}, fmt.Errorf("holiday %q: member %q is not a holiday with its own dates", h.ID, m)
			}
		}
	}
	return Vocabulary{holidays: slices.Clone(holidays)}, nil
}

func (h Holiday) validate() error {
	if strings.TrimSpace(h.Label) == "" {
		return fmt.Errorf("no label")
	}
	if len(h.Members) > 0 && (len(h.Kept) > 0 || h.AirWindow != nil || !h.Words.none()) {
		return fmt.Errorf("a group holiday has no dates or words of its own")
	}
	seen := map[string]bool{}
	for _, k := range h.Kept {
		if len(k.Countries) == 0 {
			return fmt.Errorf("a keeping names no country")
		}
		for _, c := range k.Countries {
			if err := validCountry(c); err != nil {
				return err
			}
			if seen[c] {
				return fmt.Errorf("%s keeps it twice", c)
			}
			seen[c] = true
		}
		if err := k.Validate(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(k.Countries, ","), err)
		}
	}
	if h.AirWindow != nil {
		if err := h.AirWindow.Validate(); err != nil {
			return fmt.Errorf("air window: %w", err)
		}
	}
	return nil
}

// Default is the shipped definitions (Seed). It is the vocabulary of a process with no stored
// holidays: tests, and the moment before the store's snapshot loads.
func Default() Vocabulary { return defaultVocabulary() }

var defaultVocabulary = sync.OnceValue(func() Vocabulary {
	v, err := New(Seed())
	if err != nil {
		panic(fmt.Sprintf("holidayvocab: the shipped seed is invalid: %v", err))
	}
	return v
})

// Source supplies the current holiday definitions: the store's snapshot in a running server, read
// fresh on every call so an edit applies without a restart. A nil Source is the shipped ones.
type Source func() Vocabulary

// Current is the source's definitions now.
func (s Source) Current() Vocabulary {
	if s == nil {
		return Default()
	}
	return s()
}

// Holidays returns every definition, groups and undated ones included.
func (v Vocabulary) Holidays() []Holiday { return slices.Clone(v.holidays) }

// Definition is one holiday's identity, without its words and dates.
type Definition struct {
	ID    string
	Label string
}

// Definitions returns the holidays programming can name, in display order: every one but groups.
func (v Vocabulary) Definitions() []Definition {
	var out []Definition
	for _, h := range v.holidays {
		if len(h.Members) == 0 {
			out = append(out, Definition{ID: h.ID, Label: h.Label})
		}
	}
	return out
}

// InSeason reports whether holiday id is in season at t's date in country: its own window
// contains the date, or, for a group, any member's does. An undated holiday is never in season.
func (v Vocabulary) InSeason(id, country string, t time.Time) bool {
	h, ok := v.lookup(id)
	if !ok {
		return false
	}
	if len(h.Members) > 0 {
		return slices.ContainsFunc(h.Members, func(m string) bool { return v.InSeason(m, country, t) })
	}
	k, ok := h.keptIn(v.keeper(country))
	return ok && spans(k.Rule, t)
}

// spans reports whether r's season contains t's calendar date: last year's season (one that wraps
// into January), this year's, or next year's (one that opens in the December before its date).
func spans(r Rule, t time.Time) bool {
	day := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
	for _, year := range []int{day.Year() - 1, day.Year(), day.Year() + 1} {
		if start, end := r.Span(year); !day.Before(start) && !day.After(end) {
			return true
		}
	}
	return false
}

// MatchIntent returns holiday ids named by whole-phrase affirmative intent text, outside the
// holiday's look-alikes ("Easter Island" names no holiday).
func (v Vocabulary) MatchIntent(text string) []string {
	var out []string
	for _, h := range v.holidays {
		for _, alias := range h.Words.Intents {
			if textmatch.ContainsPhraseOutside(text, alias, h.Words.LookAlikes...) {
				out = append(out, h.ID)
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
	ReasonNameInTitle   Reason = "name_in_title"    // "Halloween in the title"
	ReasonNameInText    Reason = "name_in_text"     // "mentions Halloween"
	ReasonHolidayTag    Reason = "holiday_keyword"  // "Halloween keyword"
	ReasonMoodWithGenre Reason = "mood_and_genre"   // "a ghost story and a horror film"
	ReasonMoodAiredNear Reason = "mood_and_airdate" // "a spooky episode first shown at Halloween"
	ReasonMoodInEpisode Reason = "mood_in_episode"  // "a spooky episode title"
	ReasonNone          Reason = ""
)

// Evidence is what is known about one title or episode. Text holds free prose (an overview);
// Keywords holds curated tags (TMDB keywords, media-server tags).
type Evidence struct {
	Title    string
	Text     []string
	Keywords []string
	Genres   []string
	// Episode marks a TV episode: its title describes that one episode, unlike a film title.
	Episode bool
	// AirDate is an episode's original air date (zero = unknown).
	AirDate time.Time
}

// Judgment is one floor verdict with its reason and the alias that decided it.
type Judgment struct {
	Verdict Verdict
	Reason  Reason
	Cue     string
	// NameOnly marks an about verdict that rests on a name alias alone, for a holiday whose name
	// is also an ordinary title. It boosts in season but does not bench.
	NameOnly bool
}

// Fits reports whether the judgment admits a title to the holiday's own programming.
func (j Judgment) Fits() bool { return j.Verdict == VerdictAbout || j.Verdict == VerdictAdjacent }

// Benches reports whether the judgment benches a title out of season (#2030 decision 26).
func (j Judgment) Benches() bool { return j.Verdict == VerdictAbout && !j.NameOnly }

// Judge is the deterministic floor (§6): a name alias anywhere is about the holiday. An episode
// that first aired in the holiday's broadcast window with a mood alias in its title or overview is
// about it; a mood alias in an episode's title alone is adjacent. Otherwise a mood alias in the
// title or keywords is adjacent only when the genres corroborate it; nothing else counts. An air
// date alone is never evidence: plenty of ordinary episodes air in late October.
// An unknown id is never about anything.
func (v Vocabulary) Judge(id string, ev Evidence) Judgment {
	h, ok := v.lookup(id)
	if !ok {
		return Judgment{Verdict: VerdictNot}
	}
	if cue := h.nameIn(ev.Title); cue != "" {
		return h.named(ReasonNameInTitle, cue, ev)
	}
	for _, kw := range ev.Keywords {
		if cue := h.nameIn(kw); cue != "" {
			return h.named(ReasonHolidayTag, cue, ev)
		}
	}
	for _, text := range ev.Text {
		if cue := h.nameIn(text); cue != "" {
			return h.named(ReasonNameInText, cue, ev)
		}
	}
	if ev.Episode {
		if h.airedInWindow(ev.AirDate) {
			if cue := h.moodIn(append([]string{ev.Title}, ev.Text...)...); cue != "" {
				return Judgment{Verdict: VerdictAbout, Reason: ReasonMoodAiredNear, Cue: cue}
			}
		}
		if cue := h.moodIn(ev.Title); cue != "" {
			return Judgment{Verdict: VerdictAdjacent, Reason: ReasonMoodInEpisode, Cue: cue}
		}
	}
	if !h.genresCorroborate(ev.Genres) {
		return Judgment{Verdict: VerdictNot}
	}
	if cue := h.moodIn(append([]string{ev.Title}, ev.Keywords...)...); cue != "" {
		return Judgment{Verdict: VerdictAdjacent, Reason: ReasonMoodWithGenre, Cue: cue}
	}
	return Judgment{Verdict: VerdictNot}
}

// named is the about verdict a name alias decided. For a holiday whose name is also an ordinary
// title, it rests on the name alone unless an episode first aired in the holiday's broadcast
// window or the genres corroborate it.
func (h Holiday) named(reason Reason, cue string, ev Evidence) Judgment {
	j := Judgment{Verdict: VerdictAbout, Reason: reason, Cue: cue}
	if h.NameNeedsCorroboration {
		corroborated := (ev.Episode && h.airedInWindow(ev.AirDate)) || (len(h.Words.MoodGenres) > 0 && h.genresCorroborate(ev.Genres))
		j.NameOnly = !corroborated
	}
	return j
}

func (h Holiday) moodIn(fields ...string) string {
	for _, field := range fields {
		for _, alias := range h.Words.Moods {
			if textmatch.ContainsPhrase(field, alias) {
				return alias
			}
		}
	}
	return ""
}

func (h Holiday) airedInWindow(aired time.Time) bool {
	return !aired.IsZero() && h.AirWindow != nil && spans(*h.AirWindow, aired)
}

func (v Vocabulary) lookup(id string) (Holiday, bool) {
	for _, h := range v.holidays {
		if h.ID == id {
			return h, true
		}
	}
	return Holiday{}, false
}

func (h Holiday) nameIn(text string) string {
	for _, alias := range h.Words.Names {
		if textmatch.ContainsPhraseOutside(text, alias, h.Words.LookAlikes...) {
			return alias
		}
	}
	return ""
}

// genresCorroborate matches genre words, so a TV library's "Sci-Fi & Fantasy" carries "fantasy".
func (h Holiday) genresCorroborate(genres []string) bool {
	joined := strings.Join(genres, " | ")
	for _, set := range h.Words.MoodGenres {
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
