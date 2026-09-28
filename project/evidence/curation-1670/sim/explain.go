package main

import (
	"flag"
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

// explain (#1694) attributes every next-day repeat of the `current` mechanism to what the
// scheduler knew when it arranged the window that aired it: was the earlier airing already in
// the window's as-of-window-start history (so recency ordering had the chance to push it back),
// and where did the window's slice start in the recency-ordered deck?
var explainFlag = flag.Bool("explain", false, "attribute each next-day repeat of the current mechanism (#1694)")

// arrangement is one window's arrangement: the history it read and where its slice sat.
type arrangement struct {
	opened  time.Time
	history map[string]time.Time
	// deckPos is each unit's position in the window's full (unsliced) recency-ordered deck.
	deckPos map[string]int
	// sliceStart is the deck position of the slice's first programme; sliceLen its programme count.
	sliceStart, sliceLen int
	// aired is the set of the slice's units that actually aired in this window.
	aired map[string]bool
	slice []string
}

type explainer struct {
	windows map[time.Time]*arrangement
	// airedBy maps unit|start to the window that aired it.
	airedBy map[string]*arrangement
}

func newExplainer() *explainer {
	return &explainer{windows: map[time.Time]*arrangement{}, airedBy: map[string]*arrangement{}}
}

// arranged records one arrangement. full is the same arrangement with no window (the whole deck
// in order); slice is what the window keeps.
func (e *explainer) arranged(opened time.Time, history map[string]time.Time, full, slice []schedule.Slot) {
	if e == nil {
		return
	}
	if _, ok := e.windows[opened]; ok {
		return // the first arrangement of a window is the one that airs (history is as-of-open)
	}
	a := &arrangement{opened: opened, history: history, deckPos: map[string]int{}, aired: map[string]bool{}}
	n := 0
	for _, s := range full {
		if s.IsProgram() {
			a.deckPos[s.LibraryItemID] = n
			n++
		}
	}
	for _, s := range slice {
		if s.IsProgram() {
			if a.sliceLen == 0 {
				a.sliceStart = a.deckPos[s.LibraryItemID]
			}
			a.sliceLen++
			a.slice = append(a.slice, s.LibraryItemID)
		}
	}
	e.windows[opened] = a
}

func (e *explainer) leg(opened time.Time, bs []playout.Broadcast) {
	if e == nil {
		return
	}
	a := e.windows[opened]
	if a == nil {
		return
	}
	for _, b := range bs {
		if b.Kind != schedule.SlotProgram {
			continue
		}
		a.aired[b.LibraryItemID] = true
		id := b.LibraryItemID + "|" + b.Start.String()
		if _, ok := e.airedBy[id]; !ok {
			e.airedBy[id] = a
		}
	}
}

// report prints the attribution for one run's airings.
func (e *explainer) report(label string, airings []struct {
	unit  string
	start time.Time
}, start time.Time, pool int) {
	if e == nil {
		return
	}
	sort.Slice(airings, func(i, j int) bool { return airings[i].start.Before(airings[j].start) })
	prev := map[string]time.Time{}
	var eligible, repeats, known, unknown int
	var knownTailRank []int
	for _, x := range airings {
		p, seen := prev[x.unit]
		prev[x.unit] = x.start
		if x.start.Before(start.Add(24 * time.Hour)) {
			continue
		}
		eligible++
		if !seen || x.start.Sub(p) >= 24*time.Hour {
			continue
		}
		repeats++
		a := e.airedBy[x.unit+"|"+x.start.String()]
		if a == nil {
			continue
		}
		if h, ok := a.history[x.unit]; ok && !h.Before(p) {
			known++
			// How many units the history holds as MORE recent than this one: 0 = the most
			// recently aired unit the scheduler knew of.
			newer := 0
			for _, t := range a.history {
				if t.After(h) {
					newer++
				}
			}
			knownTailRank = append(knownTailRank, newer)
		} else {
			unknown++
		}
	}
	// Slices that reached into the recency-ordered deck's tail, and the programmes slices lost.
	var wins, tailWins, lost, sliced int
	for _, a := range e.windows {
		if a.opened.Before(start) {
			continue
		}
		wins++
		reaches := false
		for _, u := range a.slice {
			sliced++
			if !a.aired[u] {
				lost++
			}
			if _, ok := a.history[u]; ok {
				reaches = true
			}
		}
		// A slice that keeps a unit the history holds as aired while some unit it holds as
		// never aired sits outside the slice passed over the fresher unit.
		inSlice := map[string]bool{}
		for _, u := range a.slice {
			inSlice[u] = true
		}
		waits := false
		for u := range a.deckPos {
			if _, ok := a.history[u]; !ok && !inSlice[u] {
				waits = true
			}
		}
		if reaches && waits {
			tailWins++
		}
	}
	sort.Ints(knownTailRank)
	fmt.Printf("%-28s airings>=24h %4d  R24h %4d (%3.0f%%)  scheduler KNEW the earlier airing: %4d  did not: %4d  known-rank(newer units) %v\n",
		label, eligible, repeats, 100*float64(repeats)/float64(max(eligible, 1)), known, unknown, knownTailRank)
	fmt.Printf("%-28s windows %d, slices keeping an aired unit while a never-aired one waits %d; sliced programmes %d, never aired in their window %d\n",
		"", wins, tailWins, sliced, lost)
}
