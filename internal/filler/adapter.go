package filler

import (
	"context"
	"log/slog"
	"sort"

	"github.com/loomarr/loomarr/internal/metrics"
)

// PodAdapter implements the scheduler's PodFiller port (§10): it loads the clip
// catalog, assembles a matched clip pool for a channel (era/audience-matched,
// seed-deterministic), and returns the clips' Tunarr program uuids for the channel
// filler-list the reconcile engine attaches. It bridges the pure Assemble() to the
// store-backed catalog — keeping the filler domain and the scheduler free of each
// other's concerns. Since Tunarr plays the filler-list into ANY flex gap, this is
// a per-channel pool (not a per-gap sequence): the scheduler no longer sizes pods
// to individual gaps.
type PodAdapter struct {
	catalog CatalogReader
	// policy is RESOLVED PER CALL, not captured once.
	//
	// ⚠ **It was a plain `Policy` value, and that quietly broke every filler setting's
	// hot-apply.** `app.go` builds the struct at boot with `set.intv(…)`/`set.dur(…)`, so the
	// values were a snapshot: changing `filler.pod_max`, `filler.min_quality` or (from V51f)
	// `filler.min_clip_duration`/`max_clip_duration` did nothing until the process restarted.
	// The setting persisted, the API read it back, and the assembler kept using the old number.
	//
	// ⚠ **No test could see it, and that is structural rather than an oversight.** Every unit
	// test constructs a `Policy` literal and calls `Assemble`/`Coverage`/`PoolCounts` directly,
	// so they exercise the FIELD and never the wiring from a setting to it. Found on the live
	// stack: `filler.max_clip_duration=45s` left `PoolReport.Eligible` at 20 with a 64s clip in
	// the catalog, and it dropped to 19 only after a restart.
	//
	// config-design §3 defines hot-apply for long-lived clients, and `AutoSplitPolicy` in this
	// package already resolves per call for exactly this reason. This is the same fix applied one
	// layer out: the DOMAIN functions still take a plain `Policy` value — they are pure and their
	// tests stay literal — and only the boundary that owns a settings service resolves it.
	policy func() Policy
	log    *slog.Logger
}

// pol resolves the current policy. A nil resolver means "no policy" (the zero value), which is
// what the majority of callers — tests and installs with nothing configured — actually want.
func (a *PodAdapter) pol() Policy {
	if a.policy == nil {
		return Policy{}
	}
	return a.policy()
}

// CatalogReader loads clips for pod assembly (implemented by the store).
type CatalogReader interface {
	// AllClips returns the full filler catalog for matching.
	AllClips(ctx context.Context) ([]Clip, error)
}

// Selection is a channel's per-channel filler choice in filler-native terms (§10) — the
// projection of schedule.FillerSelection that assembly needs. Callers (channels/app,
// which know both packages) translate the policy type to this at the boundary, keeping
// the filler domain free of a schedule dependency. All fields optional; the zero
// Selection is the whole catalog (the prior global behaviour).
type Selection struct {
	Era        EraRange // zero value = any; the inherit-vs-any decision is made before this point
	Audience   Audience // "" = any
	Categories []string // empty = any
	Kinds      []string // empty = the default kinds
	Pinned     []string // clip ids always included
	Excluded   []string // clip ids never used
	// BreakDurationMs is a per-channel override. Zero inherits Policy.BreakDurationMs.
	BreakDurationMs int64
}

// NewPodAdapter builds the scheduler-facing pod assembler.
//
// ⚠ `policy` is a RESOLVER, called on every assembly, so a settings change takes effect on the
// next pod rather than at the next restart. Pass nil for "no policy" — the zero value.
func NewPodAdapter(catalog CatalogReader, policy func() Policy, log *slog.Logger) *PodAdapter {
	return &PodAdapter{catalog: catalog, policy: policy, log: log}
}

