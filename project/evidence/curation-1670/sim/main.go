// Command sim replays channels hour by hour through the REAL pure scheduler
// (schedule.ComputeDesiredAt) and the REAL playout walk (playout.BroadcastsBetween), with the
// viewing → recency feedback loop the live guide forecast cannot show, and compares the current
// behaviour against prototype mechanisms (#1670). Nothing here is wired into the product.
//
//	go run ./project/evidence/curation-1670/sim -days 14 -out /tmp/sim.json
//
// Channel shapes are synthetic (no titles, only sizes and runtimes), chosen to match shapes the
// design docs already record and the demo library.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/project/evidence/curation-1670/kit"
)

// shape is a synthetic channel: some films and/or some series.
type shape struct {
	Name     string
	Films    int
	FilmMin  [2]int // runtime range, minutes
	Series   int
	Episodes int // per series
	EpMin    int
	Ordering schedule.OrderingMode
}

var shapes = []shape{
	{Name: "demo-cartoons", Series: 6, Episodes: 6, EpMin: 11, Ordering: schedule.OrderShuffle},
	{Name: "films-27", Films: 27, FilmMin: [2]int{88, 135}, Ordering: schedule.OrderShuffle},
	{Name: "films-60", Films: 60, FilmMin: [2]int{88, 135}, Ordering: schedule.OrderShuffle},
	{Name: "kids-6x26", Series: 6, Episodes: 26, EpMin: 11, Ordering: schedule.OrderSyndication},
	{Name: "sitcom-5x100", Series: 5, Episodes: 100, EpMin: 22, Ordering: schedule.OrderSyndication},
	{Name: "mixed-3x40+20", Series: 3, Episodes: 40, EpMin: 44, Films: 20, FilmMin: [2]int{90, 125}, Ordering: schedule.OrderShuffle},
}

// library is a fake schedule.Availability over the synthetic catalogue.
type library struct {
	movies map[provision.Key]schedule.ResolvedProgram
	series map[provision.Key][]schedule.ResolvedProgram
}

func (l library) Resolve(k provision.Key) (string, int64, bool) {
	p, ok := l.movies[k]
	return p.LibraryItemID, p.DurationMs, ok
}

func (l library) ResolveEpisodes(k provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{Programs: l.series[k]}
}

func build(s shape) ([]schedule.LineupEntry, library) {
	r := rand.New(rand.NewSource(int64(len(s.Name))*7919 + 1))
	lib := library{movies: map[provision.Key]schedule.ResolvedProgram{}, series: map[provision.Key][]schedule.ResolvedProgram{}}
	var entries []schedule.LineupEntry
	for i := range s.Films {
		k := provision.Key(fmt.Sprintf("movie:tmdb:%d", 1000+i))
		min := s.FilmMin[0] + r.Intn(s.FilmMin[1]-s.FilmMin[0]+1)
		lib.movies[k] = schedule.ResolvedProgram{LibraryItemID: fmt.Sprintf("film-%d", i), Title: fmt.Sprintf("Film %d", i), DurationMs: int64(min) * 60000}
		entries = append(entries, schedule.LineupEntry{Key: k, Title: fmt.Sprintf("Film %d", i), DurationMs: int64(min) * 60000})
	}
	for i := range s.Series {
		k := provision.Key(fmt.Sprintf("series:tvdb:%d", 5000+i))
		for e := 1; e <= s.Episodes; e++ {
			lib.series[k] = append(lib.series[k], schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("show-%d-e%d", i, e), Title: fmt.Sprintf("Show %d ep %d", i, e),
				DurationMs: int64(s.EpMin) * 60000, Season: 1 + (e-1)/26, Episode: 1 + (e-1)%26,
			})
		}
		entries = append(entries, schedule.LineupEntry{Key: k, Title: fmt.Sprintf("Show %d", i)})
	}
	return entries, lib
}

