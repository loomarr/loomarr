//go:build research

// Package kit is the shared measurement kit for the curation research (#1670, #1671).
//
// It turns a list of airings (what a channel put on screen, and when) into the repetition and
// variety numbers the report gates on. The HTTP measurer (../measure) and the offline simulator
// (../sim) both feed it, so a number read from a live backend and a number from a simulated
// mechanism mean exactly the same thing.
//
// Research code: excluded from every normal build by the `research` tag, never imported by the
// product. Run with `go run -tags research ./project/evidence/curation-1670/<tool>`.
package kit

import (
	"math"
	"sort"
	"time"
)

// Airing is one block on a channel's timeline.
type Airing struct {
	Unit  string    // the smallest repeatable thing: one episode or one film
	Title string    // the show (for an episode) or the film
	Kind  string    // "program" or "filler"
	Start time.Time // wall-clock start
	Stop  time.Time
	Clips []string // filler only: clip identities in play order
}

// Metrics is one channel's measurement over a span.
type Metrics struct {
	Hours          float64 `json:"hours"`
	Airings        int     `json:"airings"`
	PoolUnits      int     `json:"poolUnits"`      // episodes+films the channel could air (0 = unknown)
	DistinctUnits  int     `json:"distinctUnits"`  // distinct episodes+films aired
	DistinctTitles int     `json:"distinctTitles"` // distinct shows+films aired
	Coverage       float64 `json:"coverage"`       // distinctUnits / poolUnits (NaN when pool unknown)

	// RepeatRate24h is THE gate number: of the airings that start at least 24h into the span,
	// the share whose unit had already aired in the preceding 24 hours. 0 = nothing re-airs
	// within a day; 1 = everything does.
	RepeatRate24h float64 `json:"repeatRate24h"`
	// RepeatRate7d is the same over a 7-day lookback (needs a span of 8 days or more).
	RepeatRate7d float64 `json:"repeatRate7d"`

	ReairGapP10H  float64 `json:"reairGapP10H"` // re-air interval percentiles, hours
	ReairGapP50H  float64 `json:"reairGapP50H"`
	ReairGapP90H  float64 `json:"reairGapP90H"`
	ReairGapMinH  float64 `json:"reairGapMinH"`
	MaxUnitPerDay int     `json:"maxUnitPerDay"` // most airings of one unit on one calendar day

	// RepeatedAdjacency is the share of consecutive program pairs (A then B) that already
	// occurred earlier in the span — the "same loop again" texture a viewer notices even when
	// individual re-airs are spaced out.
	RepeatedAdjacency float64 `json:"repeatedAdjacency"`

	ClipPlays     int     `json:"clipPlays"`
	ClipsPerHour  float64 `json:"clipsPerHour"`
	DistinctClips int     `json:"distinctClips"`
	ClipRepeat60m float64 `json:"clipRepeat60m"` // share of clip plays whose clip played in the prior hour
	ClipRepeat24h float64 `json:"clipRepeat24h"`
	MaxClipPerDay int     `json:"maxClipPerDay"`
}

// Measure computes Metrics for one channel's airings over [from, to).
func Measure(airings []Airing, poolUnits int, from, to time.Time) Metrics {
	sort.SliceStable(airings, func(i, j int) bool { return airings[i].Start.Before(airings[j].Start) })
	m := Metrics{Hours: to.Sub(from).Hours(), PoolUnits: poolUnits, Coverage: math.NaN()}

	last := map[string]time.Time{}
	perDay := map[string]int{}
	units := map[string]bool{}
	titles := map[string]bool{}
	pairs := map[[2]string]bool{}
	var gaps []float64
	var eligible24, rep24, eligible7, rep7, pairN, pairRep int
	prev := ""

	clipLast := map[string]time.Time{}
	clipDay := map[string]int{}
	clips := map[string]bool{}
	var clip60, clip24 int

	for _, a := range airings {
		if a.Start.Before(from) || !a.Start.Before(to) {
			continue
		}
		if a.Kind == "filler" {
			for i, c := range a.Clips {
				if c == "" {
					continue
				}
				// Clips in one pod play back to back; stamp each a little later so the ordering
				// inside a pod is kept without inventing durations.
				at := a.Start.Add(time.Duration(i) * time.Second)
				m.ClipPlays++
				clips[c] = true
				if t, ok := clipLast[c]; ok {
					if at.Sub(t) <= time.Hour {
						clip60++
					}
					if at.Sub(t) <= 24*time.Hour {
						clip24++
					}
				}
				clipLast[c] = at
				k := c + "|" + at.Format("2006-01-02")
				clipDay[k]++
				if clipDay[k] > m.MaxClipPerDay {
					m.MaxClipPerDay = clipDay[k]
				}
			}
			continue
		}
		if a.Kind != "program" || a.Unit == "" {
			continue
		}
		m.Airings++
		units[a.Unit] = true
		titles[a.Title] = true
		t, seen := last[a.Unit]
		if seen {
			gaps = append(gaps, a.Start.Sub(t).Hours())
		}
		if !a.Start.Before(from.Add(24 * time.Hour)) {
			eligible24++
			if seen && a.Start.Sub(t) < 24*time.Hour {
				rep24++
			}
		}
		if !a.Start.Before(from.Add(7 * 24 * time.Hour)) {
			eligible7++
			if seen && a.Start.Sub(t) < 7*24*time.Hour {
				rep7++
			}
		}
		last[a.Unit] = a.Start
		k := a.Unit + "|" + a.Start.Format("2006-01-02")
		perDay[k]++
		if perDay[k] > m.MaxUnitPerDay {
			m.MaxUnitPerDay = perDay[k]
		}
		if prev != "" {
			p := [2]string{prev, a.Unit}
			pairN++
			if pairs[p] {
				pairRep++
			}
			pairs[p] = true
		}
		prev = a.Unit
	}

	m.DistinctUnits, m.DistinctTitles = len(units), len(titles)
	if poolUnits > 0 {
		m.Coverage = float64(m.DistinctUnits) / float64(poolUnits)
	}
	m.RepeatRate24h = ratio(rep24, eligible24)
	m.RepeatRate7d = ratio(rep7, eligible7)
	m.RepeatedAdjacency = ratio(pairRep, pairN)
	if len(gaps) > 0 {
		sort.Float64s(gaps)
		m.ReairGapMinH = gaps[0]
		m.ReairGapP10H = pct(gaps, 0.10)
		m.ReairGapP50H = pct(gaps, 0.50)
		m.ReairGapP90H = pct(gaps, 0.90)
	}
	m.DistinctClips = len(clips)
	if m.Hours > 0 {
		m.ClipsPerHour = float64(m.ClipPlays) / m.Hours
	}
	m.ClipRepeat60m = ratio(clip60, m.ClipPlays)
	m.ClipRepeat24h = ratio(clip24, m.ClipPlays)
	return m
}

func ratio(n, d int) float64 {
	if d == 0 {
		return math.NaN()
	}
	return float64(n) / float64(d)
}

// pct is a nearest-rank percentile over sorted values.
func pct(sorted []float64, p float64) float64 {
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	return sorted[i]
}
