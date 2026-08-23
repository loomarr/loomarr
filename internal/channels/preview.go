package channels

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// CycleResult is everything one preview computation answers. It exists because the draft
// preview grew a fifth thing to report (the exclusion report) and a six-value return is not a
// signature anyone can read — but ALSO because CyclePreview's tuple is consumed by the cached
// playout path (app/cyclecache.go → playoutResolver.AiringNow), which must not churn every time
// the authoring surface learns to show one more fact. So the struct is returned by the DRAFT
// variant only, and CyclePreview keeps its tuple by unpacking it.
type CycleResult struct {
	// At is the resolved wall-clock this preview was computed for (a zero `at` means "now").
	At time.Time
	// Slots is the resolved cycle in play order (program / pending / break).
	Slots []schedule.Slot
	// Active is the curation rule that wins at At, or the base-policy attribution.
	Active schedule.ActiveRuleAttribution
	// Window is the resolved rolling-window horizon at At (0 = the whole run).
	Window time.Duration
	// Excluded is what the §4 audience ceiling and the scope filters REFUSED, with a reason
	// per item. ⚠ ComputeDesiredAt has always produced this and, until now, every caller threw
	// it away — so "why isn't X on my channel" had no answer anywhere in the product, despite
	// being the documented purpose of the type. It is the same report reconcile computes,
	// because it comes from the same call.
	Excluded schedule.ExclusionReport
}

// CyclePreview computes the channel's desired cycle at a chosen wall-clock `at` for the
// time-travel preview (§8.1). It is PURE and read-only: it loads the channel, rebuilds the
// EXACT schedule.ComputeDesiredAt inputs the reconciler uses (settings-driven window + break
// density, live availability, the same pending policy), and evaluates the pure lineup builder
// at `at` — but it heals nothing, persists nothing, and calls no Tunarr. One code path with
// reconcile, so the preview can never disagree with what actually ships (mirrors the pod
// preview, §10). A zero `at` means "now" (the engine's injected clock).
//
// Returns the resolved cycle's slots (program / pending / break, in play order), the curation
// rule active at `at` (or the base-policy attribution when nothing matched — the same
// pickRule ComputeDesiredAt makes), and the resolved rolling-window horizon at `at`
// (0 = the whole run) which explains why the preview shows ~this much runtime, not all ~800
// episodes. A detached/paused channel is still previewable — the preview is a what-if, not a
// management action.
//
// Break interleaving mirrors reconcile: breaks appear only when the channel actually has a
// filler pool (else there'd be nothing to fill them), so the preview reflects the real guide.
// We deliberately do NOT heal ratings/franchises here — those mutate the lineup and belong to
// reconcile; the preview reads the channel exactly as the last reconcile left it.
func (e *Engine) CyclePreview(ctx context.Context, channelID string, at time.Time) (
	resolvedAt time.Time, slots []schedule.Slot, active schedule.ActiveRuleAttribution, window time.Duration, err error,
) {
	r, err := e.CyclePreviewDraft(ctx, channelID, at, nil, nil)
	return r.At, r.Slots, r.Active, r.Window, err
}

// CyclePreviewDraft is CyclePreview over an UNSAVED draft (§8.1 / P6 programming/preview):
// draftLineup / draftPolicy, when non-nil, stand in for the channel's saved lineup / policy so
// the editor can see exactly what an edit WOULD air before applying it — the same
// ComputeDesiredAt the reconciler runs, still read-only (nothing persists, no Tunarr call). Nil
// means "use the saved value", so CyclePreview is just this with both nil. The break density is
// computed from the DRAFT filler selection too, so a drafted filler change is reflected.
func (e *Engine) CyclePreviewDraft(
	ctx context.Context, channelID string, at time.Time,
	draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy,
) (CycleResult, error) {
	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return CycleResult{}, fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if at.IsZero() {
		at = e.now()
	}

	// Draft overrides (nil = saved). The draft lineup is applied the SAME way an operator
	// PATCH would (ApplyLineup Replace + PreserveByKey, §9) — rich metadata carried forward by
	// key, new entries as-is (unhealed, like the saved preview) — so the preview matches what
	// the save→reconcile would actually produce. The drafted policy replaces ch.Policy so the
	// filler-pool + window helpers read it exactly as they read the saved one.
	lineup := ch.Lineup
	if draftLineup != nil {
		lineup = schedule.ApplyLineup(ch.Lineup, draftLineup, schedule.LineupReplace, schedule.ApplyOpts{PreserveByKey: true})
	}
	if draftPolicy != nil {
		ch.Policy = *draftPolicy
	}

	// Mirror reconcile's chDomain assembly (reconcile.go step 2): break density only when a
	// filler pool exists (drafted selection), and the settings-driven rolling-window horizon.
	hasFillerPool := false
	if e.pods != nil {
		// HasPool, not BuildFillerList: the question is "are there commercials to play",
		// which is backend-independent. BuildFillerList answers the narrower Tunarr question
		// and would report no pool on an install with no Tunarr (§9.1).
		hasFillerPool = e.pods.HasPool(ctx, ch.ID, PodSeed(ch.ID), SelectionForChannel(ch))
	}
	chDomain := ch.Channel
	chDomain.LastAired = e.lastAiredFor(ctx, ch.ID)
	chDomain.BreaksPerHour = BreaksPerHourFor(ch.Policy, hasFillerPool, e.breaksPerHourFor())
	chDomain.BreakDurationMs = BreakDurationFor(ch.Policy, e.breakDurationFor()).Milliseconds()
	chDomain.DefaultWindow = e.defaultWindowFor()

	// Resolve every movie's runtime in ONE media-server call before the layout asks for them one
	// at a time. ComputeDesiredAt calls Availability.Resolve per key (and walks the lineup several
	// times), so without this a 25-movie channel issues 25 sequential HTTP requests — measured as
	// ~375ms of GET /v1/guide's cold latency against a Cloudflare-fronted Emby, dwarfing the
	// arrangement itself (microseconds).
	//
	// Type-asserted because Availability is an interface the tests substitute with plain maps: an
	// implementation that cannot bulk-resolve simply skips this and takes the per-item path, which
	// is the same answer at the old speed. Best-effort inside, too — see PrewarmDurations.
	//
	// SERIES keys are deliberately not prewarmed here: a series resolves through ResolveEpisodes,
	// whose episode enumeration has its own (three-tier) caching, and whose library ids are not
	// known until that enumeration runs.
	if pw, ok := e.avail.(interface{ PrewarmDurations([]string) }); ok {
		ids := make([]string, 0, len(lineup))
		for i := range lineup {
			if lineup[i].Key.IsSeries() {
				continue
			}
			if rec, err := e.store.GetTitle(ctx, lineup[i].Key); err == nil && rec.LibraryID != "" {
				ids = append(ids, rec.LibraryID)
			}
		}
		pw.PrewarmDurations(ids)
	}

	desired := schedule.ComputeDesiredAt(chDomain, lineup, e.avail, e.policy, ch.Policy, at)
	return CycleResult{
		At:     at,
		Slots:  desired.Slots,
		Active: schedule.ActiveRuleAt(ch.Policy.Rules, at),
		Window: schedule.ResolveWindow(chDomain, ch.Policy, at),
		// ⚠ Carried out rather than dropped. This is the ONLY place the exclusion report reaches
		// a caller: reconcile computes the identical report and discards it, so a title refused
		// by the ceiling or the scope filters was invisible everywhere in the product.
		Excluded: desired.Excluded,
	}, nil
}
