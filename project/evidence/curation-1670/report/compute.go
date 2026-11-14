package main

import (
	"sort"
	"time"

	"github.com/loomarr/loomarr/project/evidence/curation-1670/kit"
)

// duplicateWithin is how close two watched rows of one item on one channel may be and still be
// one airing recorded twice.
const duplicateWithin = time.Minute

// shortTuneIn is the longest stay on a channel that is surfing past, not watching.
const shortTuneIn = time.Minute

// sameWeek is the household-wide rest the same episode must have between two channels.
const sameWeek = 7 * 24 * time.Hour

// neighbourDecade is how far a clip's year may sit outside its channel's filler era and still be
// eligible (#3838 decision 21: the channel's era, then the decade either side).
const neighbourDecade = 10

// Report is the whole measurement.
type Report struct {
	From, To  time.Time
	Channels  []ChannelReport
	Household HouseholdReport
}

// ChannelReport is one channel's programme and filler numbers.
type ChannelReport struct {
	Number       int     `json:"number"`
	Name         string  `json:"name"`
	Ordering     string  `json:"ordering"`
	LineupTitles int     `json:"lineupTitles"`
	LibraryUnits int     `json:"libraryUnits"` // episodes and films of the lineup the library holds
	LibraryHours float64 `json:"libraryHours"`
	Rules        int     `json:"rules"`

	Scheduled kit.Metrics `json:"scheduled"` // over the span, from the rows that aired (aired)
	FilmOnly  bool        `json:"filmOnly"`

	CleanDays    int     `json:"cleanDays"`    // whole days in the span
	MaxShowShare float64 `json:"maxShowShare"` // the largest share of a whole day one title took
	ShowsPerDay  int     `json:"showsPerDay"`  // median distinct titles on a whole day

	NeverScheduled   []string     `json:"neverScheduled"` // lineup titles with no start in the span
	Relaxations      []Relaxation `json:"relaxations"`
	RelaxedToNothing bool         `json:"relaxedToNothing"`

	ChannelDays   int      `json:"channelDays"`
	RedealDays    []string `json:"redealDays"`    // local dates on which a recorded deal was replaced
	ReplacedDeals int      `json:"replacedDeals"` // ledger rows that never aired

	WatchedRows      int `json:"watchedRows"`
	DuplicateWatched int `json:"duplicateWatched"`
	FillerAirings    int `json:"fillerAirings"`
	FillerExcluded   int `json:"fillerExcluded"`   // airings of clips the channel's rules would refuse
	InEraCommercials int `json:"inEraCommercials"` // airable; -1 when the channel names no filler era
}

// HouseholdReport is what spans channels.
type HouseholdReport struct {
	SameEpisodeTwoChannels int `json:"sameEpisodeTwoChannels"` // an episode or film on a second channel within a week
	ShortTuneInSeen        int `json:"shortTuneInSeen"`        // watched rows recorded only by a stay under a minute

	// EveningCutHour is the local hour evenings turn over, learned from the household's viewing
	// (eveningCut); EveningCutLearned is false when there was no viewing and it is midnight.
	EveningCutHour    int  `json:"eveningCutHour"`
	EveningCutLearned bool `json:"eveningCutLearned"`

	Evenings           int           `json:"evenings"`
	EveningsWithRepeat int           `json:"eveningsWithRepeat"`
	WorstEvening       EveningReport `json:"worstEvening"`

	AirableClips   int     `json:"airableClips"`
	AirableMinutes float64 `json:"airableMinutes"`
	HeldClips      int     `json:"heldClips"`
	HeldMinutes    float64 `json:"heldMinutes"`
}

// EveningReport is one watched evening of commercial breaks, household-wide.
type EveningReport struct {
	Start         time.Time `json:"start"`
	Plays         int       `json:"plays"`
	DistinctClips int       `json:"distinctClips"`
	MostPlays     int       `json:"mostPlays"` // the most any one clip aired that evening
	Repeated      int       `json:"repeated"`  // clips that aired more than once
}

