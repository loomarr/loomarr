package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// podPreviewAdapter bridges pod.Adapter → api.PodPreviewer (§12). It exists to
// derive the selection + seed from the channel using the SAME exported helpers the
// reconciler calls, so a preview and the next reconcile of that channel produce the
// identical pool. The API stays out of that derivation entirely.
type podPreviewAdapter struct {
	store store.Store
	pods  *pod.Adapter
}

// Preview assembles the pool for the channel's SAVED filler selection (the GET …/pods
// path).
func (a podPreviewAdapter) Preview(ctx context.Context, channelID string) (pod.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return pod.Pod{}, err
	}
	return a.pods.Preview(ctx, ch.ID, channels.PodSeed(ch.ID), channels.SelectionForChannel(ch))
}

// PreviewAt assembles the pod for the break starting at a specific instant — what internal
// playout airs there, and what the guide's hover card promises.
//
// The SAME call both consumers make, for the §10 one-assembler reason: if the guide computed
// a break independently, the hover card would eventually list clips that are not the ones
// playing, and nobody would be able to reproduce the discrepancy on demand.
func (a podPreviewAdapter) PreviewAt(ctx context.Context, channelID string, slot pod.BreakSlot) (pod.Pod, error) {
	pods, err := a.PreviewAtMany(ctx, channelID, []pod.BreakSlot{slot})
	if err != nil {
		return pod.Pod{}, err
	}
	return pods[0], nil
}

// PreviewAtMany is PreviewAt for every break of one channel in a window (#1420): the channel is
// read once and the batch shares one catalog and exposure read, while each break keeps its own
// seed, snapshot and slot length — pod for pod what PreviewAt returns.
func (a podPreviewAdapter) PreviewAtMany(ctx context.Context, channelID string, slots []pod.BreakSlot) ([]pod.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	breaks := make([]pod.Break, len(slots))
	for i, s := range slots {
		ms := s.Start.UnixMilli()
		breaks[i] = pod.Break{Seed: channels.PodSeedAt(ch.ID, ms), Start: time.UnixMilli(ms).UTC(), Length: s.Length}
	}
	return a.pods.PreviewAtMany(ctx, ch.ID, channels.SelectionForChannel(ch), breaks)
}

// Coverage reports which ladder rung this channel's breaks would draw from (V29b-api).
//
// Resolved through `SelectionForChannel`, exactly like the previews above — coverage that
// derived a channel's selection its own way could report on a different window than the pods
// it claims to describe, which is the "lying meter" the whole phase exists to prevent.
func (a podPreviewAdapter) Coverage(ctx context.Context, channelID string) (pod.CoverageReport, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return pod.CoverageReport{}, err
	}
	return a.pods.CoverageFor(ctx, ch.ID, channels.SelectionForChannel(ch))
}

// CoverageDraft is Coverage for an UNSAVED selection (§10 V51f).
//
// ⚠ It takes the selection already resolved by the caller — `channels.SelectionFrom` at the API
// boundary — for the same reason `PreviewDraft` does not re-apply the scope era: the era's
// inherit-vs-explicitly-any decision has exactly one writer, and a second application here could
// not tell an operator who chose "any era" from one who left the field alone.
func (a podPreviewAdapter) CoverageDraft(ctx context.Context, channelID string, sel pod.Selection) (pod.CoverageReport, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return pod.CoverageReport{}, err
	}
	return a.pods.CoverageFor(ctx, ch.ID, sel)
}

// ClipFit reports how one clip relates to every channel's selection (§10 V35 item 1.7).
//
// ⚠ Resolved through `SelectionForChannel` per channel, exactly like Coverage above — the note
// beside a checkbox is a claim about what will play, and deriving the selection any other way
// is how a picker comes to disagree with the pods it is about to change.
//
// ⚠ EVERY channel, including paused and detached ones. Coverage and Pool exclude them because
// they report on breaks that are currently airing; this is a picker, and an operator pinning a
// clip to a paused channel is making a decision that takes effect when it resumes. Hiding the
// row would look like the channel had been deleted.
func (a podPreviewAdapter) ClipFit(ctx context.Context, clipPath string) (map[string]pod.Fit, error) {
	clip, err := a.store.GetClip(ctx, clipPath)
	if err != nil {
		return nil, err
	}
	chans, err := a.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]pod.Fit, len(chans))
	for _, ch := range chans {
		// `store.Clip` embeds `clips.Clip`, so this is the same value the catalog load hands
		// the assembler — not a re-derivation.
		out[ch.ID] = a.pods.FitForChannel(ch.ID, channels.SelectionForChannel(ch), clip.Clip)
	}
	return out, nil
}

