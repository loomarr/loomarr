package filler

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
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
	airings AiringReader
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
	policy  func() Policy
	log     *slog.Logger
	metrics PodMetrics
	// now dates the season of a question asked without a break start (seasonAt).
	now func() time.Time
}

// PodMetrics observes only attached pods; previews deliberately do not count.
type PodMetrics interface {
	FillerPodAssembled(matchLevel string)
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

// AiringReader is the durable airing-history seam used only for actual break planning. Keeping it
// separate from CatalogReader makes previews/tests that intentionally have no history explicit,
// and prevents assembly from acquiring any write capability.
//
// ⚠ The history is the HOUSEHOLD's, not one channel's: a commercial seen on any channel rests on
// every channel (#2017). Per-channel history let one TV switching channels see the same clips in
// the same order on two channels within the hour.
type AiringReader interface {
	FillerAirings(ctx context.Context) (AiringHistory, error)
}

// Selection is a channel's per-channel filler choice in filler-native terms (§10) — the
// projection of schedule.FillerSelection that assembly needs. Callers (channels/app,
// which know both packages) translate the policy type to this at the boundary, keeping
// the filler domain free of a schedule dependency. All fields optional; the zero
// Selection is the whole catalog (the prior global behaviour).
type Selection struct {
	Era        EraRange   // zero value = any; the inherit-vs-any decision is made before this point
	EraWindows []EraRange // normalized union; when present, authoritative over Era
	Audience   Audience   // "" = any
	Categories []string   // empty = any
	Kinds      []string   // empty = the default kinds (all but political and news)
	Pinned     []string   // clip ids always included
	Excluded   []string   // clip ids never used
	Geography  Geography  // zero country inherits Installation geography from Policy
	Language   string     // the channel's commercial language; "" inherits Policy.Language
	// BreakDurationMs is a per-channel override. Zero inherits Policy.BreakDurationMs.
	BreakDurationMs int64
	// Spacing is the per-channel repeat spacing for one commercial. Nil inherits Policy.Cooldown;
	// a present zero is real ("may repeat in any later break"), hence the pointer.
	Spacing *time.Duration
}

// NewPodAdapter builds the scheduler-facing pod assembler.
//
// ⚠ `policy` is a RESOLVER, called on every assembly, so a settings change takes effect on the
// next pod rather than at the next restart. Pass nil for "no policy" — the zero value.
func NewPodAdapter(catalog CatalogReader, airings AiringReader, policy func() Policy, log *slog.Logger) *PodAdapter {
	return &PodAdapter{catalog: catalog, airings: airings, policy: policy, log: log, now: time.Now}
}

// WithClock replaces the wall clock seasonAt falls back to.
func (a *PodAdapter) WithClock(now func() time.Time) *PodAdapter {
	a.now = now
	return a
}

// seasonAt is the household calendar's season for a break starting at t. A question with no
// break start (the reconciler's pool check, a Tunarr filler list, coverage, a clip's fit) is
// answered for now, as programming's seasonality is: the periodic sweep rolls it with the date.
func (a *PodAdapter) seasonAt(pol Policy, t time.Time) schedule.HolidaySeason {
	if t.IsZero() {
		t = a.now()
	}
	return pol.Calendar.SeasonAt(t)
}

// WithMetrics binds generation-scoped observations without coupling filler to Prometheus.
func (a *PodAdapter) WithMetrics(observer PodMetrics) *PodAdapter {
	a.metrics = observer
	return a
}

const defaultBreakDurationMs int64 = 120_000 // 2m; mirrors filler.break_duration

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
// A pod no ad fits is not a pool. The embedded fallback card alone has nothing to play, and
// promising breaks would render as empty channel-named blocks in the guide (§10). Station material
// alone is not a pool either: it covers a break whose ads are all resting (#2036), and is no reason
// to give breaks to a channel with no ad, such as a Christmas-only catalog in July (#2030).
func (a *PodAdapter) HasPool(ctx context.Context, channelID string, seed int64, sel Selection) bool {
	pod, err := a.Preview(ctx, channelID, seed, sel)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler pool check failed (channel stays break-free)", "channel", channelID, "err", err)
		}
		return false
	}
	if pod.MatchLevel == MatchBumperCard {
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

// PlayableDurationMs reports the duration of real files in the deterministic pod: the length
// reconcile gives every break on the channel's timeline. The embedded fallback card deliberately
// contributes nothing: it is generated at playout time and must not extend a commercial break
// after the selected clips end.
//
// ⚠ It is sized WITHOUT the play history, so only the catalog, the selection and the policy move
// it. Sized from the live rotation, every commercial that aired re-sized every break, the
// timeline walked from the anchor shifted by tens of seconds or minutes at each reconcile, and
// the viewer was thrown back into the programme or into the same break again (live smoke,
// 2026-10-24). Rotation still decides WHICH clips air: PreviewAt assembles each break to this
// length from its own history snapshot. Holiday seasons are left out for the same reason: a
// season opening at midnight must not re-size every break (#2030).
func (a *PodAdapter) PlayableDurationMs(ctx context.Context, channelID string, seed int64, sel Selection) int64 {
	clips, err := a.catalog.AllClips(ctx)
	var pod Pod
	if err == nil && len(clips) > 0 {
		pod = a.assemble(clips, nil, channelID, Break{Seed: seed, yearRound: true}, sel, a.pol())
	}
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
		if a.metrics != nil {
			a.metrics.FillerPodAssembled(string(pod.MatchLevel))
		}
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
	return a.previewAt(ctx, channelID, seed, sel, time.Time{})
}

// PreviewAt assembles an actual break from an immutable history snapshot strictly before its
// start. Actual clip-start writes during the break therefore cannot reshuffle the remaining pod.
func (a *PodAdapter) PreviewAt(ctx context.Context, channelID string, seed int64, sel Selection, breakStart time.Time) (Pod, error) {
	return a.previewAt(ctx, channelID, seed, sel, breakStart)
}

// BreakSlot is one break on a channel's timeline as its callers know it: when it starts and how
// long its scheduled slot is. The seed is derived from the start by whoever owns the channel.
type BreakSlot struct {
	Start  time.Time
	Length time.Duration
}

// Break names one break to assemble: its start, the seed derived from it, and the length of its
// scheduled slot. A zero Length is the configured break length (a break not yet on a timeline).
type Break struct {
	Seed   int64
	Start  time.Time
	Length time.Duration
	// yearRound assembles with no holiday seasons, every clip year-round: PlayableDurationMs sizes
	// breaks with it so a season opening or closing never re-sizes the timeline.
	yearRound bool
}

// PreviewAtMany assembles several breaks of one channel — the guide's per-airing hover cards
// (#1420). Each pod is exactly what PreviewAt returns for that break; the catalog, the policy and
// the household's airing history are read ONCE and each break's snapshot is cut from them in
// memory. It used to be a catalog read plus an exposure query per break, 40-80 full-catalog reads
// for a four-hour guide once breaks were on.
func (a *PodAdapter) PreviewAtMany(ctx context.Context, channelID string, sel Selection, breaks []Break) ([]Pod, error) {
	if len(breaks) == 0 {
		return nil, nil
	}
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return nil, err
	}
	pods := make([]Pod, len(breaks))
	if len(clips) == 0 {
		return pods, nil
	}
	var history *AiringHistory
	if a.airings != nil {
		h, err := a.airings.FillerAirings(ctx)
		if err != nil {
			return nil, err
		}
		history = &h
	}
	// ⚠ Resolved ONCE for the whole batch, for the same reason assemble resolves it once per pod:
	// a concurrent settings write must not size some breaks by one policy and some by another.
	pol := a.pol()
	for i, b := range breaks {
		pods[i] = a.assemble(clips, history, channelID, b, sel, pol)
	}
	return pods, nil
}