// Compute measures the snapshot over [from, to), with days laid on loc.
func Compute(s Snapshot, from, to time.Time, loc *time.Location) Report {
	r := Report{From: from, To: to}
	recorded := s.Scheduled
	timeline, redealt := aired(s.Scheduled, s.Items, from, to, loc)
	// On a re-dealt day the ledger cannot say which deal ran, so the repeat measures read only the
	// days it can: a picked timeline there would invent re-airs minutes apart.
	var kept []ProgramAiring
	for _, a := range timeline {
		if !redealt[a.ChannelID][day(a.AiredAt, loc)] {
			kept = append(kept, a)
		}
	}
	s.Scheduled = kept
	for _, c := range s.Channels {
		cr := channelReport(s, c, from, to, loc)
		for d := range redealt[c.ID] {
			cr.RedealDays = append(cr.RedealDays, d)
		}
		sort.Strings(cr.RedealDays)
		// A re-dealt day still aired: it counts as a channel-day, and a title it started aired.
		cr.ChannelDays += len(cr.RedealDays)
		dealt := map[string]bool{}
		for _, a := range timeline {
			if a.ChannelID == c.ID {
				dealt[a.Key] = true
			}
		}
		never := cr.NeverScheduled[:0]
		for _, t := range c.Lineup {
			if !dealt[t.Key] {
				never = append(never, t.Title)
			}
		}
		cr.NeverScheduled = never
		for _, a := range recorded {
			if a.ChannelID == c.ID && inSpan(a.AiredAt, from, to) {
				cr.ReplacedDeals++
			}
		}
		for _, a := range timeline {
			if a.ChannelID == c.ID {
				cr.ReplacedDeals--
			}
		}
		r.Channels = append(r.Channels, cr)
	}
	r.Household = householdReport(s, from, to, loc)
	return r
}

func inSpan(t, from, to time.Time) bool { return !t.Before(from) && t.Before(to) }

func day(t time.Time, loc *time.Location) string { return t.In(loc).Format("2006-01-02") }

func channelReport(s Snapshot, c Channel, from, to time.Time, loc *time.Location) ChannelReport {
	cr := ChannelReport{
		Number: c.Number, Name: c.Name, Ordering: c.Ordering, LineupTitles: len(c.Lineup),
		Rules: c.Rules, Relaxations: c.Applied, RelaxedToNothing: relaxedToNothing(c.Applied),
		NeverScheduled: []string{}, RedealDays: []string{}, InEraCommercials: -1,
	}
	var ms int64
	for _, t := range c.Lineup {
		cr.LibraryUnits += t.Units
		ms += t.RuntimeMs
	}
	cr.LibraryHours = time.Duration(ms * int64(time.Millisecond)).Hours()

	var rows []ProgramAiring
	for _, a := range s.Scheduled {
		if a.ChannelID == c.ID && inSpan(a.AiredAt, from, to) {
			rows = append(rows, a)
		}
	}
	days := map[string]bool{}
	for _, a := range rows {
		days[day(a.AiredAt, loc)] = true
	}
	cr.ChannelDays = len(days)

	scheduledKeys := map[string]bool{}
	cr.FilmOnly = len(rows) > 0
	var kept []kit.Airing
	for _, a := range rows {
		scheduledKeys[a.Key] = true
		it := s.Items[a.ItemID]
		if it.Kind != "movie" {
			cr.FilmOnly = false
		}
		kept = append(kept, kit.Airing{
			Unit: a.ItemID, Title: a.Key, Kind: "program", Start: a.AiredAt,
			Stop: a.AiredAt.Add(time.Duration(it.RuntimeMs) * time.Millisecond),
		})
	}
	cr.Scheduled = kit.Measure(kept, cr.LibraryUnits, from, to)
	for _, t := range c.Lineup {
		if !scheduledKeys[t.Key] {
			cr.NeverScheduled = append(cr.NeverScheduled, t.Title)
		}
	}
	cr.CleanDays, cr.MaxShowShare, cr.ShowsPerDay = dayShape(kept, from, to, loc)

	cr.WatchedRows, cr.DuplicateWatched = watchedRows(s.Watched, c.ID, from, to)
	cr.FillerAirings, cr.FillerExcluded = fillerExcluded(s, c, from, to)
	if len(c.FillerEra) > 0 {
		cr.InEraCommercials = inEraCommercials(s, c)
	}
	return cr
}

