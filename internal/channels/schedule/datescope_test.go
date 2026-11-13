package schedule_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

// eraCorpus is one lineup that touches every date axis: movies (release year), series
// (premiere year) and episodes (airing year), each with an unknown year (0) in the mix.
func eraCorpus() ([]schedule.LineupEntry, schedule.Availability, map[string]int, map[string]provision.Key) {
	const hours = int64(3_000_000_000) // > every default no-repeat window: no ladder step fires
	movies := map[string]int{"m1985": 1985, "m1990": 1990, "m1995": 1995, "m1999": 1999, "m2005": 2005, "m0": 0}
	series := []struct {
		key      string
		premiere int
		episodes map[string]int
	}{
		{"series:tmdb:90000001", 1989, map[string]int{"s1-1988": 1988, "s1-1993": 1993, "s1-2022": 2022, "s1-0": 0}},
		{"series:tmdb:90000002", 1995, map[string]int{"s2-1995": 1995, "s2-2003": 2003}},
		{"series:tmdb:90000003", 0, map[string]int{"s3-1991": 1991, "s3-2010": 2010}},
	}
	avail := seriesAvail{
		movies: map[provision.Key]struct {
			id  string
			dur int64
		}{},
		episodes: map[provision.Key][]schedule.ResolvedProgram{},
	}
	years := map[string]int{}            // program id → its own year (movie release or episode airing)
	parent := map[string]provision.Key{} // program id → the lineup entry it came from
	var entries []schedule.LineupEntry
	for id, year := range movies {
		key := provision.Key("movie:tmdb:" + id)
		entries = append(entries, schedule.LineupEntry{Key: key, Title: id, Year: year})
		avail.movies[key] = struct {
			id  string
			dur int64
		}{id, hours}
		years[id], parent[id] = year, key
	}
	for _, s := range series {
		key := provision.Key(s.key)
		entries = append(entries, schedule.LineupEntry{Key: key, Title: s.key, Year: s.premiere})
		n := 0
		for id, year := range s.episodes {
			n++
			avail.episodes[key] = append(avail.episodes[key], schedule.ResolvedProgram{
				LibraryItemID: id, Title: id, DurationMs: hours, Season: 1, Episode: n, Year: year,
			})
			years[id], parent[id] = year, key
		}
	}
	slices.SortFunc(entries, func(a, b schedule.LineupEntry) int { return strings.Compare(string(a.Key), string(b.Key)) })
	for key := range avail.episodes {
		slices.SortFunc(avail.episodes[key], func(a, b schedule.ResolvedProgram) int {
			return strings.Compare(a.LibraryItemID, b.LibraryItemID)
		})
	}
	return entries, avail, years, parent
}

func airedIDs(d schedule.DesiredLineup) []string {
	ids := programItemIDs(d)
	slices.Sort(ids)
	return slices.Compact(ids)
}

// Era-shaped dates (what migration 00230 folded a stored `era` into) must air exactly what the
// scheduler aired when it still evaluated Era. The oracle restates the deleted evaluator
// independently: a dated title (movie release / series premiere) and a dated episode must each
// fall in the range, and an unknown year (0) is never excluded.
func TestEraShapedDatesAirTheEraLineup(t *testing.T) {
	entries, avail, years, parent := eraCorpus()
	entryYear := map[provision.Key]int{}
	for _, e := range entries {
		entryYear[e.Key] = e.Year
	}
	for _, era := range []schedule.Range{{From: 1990, To: 1999}, {From: 1980, To: 1999}, {From: 1990}, {To: 1999}} {
		t.Run(fmt.Sprintf("%d-%d", era.From, era.To), func(t *testing.T) {
			var want []string
			for id, year := range years {
				titleYear := entryYear[parent[id]]
				if (titleYear == 0 || era.Contains(titleYear)) && (year == 0 || era.Contains(year)) {
					want = append(want, id)
				}
			}
			slices.Sort(want)

			normalised := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
				Scope: schedule.ScopePolicy{Dates: schedule.EraDates(era)},
			}}
			if got := airedIDs(computeWithPolicy(entries, avail, normalised)); !slices.Equal(got, want) {
				t.Errorf("era-shaped dates aired %v, want %v", got, want)
			}
		})
	}
}

