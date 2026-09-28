// Package ideas builds library-grounded channel ideas for Home (#1665): what the household's own
// library could make into a channel that nothing plays yet, and what a holiday ahead could fill.
// It needs no LLM, because the LLM can be off. Each idea carries a typed reason and its numbers,
// never prose, so clients word it and a translation never has to parse a sentence.
package ideas

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/provision"
)

// Item is one library movie or series. Keys are every provision key it answers to (TVDB and TMDB
// forms), canonical first, because a lineup entry may carry either.
type Item struct {
	Keys      []provision.Key
	MediaType provision.MediaType
	Name      string
	Year      int
	Genres    []string
}

// Holiday is one seasonal-calendar window the caller considers ahead (or on now).
type Holiday struct {
	ID         string
	Start, End time.Time
}

type Facet string

const (
	FacetGenre   Facet = "genre"
	FacetDecade  Facet = "decade"
	FacetHoliday Facet = "holiday"
)

type ReasonKind string

const (
	// ReasonUnaired: Count library titles in this facet play on no channel yet.
	ReasonUnaired ReasonKind = "unaired"
	// ReasonHoliday: a holiday window is ahead or on now, and Count titles are about it.
	ReasonHoliday ReasonKind = "holiday"
)

type Reason struct {
	Kind  ReasonKind
	Count int
	// HolidayID, StartsAt and EndsAt are set for ReasonHoliday only.
	HolidayID        string
	StartsAt, EndsAt time.Time
}

// Idea is one channel idea. ID is stable across calls ("genre:comedy", "decade:1990",
// "holiday:halloween"), so a person's hide sticks. Keys are the titles, newest first and capped at
// MaxKeys; Movies and Series count all of them.
type Idea struct {
	ID     string
	Facet  Facet
	Value  string // the genre as the library spells it, the decade's first year, or the holiday id
	Reason Reason
	Keys   []provision.Key
	Movies int
	Series int
}

// Input is everything an idea is built from. OnChannel holds every key on a live or paused
// channel's lineup; Hidden holds the caller's hidden idea IDs.
type Input struct {
	Library   []Item
	OnChannel map[provision.Key]bool
	Holidays  []Holiday
	Hidden    map[string]bool
	Now       time.Time
}

const (
	// MinFacetTitles is the smallest facet worth a channel: fewer and it repeats within a day.
	MinFacetTitles = 6
	// MinHolidayTitles is lower because a holiday channel runs for weeks, not all year.
	MinHolidayTitles = 3
	// MaxKeys bounds one idea's lineup in the response. Counts still cover every title.
	MaxKeys = 100
)

// Build returns the ideas, holidays first (soonest), then facets by how many titles they would put
// on air, then by ID.
func Build(in Input) []Idea {
	var out []Idea
	for _, h := range in.Holidays {
		out = append(out, holidayIdea(h, in.Library))
	}
	out = append(out, facetIdeas(FacetGenre, in, func(it Item) []string { return it.Genres })...)
	out = append(out, facetIdeas(FacetDecade, in, decadeOf)...)

	kept := out[:0]
	for _, idea := range out {
		if idea.ID != "" && !in.Hidden[idea.ID] {
			kept = append(kept, idea)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if (a.Facet == FacetHoliday) != (b.Facet == FacetHoliday) {
			return a.Facet == FacetHoliday
		}
		if a.Facet == FacetHoliday && !a.Reason.StartsAt.Equal(b.Reason.StartsAt) {
			return a.Reason.StartsAt.Before(b.Reason.StartsAt)
		}
		if a.Reason.Count != b.Reason.Count {
			return a.Reason.Count > b.Reason.Count
		}
		return a.ID < b.ID
	})
	return kept
}

// holidayIdea gathers the titles about a holiday. It matches the title, never the genre, for the
// reason the scheduler's seasonal detection does (schedule.isSeasonal): a horror film is not about
// Halloween. Titles already on a channel are kept, since a seasonal channel is its own lineup.
func holidayIdea(h Holiday, library []Item) Idea {
	var matched []Item
	for _, it := range library {
		if holidayvocab.MatchesEvidence(h.ID, strings.ToLower(it.Name)) {
			matched = append(matched, it)
		}
	}
	if len(matched) < MinHolidayTitles {
		return Idea{}
	}
	idea := newIdea(FacetHoliday, h.ID, h.ID, matched)
	idea.Reason = Reason{Kind: ReasonHoliday, Count: len(matched), HolidayID: h.ID, StartsAt: h.Start, EndsAt: h.End}
	return idea
}

// facetIdeas groups the unaired titles by a facet (case-insensitively, keeping the library's
// first spelling) and makes an idea of each group big enough for a channel.
func facetIdeas(facet Facet, in Input, values func(Item) []string) []Idea {
	type group struct {
		value string
		items []Item
	}
	groups := map[string]*group{}
	for _, it := range in.Library {
		if onChannel(it, in.OnChannel) {
			continue
		}
		for _, v := range values(it) {
			norm := strings.ToLower(strings.TrimSpace(v))
			if norm == "" {
				continue
			}
			g, ok := groups[norm]
			if !ok {
				g = &group{value: strings.TrimSpace(v)}
				groups[norm] = g
			}
			g.items = append(g.items, it)
		}
	}
	var out []Idea
	for norm, g := range groups {
		if len(g.items) < MinFacetTitles {
			continue
		}
		idea := newIdea(facet, norm, g.value, g.items)
		idea.Reason = Reason{Kind: ReasonUnaired, Count: len(g.items)}
		out = append(out, idea)
	}
	return out
}

func newIdea(facet Facet, idPart, value string, items []Item) Idea {
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Year != sorted[j].Year {
			return sorted[i].Year > sorted[j].Year
		}
		return sorted[i].Name < sorted[j].Name
	})
	idea := Idea{ID: string(facet) + ":" + idPart, Facet: facet, Value: value}
	for _, it := range sorted {
		if it.MediaType == provision.Series {
			idea.Series++
		} else {
			idea.Movies++
		}
		if len(idea.Keys) < MaxKeys && len(it.Keys) > 0 {
			idea.Keys = append(idea.Keys, it.Keys[0])
		}
	}
	return idea
}

func onChannel(it Item, keys map[provision.Key]bool) bool {
	for _, k := range it.Keys {
		if keys[k] {
			return true
		}
	}
	return false
}

func decadeOf(it Item) []string {
	if it.Year <= 0 {
		return nil
	}
	return []string{strconv.Itoa(it.Year / 10 * 10)}
}