// viewing says whether someone is watching the channel at t (drives airing records, §3.1).
type viewing struct {
	Name  string
	Watch func(t time.Time) bool
}

var viewings = []viewing{
	{"nobody", func(time.Time) bool { return false }},
	{"evenings", func(t time.Time) bool { h := t.Hour(); return h >= 19 && h < 23 }},
	{"always-on", func(time.Time) bool { return true }},
}

// run is one simulated channel under one mechanism and viewing pattern.
type run struct {
	Shape     string      `json:"shape"`
	Viewing   string      `json:"viewing"`
	Mechanism string      `json:"mechanism"`
	PoolHours float64     `json:"poolHours"`
	Metrics   kit.Metrics `json:"metrics"`
	// PrimeR7d is the 7-day repeat rate for airings starting 19:00–23:00 local.
	PrimeR7d float64 `json:"primeR7d"`
	// CutPerDay counts programmes that did not air to their end (a hard cut mid-programme).
	CutPerDay float64 `json:"cutPerDay"`
	// EPGMiss is the share of 5-minute samples where what aired differs from the guide
	// forecast made at the start of that rolling window.
	EPGMiss float64 `json:"epgMiss"`
}

const window = 24 * time.Hour

// simLoc is the household's wall clock: viewing hours and "primetime" are local.
var simLoc = func() *time.Location {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return l
}()

func main() {
	days := flag.Int("days", 14, "days to simulate")
	out := flag.String("out", "", "write JSON results here")
	only := flag.String("shape", "", "run only shapes whose name contains this")
	flag.Parse()

	loc := simLoc
	// A window-aligned start (00:00 UTC), as the scheduler's windowIndex sees days.
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Duration(*days) * 24 * time.Hour)

	var runs []run
	for _, s := range shapes {
		if *only != "" && !strings.Contains(s.Name, *only) {
			continue
		}
		entries, lib := build(s)
		pool, poolMs := poolOf(lib)
		for _, v := range viewings {
			a, cut, miss := current(s, entries, lib, v, start, end, loc)
			runs = append(runs, finish(s, v.Name, "current", pool, poolMs, a, cut, miss, start, end, loc))
		}
		// Prototype 1 ignores viewing: it keeps its own ledger of what AIRED, watched or not.
		a, cut := ledger(s, entries, lib, start, end, nil)
		runs = append(runs, finish(s, "any", "ledger-lru", pool, poolMs, a, cut, 0, start, end, loc))
		// Prototype 2 is household-aware: the same ledger, but a unit the household WATCHED
		// counts as more recent than one that merely aired, and never-watched units go first in
		// each batch (which starts at the evening boundary here), so novelty lands where people
		// are watching.
		for _, v := range viewings[1:] {
			a, cut := ledger(s, entries, lib, start, end, v.Watch)
			runs = append(runs, finish(s, v.Name, "watched-lru", pool, poolMs, a, cut, 0, start, end, loc))
		}
	}
	table(runs)
	fillerRuns := fillerSim(start, end)
	if *out != "" {
		b, err := json.MarshalIndent(map[string]any{"programs": runs, "filler": fillerRuns}, "", "  ")
		if err == nil {
			err = os.WriteFile(*out, b, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "sim:", err)
			os.Exit(1)
		}
	}
}

func poolOf(lib library) (int, int64) {
	n, ms := 0, int64(0)
	for _, p := range lib.movies {
		n, ms = n+1, ms+p.DurationMs
	}
	for _, eps := range lib.series {
		for _, p := range eps {
			n, ms = n+1, ms+p.DurationMs
		}
	}
	return n, ms
}

