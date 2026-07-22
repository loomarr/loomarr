package filler

import (
	"math/rand"
	"sort"
)

// Pod is an assembled ad break (§10): an intro bumper → matched commercials →
// return bumper, sized to a flex gap. It's what the scheduler inserts between
// programs via Tunarr flex + filler lists. Every entry is a real catalog clip
// (grounding: §10) or the embedded bumper-card fallback.
type Pod struct {
	Entries    []PodEntry
	TotalMs    int64
	MatchLevel MatchLevel // how far down the fallback ladder we went (§10)
}

// PodEntry is one clip placed in a pod, in play order.
type PodEntry struct {
	TunarrProgramID string // "" for the embedded bumper-card fallback
	Name            string
	Kind            Kind
	DurationMs      int64
	IsFallbackCard  bool // the embedded default bumper card (bottom of the ladder)
}

// MatchLevel records how the pod was filled — the fallback ladder rung reached
// (§10). Surfaced so the UI can show "matched" vs "fallback-widened" vs
// "bumper-card-only" (the PodTimeline states in the frontend design).
type MatchLevel string

const (
	MatchExact      MatchLevel = "exact"       // exact-era + audience matches filled the pod
	MatchWidened    MatchLevel = "widened"     // era widened to the decade / any-era to fill
	MatchAudience   MatchLevel = "audience"    // any appropriate-audience clip, era ignored
	MatchBumperCard MatchLevel = "bumper_card" // only the embedded fallback card — never dead air
)

// Window describes the break to fill (§10). It carries the deterministic seed
// material (channel + window) so the same break rebuilds identically.
type Window struct {
	ChannelID string
	// Seed derives from channel + window start so pod assembly is reproducible
	// (§10/§19 seeded-deterministic). Tests pass a fixed seed.
	Seed int64
	// Era + Audience are the block's target (90s cartoons → 1990s + kids).
	Era      int
	Audience Audience
	// GapMs is the flex gap to fill.
	GapMs int64
	// BreaksMax caps clips per pod (FILLER_POD_MAX, §15).
	PodMax int
	// The per-channel selection (§10 FillerSelection), all optional:
	//   Categories — commercial-pool category narrowing (empty = any).
	//   Kinds      — which clip kinds the pod may use (empty = the default set);
	//                applied catalog-wide in Assemble, so it shapes bumpers too.
	//   Pinned     — clip ids to force in as a top-priority pool (before the ladder).
	//   Excluded   — clip ids never used; threaded into `used` by the caller/Assemble.
	Categories []string
	Kinds      []string
	Pinned     []string
	Excluded   []string
}

// Policy tunes assembly (from §15 FILLER_* + per-channel pod policy).
type Policy struct {
	// EraStrict: when true, never widen era beyond exact (the "strict" pod era
	// setting in the UI); when false, the ladder may widen (§10 fallback ladder).
	EraStrict bool
	// MinClipMs/MaxClipMs bound which clips are eligible (density, §10).
	MinClipMs int64
	MaxClipMs int64
	// PodMax caps clips per pod (FILLER_POD_MAX, §15). 0 ⇒ a sane default (fillCommercials
	// falls back to 4). Wired from the setting so preview and reconcile agree — previously
	// the adapter hardcoded 32, ignoring the knob.
	PodMax int
}

// FallbackCard is the embedded default bumper-card asset (§10): Loomarr ships one
// and sets it as each channel's Tunarr fallback at creation, so the bottom of the
// ladder exists on day one. Operators can replace it per channel.
var FallbackCard = PodEntry{
	Name:           "Loomarr — We'll be right back",
	Kind:           Bumper,
	DurationMs:     5000, // 5s card
	IsFallbackCard: true,
}