func (a *PodAdapter) previewAt(ctx context.Context, channelID string, seed int64, sel Selection, breakStart time.Time) (Pod, error) {
	pods, err := a.PreviewAtMany(ctx, channelID, sel, []Break{{Seed: seed, Start: breakStart}})
	if err != nil {
		return Pod{}, err
	}
	return pods[0], nil
}

// assemble is the one per-break assembly every preview path runs, over inputs already loaded. A
// break on a timeline is assembled to its slot's own length, which is the hard budget: the slot
// may be shorter than the configured break (reconcile caps it at the playable pod), and a pod
// assembled to the setting would run past the slot's end and lose its last ad mid-spot.
func (a *PodAdapter) assemble(clips []Clip, history *AiringHistory, channelID string, b Break, sel Selection, pol Policy) Pod {
	seed, breakStart := b.Seed, b.Start
	breakDurationMs := effectiveBreakDurationMs(sel, pol)
	if b.Length > 0 {
		breakDurationMs = b.Length.Milliseconds()
	}
	if sel.Spacing != nil && *sel.Spacing >= 0 {
		pol.Cooldown = *sel.Spacing
	}
	podMax := pol.PodMax
	if podMax <= 0 {
		podMax = 4 // matches fillCommercials' own fallback (the setting default is 4)
	}
	// The window IS the per-channel selection (§10): era/audience narrow the ladder,
	// categories/kinds narrow the catalog, pinned/excluded are the overrides. An empty
	// Selection leaves every field at its zero "any" value → the whole catalog, which is
	// the prior behaviour (and the additive default for a channel with no filler choice).
	var season schedule.HolidaySeason
	if !b.yearRound {
		season = a.seasonAt(pol, breakStart)
	}
	w := a.windowFor(channelID, seed, sel, podMax, breakDurationMs, season)
	if history != nil {
		w.Exposures = ExposuresBefore(*history, breakStart)
		w.SnapshotAt = breakStart
	}
	w.PodMax = podMaxForDuration(clips, w, pol, podMax, breakDurationMs)
	return Assemble(clips, w, pol, map[string]bool{})
}