// poolGapMs is the notional window Assemble fills to size the channel's clip pool.
// A channel's filler-list is a POOL Tunarr draws from, not a single sized break,
// so we assemble a generous pool (~one long break's worth); Tunarr picks per gap.
const poolGapMs int64 = 600_000 // 10 min of clips

const defaultBreakDurationMs int64 = 300_000 // 5 min; mirrors filler.break_duration

// BuildFillerList implements channels.PodFiller: assemble a matched clip pool for a
// channel and return the clips' Tunarr program uuids for its filler-list. ok=false
// on any error or an empty/all-fallback pool, so the reconcile skips the attach and
// the channel's flex falls back to the bumper card — never dead air (§10, §9). The
// pool is seed-deterministic (same catalog + seed → same uuids), so a re-reconcile
// produces the same list (idempotency at the EnsureFillerList layer, §9).
// HasPool reports whether the channel has any playable commercials at all.
//
// Backend-INDEPENDENT, and that is the point (§9.1): it asks "are there clips to fill a break
// with", which decides whether the scheduler interleaves breaks. BuildFillerList answers the
// narrower Tunarr question — "which program uuids go in a filler-list" — and using it as this
// gate meant an install with no Tunarr assembled a perfect pod and then got no breaks, because
// none of its clips carried a uuid.
//
// A pod of ONLY the embedded fallback card is not a pool: there is nothing to play, and
// promising breaks would render as empty channel-named blocks in the guide (§10).
func (a *PodAdapter) HasPool(ctx context.Context, channelID string, seed int64, sel Selection) bool {
	pod, err := a.Preview(ctx, channelID, seed, sel)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler pool check failed (channel stays break-free)", "channel", channelID, "err", err)
		}
		return false
	}
	for _, e := range pod.Entries {
		// A real file, whichever backend plays it. The fallback card has no Path.
		if e.Path != "" {
			return true
		}
	}
	return false
}

// PlayableDurationMs reports the duration of real files in the deterministic pod.
// The embedded fallback card deliberately contributes nothing: it is generated at
// playout time and must not extend a commercial break after the selected clips end.
func (a *PodAdapter) PlayableDurationMs(ctx context.Context, channelID string, seed int64, sel Selection) int64 {
	pod, err := a.Preview(ctx, channelID, seed, sel)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler duration check failed (channel stays break-free)", "channel", channelID, "err", err)
		}
		return 0
	}
	var total int64
	for _, entry := range pod.Entries {
		if entry.Path != "" && entry.DurationMs > 0 {
			total += entry.DurationMs
		}
	}
	return total
}

func (a *PodAdapter) BuildFillerList(ctx context.Context, channelID string, seed int64, sel Selection) ([]string, bool) {
	pod, err := a.Preview(ctx, channelID, seed, sel)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler list: load catalog failed (channel stays flex)", "channel", channelID, "err", err)
		}
		return nil, false
	}
	// §17 fallback-ladder depth: record the rung this reconcile's pod reached.
	// Recorded here (the attach path) not in Preview, so UI previews don't inflate
	// it; skipped when the catalog is empty (a zero-value MatchLevel).
	if pod.MatchLevel != "" {
		metrics.FillerPodAssembled(string(pod.MatchLevel))
	}
	ids := make([]string, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		if e.TunarrProgramID == "" {
			continue // the embedded bumper-card fallback isn't a real Tunarr program
		}
		ids = append(ids, e.TunarrProgramID)
	}
	if len(ids) == 0 {
		return nil, false // only the fallback card → nothing to attach; flex
	}
	return ids, true
}