// relaxedToNothing reports whether a texture guard was relaxed away entirely: the series gap to
// zero or the block limit to unbounded (#4140 step 1).
func relaxedToNothing(applied []Relaxation) bool {
	for _, a := range applied {
		if (a.Kind == "seriesMinGap" && a.To == "0s") || (a.Kind == "blockMax" && a.To == "unbounded") {
			return true
		}
	}
	return false
}

// dayShape reads the whole days inside the span (a re-dealt day is already gone from airings):
// how many there are, the largest share of one day a single title took, and the median number of
// distinct titles a day.
func dayShape(airings []kit.Airing, from, to time.Time, loc *time.Location) (days int, maxShare float64, median int) {
	first, last := day(from, loc), day(to.Add(-time.Nanosecond), loc)
	perDay := map[string]map[string]int{}
	for _, a := range airings {
		d := day(a.Start, loc)
		if d == first || d == last {
			continue // a partial day says nothing about a day's shape
		}
		if perDay[d] == nil {
			perDay[d] = map[string]int{}
		}
		perDay[d][a.Title]++
	}
	var counts []int
	for _, titles := range perDay {
		total := 0
		for _, n := range titles {
			total += n
		}
		for _, n := range titles {
			if share := float64(n) / float64(total); share > maxShare {
				maxShare = share
			}
		}
		counts = append(counts, len(titles))
	}
	if len(counts) == 0 {
		return 0, 0, 0
	}
	sort.Ints(counts)
	return len(counts), maxShare, counts[len(counts)/2]
}

// watchedRows counts a channel's watched rows in the span, and how many of them repeat an airing
// already recorded moments before.
func watchedRows(watched []ProgramAiring, channel string, from, to time.Time) (rows, duplicates int) {
	last := map[string]time.Time{}
	for _, a := range watched {
		if a.ChannelID != channel || !inSpan(a.AiredAt, from, to) {
			continue
		}
		rows++
		if t, ok := last[a.ItemID]; ok && a.AiredAt.Sub(t) <= duplicateWithin {
			duplicates++
		}
		last[a.ItemID] = a.AiredAt
	}
	return rows, duplicates
}

// fillerExcluded counts a channel's filler airings in the span, and those of clips its rules
// would refuse under #3838: a clip missing a fact (no role, no era), a political or news clip the
// channel did not opt into, and a clip further from the channel's era than the decade either side.
func fillerExcluded(s Snapshot, c Channel, from, to time.Time) (airings, excluded int) {
	for _, a := range s.FillerAirings {
		if a.ChannelID != c.ID || !inSpan(a.AiredAt, from, to) {
			continue
		}
		airings++
		if refused(s.Clips[a.Hash], c) {
			excluded++
		}
	}
	return airings, excluded
}

func refused(clip Clip, c Channel) bool {
	switch {
	case clip.Role == "" || clip.Role == "unclassified" || clip.Era == 0:
		return true
	case (clip.Role == "political" || clip.Role == "news") && !contains(c.FillerRoles, clip.Role):
		return true
	case len(c.FillerEra) > 0 && !nearEra(clip.Era, c.FillerEra, neighbourDecade):
		return true
	}
	return false
}