func finish(s shape, v, mech string, pool int, poolMs int64, a []kit.Airing, cut int, miss float64, start, end time.Time, loc *time.Location) run {
	for i := range a {
		a[i].Start, a[i].Stop = a[i].Start.In(loc), a[i].Stop.In(loc)
	}
	r := run{Shape: s.Name, Viewing: v, Mechanism: mech, PoolHours: float64(poolMs) / 3.6e6,
		Metrics: kit.Measure(a, pool, start, end), EPGMiss: miss}
	r.CutPerDay = float64(cut) / end.Sub(start).Hours() * 24
	var prime []kit.Airing
	for _, x := range a {
		if h := x.Start.Hour(); h >= 19 && h < 23 {
			prime = append(prime, x)
		}
	}
	r.PrimeR7d = kit.Measure(prime, pool, start, end).RepeatRate7d
	return r
}

// current replays today's pipeline. Every hour (reconcile cadence, coarsened) reconcile turns the
// rolling window with carry-over (#1675, playout.WindowTurn) and recomputes the lineup from the
// recorded airings as of that window's start (#1674). Playout walks the accepted cycle from its
// anchor, and the next window airs from the end of the programme crossing the boundary, even
// between reconciles (the resolver's carry-over walk). The window grid is the household's wall
// clock (guide.timezone). Airings are recorded per unit, at programme start, only while watched.
func current(s shape, entries []schedule.LineupEntry, lib library, v viewing, start, end time.Time, loc *time.Location) ([]kit.Airing, int, float64) {
	ch := schedule.Channel{ID: "sim-" + s.Name, Name: s.Name, Number: 1, Strategy: schedule.Shuffle, DefaultWindow: window, WindowZone: loc}
	policy := schedule.ChannelPolicy{}
	policy.Ordering = s.Ordering
	var desired []schedule.Slot
	anchor := start.Add(-37 * time.Hour) // a channel that went live some time before the span
	lastAired := airLog{}

	arrange := func(opened, at time.Time, last airLog) []schedule.Slot {
		c := ch
		c.WindowOpened = opened
		c.LastAired = last.asOf(opened)
		return schedule.ComputeDesiredAt(c, entries, lib, schedule.PodFill, policy, at).Slots
	}
	reconcile := func(t time.Time) {
		opened, epoch, _ := playout.WindowTurn(desired, anchor, window, loc, t)
		if opened.IsZero() {
			opened = schedule.WindowStart(t, window, loc)
		}
		desired, anchor = arrange(opened, t, lastAired), epoch
	}
	// walk is the timeline over [from, to): the accepted cycle until the programme crossing its
	// boundary ends, then each next window from where the one before ended. A window arranged
	// before reconcile commits it sees the airings recorded so far, as the product does (each
	// tune-in records as it airs).
	walk := func(from, to time.Time, watched bool) []playout.Broadcast {
		slots, epoch, last := desired, anchor, lastAired
		var out []playout.Broadcast
		for {
			stop := playout.CarryOverEnd(slots, epoch, schedule.NextWindowStart(epoch, window, loc))
			leg := playout.BroadcastsBetween(slots, epoch, maxTime(from, epoch), minTime(to, stop))
			out = append(out, leg...)
			if !stop.Before(to) {
				return out
			}
			if watched {
				last = last.with(leg, from)
			}
			slots, epoch = arrange(schedule.WindowStart(stop, window, loc), stop, last), stop
		}
	}
	for t := anchor; t.Before(start); t = t.Add(time.Hour) { // the channel's life before the span
		reconcile(t)
	}

	seen := map[string]bool{}
	var airings []kit.Airing
	cuts := 0
	var forecast, prev []playout.Broadcast
	samples, misses := 0, 0
	for t := start; t.Before(end); t = t.Add(time.Hour) {
		reconcile(t)
		watched := v.Watch(t.In(loc))
		hour := walk(t, t.Add(time.Hour), watched)
		if schedule.WindowStart(t, window, loc).Equal(t) { // a rolling-window boundary: the guide's forecast
			forecast = walk(t, t.Add(window), false)
		}
		for m := 0; m < 60; m += 5 {
			at := t.Add(time.Duration(m) * time.Minute)
			samples++
			if unitAt(hour, at) != unitAt(forecast, at) {
				misses++
			}
		}
		// A programme the last hour walked past its end which this hour's timeline no longer airs
		// at the same start was replaced before it ended: cut mid-programme.
		if n := len(prev); n > 0 && prev[n-1].Kind == schedule.SlotProgram && prev[n-1].Stop.After(t) {
			if len(hour) == 0 || hour[0].LibraryItemID != prev[n-1].LibraryItemID || !hour[0].Start.Equal(prev[n-1].Start) {
				cuts++
			}
		}
		prev = hour
		for _, b := range hour {
			if b.Kind != schedule.SlotProgram {
				continue
			}
			id := b.LibraryItemID + "|" + b.Start.String()
			if seen[id] {
				continue
			}
			seen[id] = true
			airings = append(airings, kit.Airing{Unit: b.LibraryItemID, Title: string(b.Key), Kind: "program", Start: b.Start, Stop: b.Stop})
		}
		if watched {
			lastAired = lastAired.with(hour, t)
		}
	}
	return airings, cuts, float64(misses) / float64(samples)
}

