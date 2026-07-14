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
	LibraryItemID  string // "" for the embedded bumper-card fallback
	Name           string
	Kind           Kind
	DurationMs     int64
	IsFallbackCard bool // the embedded default bumper card (bottom of the ladder)
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
}

// Policy tunes assembly (from §15 FILLER_* + per-channel pod policy).
type Policy struct {
	// EraStrict: when true, never widen era beyond exact (the "strict" pod era
	// setting in the UI); when false, the ladder may widen (§10 fallback ladder).
	EraStrict bool
	// MinClipMs/MaxClipMs bound which clips are eligible (density, §10).
	MinClipMs int64
	MaxClipMs int64
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
// `used` is the set of LibraryItemIDs already played in the current window — the
// caller threads it across pods in a window for no-repeat-in-window (§10). Assemble
// adds the clips it uses to `used`.
func Assemble(catalog []Clip, w Window, policy Policy, used map[string]bool) Pod {
	if used == nil {
		used = map[string]bool{}
	}
	rng := rand.New(rand.NewSource(w.Seed))

	// Rank candidates by the fallback ladder, then fill the gap. The ladder is a
	// sequence of successively-looser candidate pools (§10); we take from the
	// tightest that still has clips.
	pools := candidatePools(catalog, w, policy)

	pod := Pod{MatchLevel: MatchBumperCard}
	// Intro bumper (best-effort — a matched bumper if we have one).
	if b, ok := pickBumper(catalog, w, used, rng); ok {
		pod.append(b, used)
	}

	// Fill the middle with matched commercials from the tightest non-empty pool,
	// enforcing category variety + no-repeat, up to PodMax and the gap.
	level, commercials := fillCommercials(pools, w, policy, used, rng)
	if level != "" {
		pod.MatchLevel = level
	}
	for _, c := range commercials {
		pod.append(clipToEntry(c), used)
	}

	// Return bumper.
	if b, ok := pickBumper(catalog, w, used, rng); ok {
		pod.append(b, used)
	}

	// Bottom of the ladder: if nothing matched (no commercials placed), the pod is
	// just the embedded card so the break is never empty (§10 never dead air).
	if len(commercials) == 0 {
		pod.append(FallbackCard, used)
		pod.MatchLevel = MatchBumperCard
	}
	return pod
}

func (p *Pod) append(e PodEntry, used map[string]bool) {
	p.Entries = append(p.Entries, e)
	p.TotalMs += e.DurationMs
	if e.LibraryItemID != "" {
		used[e.LibraryItemID] = true
	}
}

func clipToEntry(c Clip) PodEntry {
	return PodEntry{LibraryItemID: c.LibraryItemID, Name: c.Name, Kind: c.Kind, DurationMs: c.DurationMs}
}

// pickBumper chooses a bumper/station-id for a pod bookend, preferring era match,
// avoiding no-repeat, deterministic under rng. Returns ok=false if none exist.
func pickBumper(catalog []Clip, w Window, used map[string]bool, rng *rand.Rand) (PodEntry, bool) {
	var bumpers []Clip
	for _, c := range catalog {
		if c.IsBumper() && !used[c.LibraryItemID] {
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
	sort.Slice(bumpers, func(i, j int) bool { return bumpers[i].LibraryItemID < bumpers[j].LibraryItemID })
	return clipToEntry(bumpers[rng.Intn(len(bumpers))]), true
}