// Preview assembles the pool a channel WOULD get, without touching Tunarr (§12: the
// Filler view previews a channel's pods). It is the SAME call BuildFillerList makes —
// deliberately one code path, because a preview that diverges from what reconcile
// actually attaches is worse than no preview: it would confidently show pods the
// operator never receives.
//
// Assemble is pure and seeded-deterministic, so preview and the next reconcile of the
// same channel produce identical output. Returns an empty pod (not an error) when the
// catalog is empty — "no clips yet" is a normal state the UI renders, not a failure.
func (a *PodAdapter) Preview(ctx context.Context, channelID string, seed int64, sel Selection) (Pod, error) {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return Pod{}, err
	}
	if len(clips) == 0 {
		return Pod{}, nil
	}
	// ⚠ Resolved ONCE and reused below. Calling the resolver twice in one assembly could read
	// two different snapshots if a setting is written between them — a pod sized by one policy
	// and filled by another, which is a bug that would appear only under a concurrent write.
	pol := a.pol()
	breakDurationMs := effectiveBreakDurationMs(sel, pol)
	podMax := pol.PodMax
	if podMax <= 0 {
		podMax = 4 // matches fillCommercials' own fallback (the setting default is 4)
	}
	// The window IS the per-channel selection (§10): era/audience narrow the ladder,
	// categories/kinds narrow the catalog, pinned/excluded are the overrides. An empty
	// Selection leaves every field at its zero "any" value → the whole catalog, which is
	// the prior behaviour (and the additive default for a channel with no filler choice).
	w := a.windowFor(channelID, seed, sel, podMax, breakDurationMs)
	w.PodMax = podMaxForDuration(clips, w, pol, podMax, breakDurationMs)
	return Assemble(clips, w, pol, map[string]bool{}), nil
}

// effectiveBreakDurationMs resolves the per-channel override over the live global setting.
// A zero is inheritance, never "off" — breaks_per_hour owns the off switch. Invalid legacy
// values also fall back to the documented 5 minute default rather than recreating Tunarr's
// silent 30 second clamp and making Loomarr's preview disagree with playout.
func effectiveBreakDurationMs(sel Selection, pol Policy) int64 {
	if sel.BreakDurationMs >= 30_000 {
		return sel.BreakDurationMs
	}
	if pol.BreakDurationMs >= 30_000 {
		return pol.BreakDurationMs
	}
	return defaultBreakDurationMs
}

// podMaxForDuration makes pod_max a soft density ceiling when it is too small to plausibly
// fill the requested break. It derives the required count from the median duration of the
// tightest non-empty matching pool, so a 30s catalog gets about ten slots for a 5m break while
// a 60s catalog gets about five. Assemble still owns the hard no-repeat and gap invariants.
func podMaxForDuration(clips []Clip, w Window, pol Policy, configured int, targetMs int64) int {
	var matched []Clip
	for _, pool := range candidatePools(clips, w, pol) {
		if len(pool.clips) > 0 {
			matched = pool.clips
			break
		}
	}
	if len(matched) == 0 {
		return configured
	}

	durations := make([]int64, 0, len(matched))
	for _, c := range matched {
		if c.DurationMs > 0 {
			durations = append(durations, c.DurationMs)
		}
	}
	if len(durations) == 0 {
		return configured
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	median := durations[len(durations)/2]
	required := int((targetMs + median - 1) / median)
	if required > configured {
		return required
	}
	return configured
}

// windowFor builds the Window a channel's selection describes.
//
// Extracted so Preview and Coverage cannot disagree about what a channel's selection MEANS.
// They already share the ladder (both end up in `candidatePools`); this is the step before it,
// and duplicating it is how a meter ends up reporting on a slightly different window than the
// pod it claims to describe — an off-by-one-field bug that no ladder test would catch.
func (a *PodAdapter) windowFor(channelID string, seed int64, sel Selection, podMax int, breakDurationMs int64) Window {
	gapMs := poolGapMs
	if breakDurationMs > gapMs {
		gapMs = breakDurationMs
	}
	return Window{
		ChannelID: channelID, Seed: seed,
		Era: sel.Era, Audience: sel.Audience,
		GapMs: gapMs, PodMax: podMax, BreakDurationMs: breakDurationMs,
		Categories: sel.Categories, Kinds: sel.Kinds,
		Pinned: sel.Pinned, Excluded: sel.Excluded,
	}
}

// CoverageFor reports which ladder rung this channel's breaks would draw from (V29a/V29b-api).
//
// The same catalog load, the same Window, the same policy as Preview — only the question
// differs: Preview draws a pod, this counts what is available to draw from. A seed is
// deliberately NOT taken: coverage is a property of the catalog and the selection, not of any
// one break, so passing a seed would imply an answer that varies per pod when it does not.
func (a *PodAdapter) CoverageFor(ctx context.Context, channelID string, sel Selection) (CoverageReport, error) {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return CoverageReport{}, err
	}
	return a.CoverageFrom(clips, channelID, sel), nil
}