// effectiveBreakDurationMs resolves the per-channel override over the live global setting.
// A zero is inheritance, never "off" — breaks_per_hour owns the off switch. Invalid legacy
// values also fall back to the documented 2 minute default rather than recreating Tunarr's
// silent 30 second clamp and making Loomarr's preview disagree with playout.
func effectiveBreakDurationMs(sel Selection, pol Policy) int64 {
	return ResolveBreakDurationMs(sel.BreakDurationMs, pol.BreakDurationMs)
}

// ResolveBreakDurationMs is the ONE rule for how long a break is: the channel's own length when it
// is valid, else the install setting when that is, else the 2 minute default the setting ships
// with. Reconcile cuts the slot with it and the pod is assembled to it, so the two cannot differ.
//
// ⚠ It replaces two functions that disagreed about the fallback — the scheduler's said 5 minutes
// and the assembler's 2 — so a nonsense value cut a 5 minute slot for a 2 minute pod. Disabling
// breaks belongs to breaks-per-hour, never to a short length.
func ResolveBreakDurationMs(channelMs, globalMs int64) int64 {
	if channelMs >= minBreakDurationMs {
		return channelMs
	}
	if globalMs >= minBreakDurationMs {
		return globalMs
	}
	return defaultBreakDurationMs
}

const minBreakDurationMs int64 = 30_000

// podMaxForDuration makes pod_max a floor, never the reason a clip that fits is left out. The
// count a break may need is bounded by its time over the SHORTEST eligible clip, so that is the
// ceiling (up to maxPodClips); pod_max only raises it, for a catalog too sparse to ask for more.
// Assemble still owns the hard no-repeat and gap invariants — time, not count, bounds the pod.
//
// ⚠ It used to size from the MEDIAN clip. That left a fitting short clip out of about 1% of
// breaks whenever the pool mixed lengths, and the household had asked for fuller breaks.
//
// ⚠ The widest rung, because that is what a pod draws from since #1684 (the rungs nest, so it
// holds every candidate). Sizing from the tightest rung let one long exact-era clip set the
// density for a break filled mostly from the rungs below it.
//
// The count is deliberately NOT reduced for the bumpers: they come off the time budget in
// Assemble, and what they leave is exactly where a short whole clip goes.
func podMaxForDuration(clips []Clip, w Window, pol Policy, configured int, targetMs int64) int {
	pools := candidatePools(clips, w, pol)
	matched := pools[len(pools)-1].clips
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
	shortest := durations[0]
	for _, d := range durations {
		shortest = min(shortest, d)
	}
	required := int(min((targetMs+shortest-1)/shortest, maxPodClips))
	if required > configured {
		return required
	}
	return configured
}

// maxPodClips bounds the time-sized clip ceiling so a catalog of near-zero-length clips cannot
// ask for a pod of thousands of entries.
const maxPodClips = 64

// windowFor builds the Window a channel's selection describes.
//
// Extracted so Preview and Coverage cannot disagree about what a channel's selection MEANS.
// They already share the ladder (both end up in `candidatePools`); this is the step before it,
// and duplicating it is how a meter ends up reporting on a slightly different window than the
// pod it claims to describe — an off-by-one-field bug that no ladder test would catch.
func (a *PodAdapter) windowFor(channelID string, seed int64, sel Selection, podMax int, breakDurationMs int64, season schedule.HolidaySeason) Window {
	return Window{
		Season:    season,
		ChannelID: channelID, Seed: seed,
		Era: sel.Era, Audience: sel.Audience, Geography: sel.Geography, Language: sel.Language,
		EraWindows: append([]EraRange(nil), sel.EraWindows...),
		// ⚠ The gap IS the break. The scheduler cuts a slot of exactly breakDurationMs and the
		// playout walks the pod through it by offset, so a pod one second longer loses its tail —
		// the return bumper and the last ad's end. This was max(10 minutes, break), which let a
		// 2 minute break hold 132 seconds of pod (4 × 30s ads + two bumpers).
		GapMs: breakDurationMs, PodMax: podMax, BreakDurationMs: breakDurationMs,
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
	w := a.windowFor(channelID, 0, sel, podMax, breakDurationMs, a.seasonAt(pol, time.Time{}))
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
	return FitFor(c, a.windowFor(channelID, 0, sel, podMax, breakDurationMs, a.seasonAt(pol, time.Time{})), pol)
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