// airLog mirrors the #1674 airings table: one row per airing of a unit, with when it was recorded.
type airLog map[string][][2]time.Time

func (l airLog) record(unit string, aired, recorded time.Time) {
	l[unit] = append(l[unit], [2]time.Time{aired, recorded})
}

func (l airLog) asOf(before time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	if before.IsZero() {
		return out
	}
	for u, rows := range l {
		for _, r := range rows {
			if r[1].Before(before) && r[0].After(out[u]) {
				out[u] = r[0]
			}
		}
	}
	return out
}

// with is a copy of the log plus a record of every programme in bs, observed from `seen` on:
// RecordAiring stamps the programme start and never records earlier than the programme itself.
func (l airLog) with(bs []playout.Broadcast, seen time.Time) airLog {
	m := make(airLog, len(l))
	for u, rows := range l {
		m[u] = rows[:len(rows):len(rows)]
	}
	for _, b := range bs {
		if b.Kind == schedule.SlotProgram && b.LibraryItemID != "" {
			m.record(b.LibraryItemID, b.Start, maxTime(b.Start, seen))
		}
	}
	return m
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func unitAt(bs []playout.Broadcast, at time.Time) string {
	for _, b := range bs {
		if !at.Before(b.Start) && at.Before(b.Stop) {
			return b.LibraryItemID
		}
	}
	return ""
}

// ledger is the prototype: an APPEND-ONLY timeline with an airing ledger per episode/film.
//
//   - What aired is recorded when it is scheduled to air, watched or not (a linear channel airs
//     whether or not anyone is tuned in), per UNIT (episode or film), not per show.
//   - At each window boundary the next ~24h batch is the least-recently-aired units (never-aired
//     first), so every unit airs once before any repeats, whatever the deck order does.
//   - The batch is placed in the order of a deck reshuffled every pass (seed XOR pass, the
//     §5 promise), so adjacencies are fresh from loop to loop.
//   - The timeline is appended, never re-walked from a fixed anchor: the programme that crosses
//     a window boundary finishes, and the next batch starts after it. No mid-programme cut.
func ledger(s shape, entries []schedule.LineupEntry, lib library, start, end time.Time, watch func(time.Time) bool) ([]kit.Airing, int) {
	ch := schedule.Channel{ID: "sim-" + s.Name, Name: s.Name, Number: 1, Strategy: schedule.Shuffle}
	policy := schedule.ChannelPolicy{}
	policy.Ordering = s.Ordering
	policy.Window = schedule.WindowFull

	last := map[string]time.Time{}
	watched := map[string]time.Time{}
	airedCount := 0
	var airings []kit.Airing
	var queue []schedule.Slot
	t := start
	nextBoundary := start
	pool, _ := poolOf(lib)
	for t.Before(end) {
		if !t.Before(nextBoundary) || len(queue) == 0 {
			pass := int64(airedCount / max(pool, 1))
			ch.Shuffle.Seed = 0x5eed ^ pass
			deck := schedule.ComputeDesiredAt(ch, entries, lib, schedule.PodFill, policy, t).Slots
			queue = batch(deck, last, watched, window)
			for !nextBoundary.After(t) {
				nextBoundary = nextBoundary.Add(window)
			}
		}
		p := queue[0]
		queue = queue[1:]
		d := time.Duration(p.DurationMs) * time.Millisecond
		airings = append(airings, kit.Airing{Unit: p.LibraryItemID, Title: string(p.Key), Kind: "program", Start: t, Stop: t.Add(d)})
		last[p.LibraryItemID] = t
		if watch != nil && watch(t.In(simLoc)) {
			watched[p.LibraryItemID] = t
		}
		airedCount++
		t = t.Add(d)
	}
	return airings, 0
}

// batch picks ~budget of the least-recently-aired programmes and returns them in deck order.
// With a non-empty `watched` memory, a unit the household watched ranks behind every unit it has
// not, and the batch leads with the never-watched units (the evening, in this simulation).
func batch(deck []schedule.Slot, last, watched map[string]time.Time, budget time.Duration) []schedule.Slot {
	var progs []schedule.Slot
	pos := map[string]int{}
	for _, s := range deck {
		if s.IsProgram() {
			pos[s.LibraryItemID] = len(progs)
			progs = append(progs, s)
		}
	}
	lru := append([]schedule.Slot(nil), progs...)
	sort.SliceStable(lru, func(i, j int) bool {
		wi, wki := watched[lru[i].LibraryItemID]
		wj, wkj := watched[lru[j].LibraryItemID]
		if wki != wkj {
			return !wki
		}
		if wki && !wi.Equal(wj) {
			return wi.Before(wj)
		}
		ti, oki := last[lru[i].LibraryItemID]
		tj, okj := last[lru[j].LibraryItemID]
		if oki != okj {
			return !oki
		}
		return ti.Before(tj)
	})
	var acc int64
	var picked []schedule.Slot
	for _, s := range lru {
		picked = append(picked, s)
		acc += s.DurationMs
		if acc >= budget.Milliseconds() {
			break
		}
	}
	sort.SliceStable(picked, func(i, j int) bool {
		_, wi := watched[picked[i].LibraryItemID]
		_, wj := watched[picked[j].LibraryItemID]
		if wi != wj {
			return !wi
		}
		return pos[picked[i].LibraryItemID] < pos[picked[j].LibraryItemID]
	})
	return picked
}

func table(runs []run) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "shape\tpool\tpool h\tmechanism\tviewing\taired\tR24h\tR7d\tprimeR7d\tgap p10\tp50\tp90\tmax/day\tadjRep\tcuts/day\tEPG miss\t")
	for _, r := range runs {
		m := r.Metrics
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%.0f\t%s\t%s\t%d\t%s\t%s\t%s\t%.1f\t%.1f\t%.1f\t%d\t%s\t%.1f\t%s\t\n",
			r.Shape, m.PoolUnits, r.PoolHours, r.Mechanism, r.Viewing, m.DistinctUnits, pc(m.RepeatRate24h), pc(m.RepeatRate7d),
			pc(r.PrimeR7d), m.ReairGapP10H, m.ReairGapP50H, m.ReairGapP90H, m.MaxUnitPerDay, pc(m.RepeatedAdjacency), r.CutPerDay, pc(r.EPGMiss))
	}
	_ = tw.Flush()
}