// Assemble builds a pod for a window from the catalog (§10). It is PURE and
// SEEDED-DETERMINISTIC: same clips + same window.Seed → identical pod, so a break
// reproduces exactly across reconciles and in tests (§19). It respects era/
// audience matching, category variety (no three of one category in a row), pod
// density (PodMax + the gap), and no-repeat within the window; when matches run
// out it descends the fallback ladder, ending at the embedded bumper card so a
// pod is never empty ("never dead air", §10).
//
// `used` is the set of TunarrProgramIDs already played in the current window — the
// caller threads it across pods in a window for no-repeat-in-window (§10). Assemble
// adds the clips it uses to `used`.
func Assemble(catalog []Clip, w Window, policy Policy, used map[string]bool) Pod {
	if used == nil {
		used = map[string]bool{}
	}
	rng := rand.New(rand.NewSource(w.Seed))

	// Per-channel selection (§10), all no-ops when unset:
	//  - EXCLUDE: seed the no-repeat set with the excluded ids. Because `used` is
	//    already checked as an exclusion at every pick site (bumpers + every ladder
	//    rung + pin), this removes them everywhere with no other change. Exclude thus
	//    also WINS over pin (a pinned-and-excluded id is `used` before pin runs).
	for _, id := range w.Excluded {
		used[id] = true
	}
	//  - KINDS: narrow the catalog to the selected kinds before anything picks from
	//    it, so the choice shapes bumpers (pickBumper) as well as the commercial fill.
	catalog = filterKinds(catalog, w.Kinds)

	pod := Pod{MatchLevel: MatchBumperCard}

	// Intro bumper (best-effort — a matched bumper if we have one).
	if b, ok := pickBumper(catalog, w, used, rng); ok {
		pod.append(b, used)
	}

	// PIN: force the channel's pinned clips in first, as a top-priority pool ahead of
	// the ladder (the one thing the ranking ladder can't express). Pins still respect
	// the pod budget — a pin can't make an unbounded pod — and skip anything already
	// `used` (i.e. excluded or a pinned bumper already placed).
	pinned := pickPinned(catalog, w, policy, used)
	for _, c := range pinned {
		pod.append(clipToEntry(c), used)
	}

	// Fill the rest with matched commercials from the tightest non-empty pool,
	// enforcing category variety + no-repeat, up to PodMax and the gap. The pins
	// already consumed some of PodMax/GapMs (they're in `used` + counted in the pod's
	// TotalMs), so the remaining budget is what's left. If the pins already filled the
	// quota (remaining PodMax ≤ 0), skip the fill entirely — otherwise fillCommercials'
	// own "≤0 ⇒ default" fallback would treat an exhausted quota as "unset" and overfill.
	var level MatchLevel
	var commercials []Clip
	if remainMax := w.PodMax - len(pinned); remainMax > 0 {
		remaining := w
		remaining.PodMax = remainMax
		remaining.GapMs = w.GapMs - pod.TotalMs
		pools := candidatePools(catalog, remaining, policy)
		level, commercials = fillCommercials(pools, remaining, policy, used, rng)
		if level != "" {
			pod.MatchLevel = level
		}
		for _, c := range commercials {
			pod.append(clipToEntry(c), used)
		}
	}

	// Return bumper.
	if b, ok := pickBumper(catalog, w, used, rng); ok {
		pod.append(b, used)
	}

	// Bottom of the ladder: if NOTHING content-bearing matched (no pins, no
	// commercials), the pod is just the embedded card so the break is never empty
	// (§10 never dead air). Pins alone count as content — a pinned-only pod is a real
	// pod, not the fallback.
	if len(pinned) == 0 && len(commercials) == 0 {
		pod.append(FallbackCard, used)
		pod.MatchLevel = MatchBumperCard
	} else if level == "" && len(pinned) > 0 {
		// Pins placed but no laddered commercials filled — the pod IS the pins (plus
		// bumpers). Report an exact match: the operator got precisely what they pinned.
		pod.MatchLevel = MatchExact
	}
	return pod
}

// pickPinned returns the channel's pinned clips that exist in the (kind-filtered)
// catalog, pass the duration policy, aren't already `used` (excluded / already
// placed), and fit the pod budget — placed in the operator's pinned order (stable,
// not shuffled: a pin is an explicit choice, so honor its order). Pins bypass the
// era/audience/category ladder entirely (that's the point of pinning), but still
// respect PodMax and the gap so a long pin list can't produce an unbounded pod.
func pickPinned(catalog []Clip, w Window, policy Policy, used map[string]bool) []Clip {
	if len(w.Pinned) == 0 {
		return nil
	}
	byID := make(map[string]Clip, len(catalog))
	for _, c := range catalog {
		byID[c.TunarrProgramID] = c
	}
	budget := w.GapMs - 12000 // same bumper headroom as fillCommercials
	if budget < 0 {
		budget = w.GapMs
	}
	var out []Clip
	var totalMs int64
	for _, id := range w.Pinned {
		if len(out) >= w.PodMax {
			break
		}
		c, ok := byID[id]
		if !ok || used[id] || !durationEligible(c, policy) {
			continue
		}
		if totalMs+c.DurationMs > budget && len(out) > 0 {
			continue
		}
		out = append(out, c)
		totalMs += c.DurationMs
		used[id] = true // reserve so the ladder fill + return bumper don't reuse it
	}
	return out
}

func (p *Pod) append(e PodEntry, used map[string]bool) {
	p.Entries = append(p.Entries, e)
	p.TotalMs += e.DurationMs
	if e.TunarrProgramID != "" {
		used[e.TunarrProgramID] = true
	}
}

func clipToEntry(c Clip) PodEntry {
	return PodEntry{TunarrProgramID: c.TunarrProgramID, Name: c.Name, Kind: c.Kind, DurationMs: c.DurationMs}
}

// pickBumper chooses a bumper/station-id for a pod bookend, preferring era match,
// avoiding no-repeat, deterministic under rng. Returns ok=false if none exist.
func pickBumper(catalog []Clip, w Window, used map[string]bool, rng *rand.Rand) (PodEntry, bool) {
	var bumpers []Clip
	for _, c := range catalog {
		if c.IsBumper() && !used[c.TunarrProgramID] {
			bumpers = append(bumpers, c)
		}
	}
	if len(bumpers) == 0 {
		return PodEntry{}, false
	}
	// Prefer era-matching bumpers; fall back to any bumper.
	era := filterEra(bumpers, w.Era)
	if len(era) > 0 {
		bumpers = era
	}
	sort.Slice(bumpers, func(i, j int) bool { return bumpers[i].TunarrProgramID < bumpers[j].TunarrProgramID })
	return clipToEntry(bumpers[rng.Intn(len(bumpers))]), true
}