// A stored scope that carried BOTH era and dates was the AND of the two. Migration 00230 folds it
// to their per-axis intersection, and that intersection must keep airing exactly the AND.
func TestFoldedEraAndDatesAirTheirIntersection(t *testing.T) {
	entries, avail, years, parent := eraCorpus()
	era := schedule.Range{From: 1990, To: 1999}
	movies := schedule.Range{From: 1995, To: 2005}
	airing := schedule.Range{From: 1985, To: 1994}
	entryYear := map[provision.Key]int{}
	for _, e := range entries {
		entryYear[e.Key] = e.Year
	}
	in := func(r schedule.Range, year int) bool { return year == 0 || r.Contains(year) }
	var want []string
	for id, year := range years {
		key, titleYear := parent[id], entryYear[parent[id]]
		if !in(era, titleYear) || !in(era, year) {
			continue
		}
		if !key.IsSeries() && !in(movies, titleYear) {
			continue
		}
		if key.IsSeries() && !in(airing, year) {
			continue
		}
		want = append(want, id)
	}
	slices.Sort(want)

	// The fold of era 1990–1999 with movieRelease 1995–2005 and seriesAiring 1985–1994.
	folded := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
		Dates: &schedule.DateScope{
			MovieRelease:   []schedule.Range{{From: 1995, To: 1999}},
			SeriesPremiere: []schedule.Range{era},
			SeriesAiring:   []schedule.Range{{From: 1990, To: 1994}},
		},
	}}}
	if got := airedIDs(computeWithPolicy(entries, avail, folded)); !slices.Equal(got, want) {
		t.Fatalf("folded era+dates aired %v, want %v", got, want)
	}
}

// Behaviour change (#1877): a rule's era used to filter only the series' premiere year, so a
// 1990s rule aired a 1995 series' 2003 episode. Era now means the same on a rule as on the
// channel: every dated episode must air inside it.
func TestRuleEraDropsOutOfRangeEpisodes(t *testing.T) {
	entries, avail, _, _ := eraCorpus()
	for name, p := range map[string]schedule.ChannelPolicy{
		"era-shaped rule dates": {ProposalPolicy: schedule.ProposalPolicy{Rules: []schedule.SchedulingRule{{
			ID: "r", What: &schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990, To: 1999})},
		}}}},
		"era preset token": func() schedule.ChannelPolicy {
			what, _, ok := schedule.LowerWhat("era:1990-1999")
			if !ok {
				t.Fatal("era token did not lower")
			}
			return schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Rules: []schedule.SchedulingRule{{ID: "r", What: what}}}}
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			got := airedIDs(computeWithPolicy(entries, avail, p))
			if slices.Contains(got, "s2-2003") || slices.Contains(got, "s1-2022") {
				t.Fatalf("rule era aired an out-of-range episode: %v", got)
			}
			if !slices.Contains(got, "s2-1995") || !slices.Contains(got, "s3-1991") {
				t.Fatalf("rule era dropped an in-range episode: %v", got)
			}
		})
	}
}

// Open-ended era-shaped dates stay valid, and a scope marshals no `era`.
func TestOpenEndedEraDatesAreValidAndWriteNoEra(t *testing.T) {
	p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Scope: schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990})},
		Rules: []schedule.SchedulingRule{{ID: "r", What: &schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{To: 1999})}}},
	}}
	if err := p.Validate(); err != nil {
		t.Fatalf("open-ended era dates must be valid: %v", err)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"era"`) {
		t.Fatalf("the backend wrote era: %s", out)
	}
}

func TestDatesValidationAllowsOneOpenEnd(t *testing.T) {
	for name, tc := range map[string]struct {
		dates schedule.DateScope
		ok    bool
	}{
		"from only":                  {schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990}}}, true},
		"to only":                    {schedule.DateScope{SeriesAiring: []schedule.Range{{To: 1999}}}, true},
		"both open":                  {schedule.DateScope{MovieRelease: []schedule.Range{{}}}, false},
		"out of span":                {schedule.DateScope{MovieRelease: []schedule.Range{{From: 1850}}}, false},
		"open end then later window": {schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990}, {From: 2000, To: 2005}}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			p := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{Dates: &tc.dates}}}
			if err := p.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestNormalizeRangesTreatsZeroAsOpen(t *testing.T) {
	for _, tc := range []struct{ in, want []schedule.Range }{
		{[]schedule.Range{{From: 1990}, {From: 1990}}, []schedule.Range{{From: 1990}}},
		{[]schedule.Range{{From: 2000, To: 2005}, {From: 1990}}, []schedule.Range{{From: 1990}}},
		{[]schedule.Range{{From: 1990, To: 1995}, {From: 1990}}, []schedule.Range{{From: 1990}}},
		{[]schedule.Range{{To: 1980}, {From: 1975, To: 1990}}, []schedule.Range{{To: 1990}}},
		{[]schedule.Range{{To: 1980}, {From: 1990}}, []schedule.Range{{To: 1980}, {From: 1990}}},
	} {
		if got := schedule.NormalizeRanges(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("NormalizeRanges(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// The filler seed and the live filler inheritance both read the era the dates imply; open-ended
// era-shaped dates must still seed the same open-ended filler era.
func TestSeedFillerSelectionKeepsAnOpenEndedEra(t *testing.T) {
	seed := schedule.SeedFillerSelection(schedule.ProposalPolicy{
		Scope: schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990})},
	})
	if seed.Era == nil || *seed.Era != (schedule.Range{From: 1990}) || seed.EraWindows != nil {
		t.Fatalf("seed = %+v, want era {1990, open}", seed)
	}
	if err := (schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Filler: seed}}).Validate(); err != nil {
		t.Fatalf("seeded selection is invalid: %v", err)
	}
}