// Catalog loads the full clip catalog once, for a caller that will then ask several
// catalog-shaped questions of it (the pool report asks one per live channel). Pairs with
// CoverageFrom: load here, answer there.
func (a *PodAdapter) Catalog(ctx context.Context) ([]Clip, error) { return a.catalog.AllClips(ctx) }

// CoverageFrom is CoverageFor over a catalog the caller already holds — the same derivation,
// the same window, the same answer, minus the load.
//
// ⚠ The aggregate report (the Filler pool strip) asks this question once per live channel. Going
// through CoverageFor there made an N-channel install an N-catalog-load request, which is the
// exact cost `FitForChannel` above already refuses for the same reason. What must NOT change is
// that every caller gets its number from THIS function: the pool strip and the channel page are
// compared by operators when something looks wrong, and a second implementation of the ladder
// would eventually let them disagree. Sharing the computation is the invariant; re-reading the
// catalog was never part of it.
func (a *PodAdapter) CoverageFrom(clips []Clip, channelID string, sel Selection) CoverageReport {
	pol := a.pol()
	breakDurationMs := effectiveBreakDurationMs(sel, pol)
	podMax := pol.PodMax
	if podMax <= 0 {
		podMax = 4
	}
	// Seed 0: unused by Coverage (it never draws), and stated here rather than left implicit
	// so nobody threads a real seed through expecting it to matter.
	w := a.windowFor(channelID, 0, sel, podMax, breakDurationMs)
	w.PodMax = podMaxForDuration(clips, w, pol, podMax, breakDurationMs)
	return Coverage(clips, w, pol)
}

// FitForChannel reports how ONE clip relates to one channel's selection (§10 V35 item 1.7).
//
// The same `windowFor` derivation Preview and CoverageFor use, so the picker's per-channel note
// cannot describe a different window than the pods it is talking about. Takes the clip rather
// than loading the catalog: the caller (the picker) already has it, and re-reading the whole
// catalog once per channel row would make an N-channel picker an N-catalog-load operation.
func (a *PodAdapter) FitForChannel(channelID string, sel Selection, c Clip) Fit {
	pol := a.pol()
	breakDurationMs := effectiveBreakDurationMs(sel, pol)
	podMax := pol.PodMax
	if podMax <= 0 {
		podMax = 4
	}
	// Seed 0, for CoverageFor's reason: fit is a property of the clip and the selection, never
	// of one break, so a real seed would imply an answer that varies per pod.
	return FitFor(c, a.windowFor(channelID, 0, sel, podMax, breakDurationMs), pol)
}

// PoolCounts reports the catalog-wide half of the pool strip (§10 V35) — how much material
// exists at all, before any channel's selection narrows it.
//
// The channel-shaped half is assembled by the caller from CoverageFor, one call per live
// channel, so this adds no second opinion about matching. It only counts.
func (a *PodAdapter) PoolCounts(ctx context.Context) (PoolReport, error) {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return PoolReport{}, err
	}
	return PoolCounts(clips, a.pol()), nil
}

var _ interface {
	BuildFillerList(ctx context.Context, channelID string, seed int64, sel Selection) ([]string, bool)
} = (*PodAdapter)(nil)