func nearEra(year int, eras []YearRange, slack int) bool {
	for _, e := range eras {
		if year >= e.From-slack && year <= e.To+slack {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// inEraCommercials counts the airable commercials inside the channel's own filler era, from the
// household's country when it has one: the set #3838 sizes to its recipe.
func inEraCommercials(s Snapshot, c Channel) int {
	n := 0
	for _, clip := range s.Clips {
		if !clip.Airable() || clip.Role != "commercial" || !nearEra(clip.Era, c.FillerEra, 0) {
			continue
		}
		if s.HomeCountry != "" && clip.Country != s.HomeCountry {
			continue
		}
		n++
	}
	return n
}

func householdReport(s Snapshot, from, to time.Time, loc *time.Location) HouseholdReport {
	h := HouseholdReport{
		SameEpisodeTwoChannels: sameEpisodeTwoChannels(s.Scheduled, from, to),
		ShortTuneInSeen:        shortTuneInSeen(s.Watched, s.Viewings, from, to),
	}
	var clips []ClipAiring
	for _, a := range s.FillerAirings {
		if inSpan(a.AiredAt, from, to) {
			clips = append(clips, a)
		}
	}
	h.EveningCutHour, h.EveningCutLearned = eveningCut(s.Viewings, from, to, loc)
	for _, ev := range evenings(clips, loc, h.EveningCutHour) {
		er := measureEvening(ev)
		h.Evenings++
		if er.Repeated > 0 {
			h.EveningsWithRepeat++
		}
		if er.MostPlays > h.WorstEvening.MostPlays ||
			(er.MostPlays == h.WorstEvening.MostPlays && er.Plays > h.WorstEvening.Plays) {
			h.WorstEvening = er
		}
	}
	for _, c := range s.Clips {
		minutes := time.Duration(c.DurationMs * int64(time.Millisecond)).Minutes()
		switch {
		case c.Airable():
			h.AirableClips++
			h.AirableMinutes += minutes
		case c.Held && !c.Removed:
			h.HeldClips++
			h.HeldMinutes += minutes
		}
	}
	return h
}

// sameEpisodeTwoChannels counts the starts of an episode or film that another channel started
// within the week before. scheduled holds only the rows that aired (aired).
func sameEpisodeTwoChannels(scheduled []ProgramAiring, from, to time.Time) int {
	byItem := map[string][]ProgramAiring{}
	for _, a := range scheduled {
		if inSpan(a.AiredAt, from, to) {
			byItem[a.ItemID] = append(byItem[a.ItemID], a)
		}
	}
	n := 0
	for _, list := range byItem {
		sort.Slice(list, func(i, j int) bool { return list[i].AiredAt.Before(list[j].AiredAt) })
		for i, a := range list {
			for j := i - 1; j >= 0 && a.AiredAt.Sub(list[j].AiredAt) < sameWeek; j-- {
				if list[j].ChannelID != a.ChannelID {
					n++
					break
				}
			}
		}
	}
	return n
}

// shortTuneInSeen counts watched rows that only a stay of under a minute could have recorded: the
// row was first recorded during a short viewing of that channel and during no longer one.
func shortTuneInSeen(watched []ProgramAiring, viewings []Viewing, from, to time.Time) int {
	n := 0
	for _, a := range watched {
		if !inSpan(a.AiredAt, from, to) {
			continue
		}
		short, long := false, false
		for _, v := range viewings {
			if v.ChannelID != a.ChannelID || a.RecordedAt.Before(v.Started.Add(-5*time.Second)) || a.RecordedAt.After(v.Ended.Add(5*time.Second)) {
				continue
			}
			if v.Ended.Sub(v.Started) < shortTuneIn {
				short = true
			} else {
				long = true
			}
		}
		if short && !long {
			n++
		}
	}
	return n
}

func measureEvening(ev []ClipAiring) EveningReport {
	er := EveningReport{Plays: len(ev)}
	if len(ev) > 0 {
		er.Start = ev[0].AiredAt
	}
	plays := map[string]int{}
	for _, a := range ev {
		plays[a.Hash]++
	}
	er.DistinctClips = len(plays)
	for _, n := range plays {
		if n > er.MostPlays {
			er.MostPlays = n
		}
		if n > 1 {
			er.Repeated++
		}
	}
	return er
}