// Pool reports catalog-wide filler health for the Filler page's pool strip (§10 V35).
//
// ⚠ **Every per-channel number here comes from the SAME `Coverage` call the channel page makes**,
// once per live channel. That is the whole design: an aggregate that computed its own answer
// would be a second opinion about the ladder, and the two would eventually contradict each other
// on the two pages an operator compares when something looks wrong. Aggregating the real answer
// costs one catalog load per channel and cannot drift.
//
// Paused and detached channels are excluded: their breaks are not airing, so counting them as
// "uncovered" would report a problem the operator deliberately created.
func (a podPreviewAdapter) Pool(ctx context.Context) (pod.PoolReport, error) {
	chans, err := a.store.ListChannels(ctx)
	if err != nil {
		return pod.PoolReport{}, err
	}
	// Only channels whose breaks are airing: Reconcilable is the one definition of "not paused,
	// not detached" (every managed state qualifies, drifted included — a transient "the guide is
	// catching up", not a channel that stopped airing).
	poolChannels := make([]pod.PoolChannel, 0, len(chans))
	for _, ch := range chans {
		if !ch.Status.Reconcilable() {
			continue
		}
		poolChannels = append(poolChannels, pod.PoolChannel{
			ChannelID: ch.ID, Name: ch.Name, Number: ch.Number, Selection: channels.SelectionForChannel(ch),
		})
	}
	// ⚠ ONE catalog load answers the counts and every channel's coverage (PoolFor), and each channel's
	// number is the SAME `Coverage` derivation the channel page shows — the invariant this function
	// is built on.
	report, err := a.pods.PoolFor(ctx, poolChannels)
	if err != nil {
		return pod.PoolReport{}, err
	}

	// Counted in SQL rather than recomputed here — "untagged" is the AI-tagging job's own
	// predicate (clipstore's `UntaggedOnly`), and a second Go definition of the word would be free to
	// disagree with the job that acts on it.
	//
	// ⚠ **The predicate is shared; the SCOPE is not.** The tagging job sets `IncludeHeld: true`
	// on purpose, because held clips are exactly what the tagger must tag. Reusing that scope
	// here would count held clips while every OTHER number in this report counts the catalog
	// alone — an install with 1 Ready clip and 12 held ones would render "CLIPS 1 / 12 clips
	// still need tagging", a headline its own subtext contradicts.
	//
	// Counting held clips here is not merely inconsistent, it is unactionable: the strip's advice
	// is to go and tag them, and they are not in the Catalog to tag. Incoming carries its own
	// count for that queue.
	// Counted in SQL, not materialised.
	report.Untagged, err = a.store.CountClips(ctx, clipstore.ClipFilter{UntaggedOnly: true})
	if err != nil {
		return pod.PoolReport{}, err
	}
	return report, nil
}

// PreviewDraft assembles the pool for a DRAFT selection (the POST …/pods/preview
// sandbox) — the same seed as the saved preview (so only the selection differs), but the
// caller's unsaved selection in place of the persisted one.
//
// ⚠ **It does not re-apply the scope era, and that is a correctness rule (V51f).** With the era a
// real range and "explicitly any" a reachable state, a copy keyed off `sel.Era == 0` cannot tell
// *unset* from an operator who chose ANY, so it would re-inherit the channel's era over the top of
// an explicit choice. `channels.SelectionFrom` is the single writer, applied at the API boundary
// where the draft policy (and therefore the DRAFT's scope, not the saved channel's) is known.
func (a podPreviewAdapter) PreviewDraft(ctx context.Context, channelID string, sel pod.Selection) (pod.Pod, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return pod.Pod{}, err
	}
	return a.pods.Preview(ctx, ch.ID, channels.PodSeed(ch.ID), sel)
}
