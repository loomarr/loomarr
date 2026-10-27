package filler

import (
	"math"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/clipcatalog"
)

// The pool target is how many commercials each decade window needs (#2035): enough for the
// household's longest viewing evening without a repeat. Fetching runs while a channel's pool is
// short of it and stops when it is met.
const (
	// DefaultEvening is the evening sized for before the household has a history: about 80 ads.
	DefaultEvening = 6 * time.Hour
	// adsPerViewingHour turns an evening into ads: about 80 in six hours of breaks.
	adsPerViewingHour = 80.0 / 6
	// eveningGap ends an evening: a silence longer than this is the household away.
	eveningGap = 2 * time.Hour
	// minEveningHistory is how long the household must have watched before its own evenings size
	// the pool. Until then a quiet first week would shrink it below what a real evening needs.
	minEveningHistory = 4 * 7 * 24 * time.Hour
)

// anyEra is the decade key of a channel that takes any era.
const anyEra = 0

// PoolTargets is the pool each decade window needs.
type PoolTargets struct {
	// PerDecade is the most airings one evening drew from each decade, keyed by its first year
	// (anyEra for channels with no era).
	PerDecade map[int]float64
	// Decade is the least each decade takes: the default evening before there is a history, none
	// after, when a decade nobody watched needs nothing beyond Floor.
	Decade int
	// Floor is the least a channel takes: the rotation-spacing minimum, the ads it airs before one
	// may repeat.
	Floor int
	// FromHistory reports whether the household's evenings sized it.
	FromHistory bool
}

// For returns the pool a channel with these era windows needs: every decade it draws from, each at
// its own target, and never below the spacing floor. thisYear closes an open-ended window.
func (t PoolTargets) For(windows []EraRange, thisYear int) int {
	var need float64
	for _, d := range decadesOf(windows, thisYear) {
		need += max(t.PerDecade[d], float64(t.Decade))
	}
	// Shared airings sum in thirds and sevenths; the tolerance keeps N from reading as N+1.
	return max(int(math.Ceil(need-1e-9)), t.Floor)
}

// SizePool sizes the pool from the household's watched airings (#2017) over the last
// AiringHistoryAge. An evening is a run of airings with no silence longer than eveningGap, across
// every channel, since rotation rests an ad household-wide; each decade takes the most airings any
// one evening drew from it. An airing on a channel spanning several decades is shared between them,
// so one evening is not counted once per decade. windows maps a channel to the era windows its
// content uses; an airing on a channel no longer listed counts toward nothing. spacing is the
// commercial spacing, below which no channel's target falls.
func SizePool(airings []clipcatalog.Airing, windows map[string][]EraRange, spacing time.Duration, now time.Time) PoolTargets {
	floor := adsFor(spacing)
	since := now.Add(-clipcatalog.AiringHistoryAge)
	var recent []clipcatalog.Airing
	for _, a := range airings {
		if !a.AiredAt.Before(since) && !a.AiredAt.After(now) {
			recent = append(recent, a)
		}
	}
	slices.SortFunc(recent, func(a, b clipcatalog.Airing) int { return a.AiredAt.Compare(b.AiredAt) })
	if len(recent) == 0 || now.Sub(recent[0].AiredAt) < minEveningHistory {
		return PoolTargets{Decade: adsFor(DefaultEvening), Floor: floor}
	}

	thisYear := now.Year()
	longest := map[int]float64{}
	evening := map[int]float64{}
	closeEvening := func() {
		for d, n := range evening {
			longest[d] = max(longest[d], n)
		}
		clear(evening)
	}
	for i, a := range recent {
		if i > 0 && a.AiredAt.Sub(recent[i-1].AiredAt) > eveningGap {
			closeEvening()
		}
		w, ok := windows[a.ChannelID]
		if !ok {
			continue
		}
		decades := decadesOf(w, thisYear)
		for _, d := range decades {
			evening[d] += 1 / float64(len(decades))
		}
	}
	closeEvening()
	return PoolTargets{PerDecade: longest, Floor: floor, FromHistory: true}
}

func adsFor(d time.Duration) int {
	return int(math.Ceil(d.Hours() * adsPerViewingHour))
}

// decadesOf lists the decades the windows cover, by first year, once each. An open start reaches
// back to the decade of its end only, since a window "up to 1999" asks for that era, not every year
// before it; any era is the single anyEra key.
func decadesOf(windows []EraRange, thisYear int) []int {
	var out []int
	for _, r := range NormalizeEraWindows(windows) {
		if r.Any() {
			out = append(out, anyEra)
			continue
		}
		from, to := r.From, r.To
		if to <= 0 {
			to = thisYear
		}
		if from <= 0 {
			from = to
		}
		for d := from / 10 * 10; d <= to; d += 10 {
			out = append(out, d)
		}
	}
	if len(windows) == 0 {
		out = append(out, anyEra)
	}
	slices.Sort(out)
	return slices.Compact(out)
}