func pc(f float64) string {
	if f < 0 || f != f {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*f)
}

// ---- filler ----

type fillerRun struct {
	PoolClips int         `json:"poolClips"`
	Mode      string      `json:"mode"`
	Viewing   string      `json:"viewing"`
	Metrics   kit.Metrics `json:"metrics"`
	Evening   kit.Metrics `json:"evening"`
}

// fillerSim assembles every break with the REAL filler.Assemble over a synthetic clip pool, two
// 2-minute breaks an hour, and compares exposure fed only while watched (today: RecordClipPlay
// runs when playout resolves a clip for a viewer) against a ledger fed by every assembled break.
func fillerSim(start, end time.Time) []fillerRun {
	var out []fillerRun
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Println()
	_, _ = fmt.Fprintln(tw, "clips\tmode\tviewing\tplays\tdistinct\tC60m\tC24h\tmax/day\teve C60m\teve C24h\t")
	for _, n := range []int{40, 150, 600} {
		clips := clipPool(n)
		for _, mode := range []string{"current", "ledger"} {
			for _, v := range viewings {
				if mode == "ledger" && v.Name != "nobody" {
					continue // the ledger does not depend on viewing
				}
				exp := map[string]filler.Exposure{}
				var airings []kit.Airing
				for t := start; t.Before(end); t = t.Add(30 * time.Minute) {
					w := filler.Window{ChannelID: "sim", Seed: seedOf(t), GapMs: 120000, PodMax: 4,
						Audience: filler.Audience("general"), Exposures: copyExp(exp), SnapshotAt: t}
					cooldown := 30 * time.Minute
					if mode == "ledger" {
						cooldown = 24 * time.Hour
					}
					pod := filler.Assemble(clips, w, filler.Policy{Cooldown: cooldown}, nil)
					a := kit.Airing{Kind: "filler", Start: t}
					for _, e := range pod.Entries {
						if e.Hash == "" {
							continue
						}
						a.Clips = append(a.Clips, e.Hash)
						if mode == "ledger" || v.Watch(t.In(simLoc)) {
							x := exp[e.Hash]
							x.PlayCount++
							x.LastPlayedAt = t
							exp[e.Hash] = x
						}
					}
					airings = append(airings, a)
				}
				m := kit.Measure(airings, 0, start, end)
				// What an evening viewer actually sees: only the breaks between 19:00 and 23:00.
				var eve []kit.Airing
				for _, a := range airings {
					if h := a.Start.In(simLoc).Hour(); h >= 19 && h < 23 {
						eve = append(eve, a)
					}
				}
				em := kit.Measure(eve, 0, start, end)
				out = append(out, fillerRun{PoolClips: n, Mode: mode, Viewing: v.Name, Metrics: m, Evening: em})
				_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%d\t%s\t%s\t%d\t%s\t%s\t\n", n, mode, v.Name, m.ClipPlays, m.DistinctClips,
					pc(m.ClipRepeat60m), pc(m.ClipRepeat24h), m.MaxClipPerDay, pc(em.ClipRepeat60m), pc(em.ClipRepeat24h))
			}
		}
	}
	_ = tw.Flush()
	return out
}

func clipPool(n int) []filler.Clip {
	r := rand.New(rand.NewSource(int64(n)))
	cats := []string{"cereal", "toy", "car", "soda", "snack", "store", "phone", "shoe"}
	out := make([]filler.Clip, n)
	for i := range out {
		out[i] = filler.Clip{
			Hash: fmt.Sprintf("%064x", i+1), Path: fmt.Sprintf("aa/bb/%064x.mp4", i+1), Name: fmt.Sprintf("clip %d", i),
			Kind: filler.Commercial, Placement: filler.PlacementBreakBody, Audience: filler.Audience("general"),
			DurationMs: int64(15+15*r.Intn(3)) * 1000, Category: cats[r.Intn(len(cats))],
		}
	}
	return out
}

func seedOf(t time.Time) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("sim|" + t.UTC().Format(time.RFC3339)))
	return int64(h.Sum64())
}

func copyExp(m map[string]filler.Exposure) map[string]filler.Exposure {
	out := make(map[string]filler.Exposure, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
