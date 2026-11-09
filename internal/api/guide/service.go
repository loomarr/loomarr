package guide

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
)

// PodPreviewer answers "what commercials would this channel get" without touching
// Tunarr (§12). Implemented by pod.Adapter, which serves preview and the
// reconciler from ONE code path — a preview that could diverge from what reconcile
// attaches would be worse than none. nil ⇒ the route 501s.
type PodPreviewer interface {
	// Preview takes only a channel id: the selection + seed are derived by the adapter
	// using the SAME exported helpers the reconciler uses (channels.SelectionForChannel/
	// PodSeed), so the API cannot accidentally preview a differently-seeded pod than the
	// one that ships. This is the SAVED-state preview (GET …/pods).
	Preview(ctx context.Context, channelID string) (pod.Pod, error)
	// PreviewDraft assembles the pool for an UNSAVED draft selection (POST …/pods/preview,
	// the sandbox), through the same assembler + seed — only the selection differs. It's
	// what lets the channel page show exactly what a filler change would air before Apply.
	PreviewDraft(ctx context.Context, channelID string, sel pod.Selection) (pod.Pod, error)
	// PreviewAt assembles the pod for the break STARTING AT a specific instant — the guide's
	// per-airing composition (§12 hover card), and what internal playout actually airs.
	//
	// Separate from Preview because the seed differs: Preview answers "what pool does this
	// channel draw from", while this answers "what plays in THIS break". Consecutive breaks
	// must not replay the same adverts, which is only expressible with the start time. The pod is
	// assembled to the slot's own length, so it never runs past the break the timeline gives it.
	PreviewAt(ctx context.Context, channelID string, slot pod.BreakSlot) (pod.Pod, error)
	// PreviewAtMany is PreviewAt for many breaks of one channel, pod for pod, reading the
	// channel, catalog and play history once (#1420). The guide uses it: one call per channel
	// row instead of one per break.
	PreviewAtMany(ctx context.Context, channelID string, slots []pod.BreakSlot) ([]pod.Pod, error)
	// Coverage reports which ladder rung this channel's breaks would draw from, and how much
	// material each rung holds (V29a/V29b). Same catalog, same Window, same policy as Preview —
	// it counts what is available instead of drawing from it, which is why it takes no seed.
	//
	// On the SAME interface as the previews deliberately: the meter's whole reason to exist is
	// that it agrees with what airs, and splitting it onto its own service is the first step
	// toward two implementations of "what would this channel get".
	Coverage(ctx context.Context, channelID string) (pod.CoverageReport, error)
	// CoverageDraft is Coverage for an UNSAVED selection — the draft half of the sandbox
	// `PreviewDraft` already provides for pods (§10 V51f).
	//
	// ⚠ It exists because the meter and the pod timeline below it were describing different
	// selections during an edit: the meter read the saved policy, the timeline the draft. Both
	// now come from one selection in one response, which is the same "cannot disagree" property
	// `Coverage` was added for in the first place, applied to the editing path.
	CoverageDraft(ctx context.Context, channelID string, sel pod.Selection) (pod.CoverageReport, error)
	// ClipFit reports how ONE clip relates to EVERY channel's selection (§10 V35 item 1.7) —
	// the override picker's per-channel note, keyed by channel id.
	//
	// On this interface for the same reason Coverage and Pool are: the answer must be the one
	// assembly would give. A note reading "exact match" beside a channel whose breaks never
	// play the clip is the confident-wrong-answer failure `channelpreview.go` records the v2
	// mock's own meter committing.
	//
	// ⚠ CLIP-centric, not channel-centric: the picker shows one clip against every channel, so
	// a per-channel endpoint would be N requests to render one card.
	ClipFit(ctx context.Context, clipPath string) (map[string]pod.Fit, error)
	// Pool reports catalog-wide filler health for the Filler page's pool strip (§10 V35):
	// how much material exists, and what every live channel's breaks currently resolve to.
	//
	// On this interface for the same reason Coverage is: its per-channel numbers ARE Coverage,
	// called once per channel. An aggregate that computed its own would be a second opinion
	// about the ladder, and the two would disagree on exactly the pages an operator compares
	// when a channel looks wrong.
	Pool(ctx context.Context) (pod.PoolReport, error)
}

// GuideReader answers "what is airing now" from whichever backend streams each channel.
// Keys and arguments are always Loomarr channel ids; an adapter that reads a remote backend
// owns the translation to that backend's id. nil ⇒ now/next reads empty.
type GuideReader interface {
	// NowNext returns, per Loomarr channel id, the program airing at `now` and the one
	// following it. A channel with no generated guide is simply absent.
	NowNext(ctx context.Context, now time.Time) (map[string]ChannelNowNext, error)
	// Upcoming returns, for one Loomarr channel, the program airing now (if any) followed by
	// the next programs in airtime order, up to `limit` entries; commercial/flex gaps are
	// skipped (§9 guide freshness). Powers the Overview "what's on later" strip. An unknown
	// id / empty guide yields an empty slice, not an error.
	Upcoming(ctx context.Context, channelID string, now time.Time, limit int) ([]NowNextEntry, error)
}

// ChannelService is the channel-management surface the API depends on (§9).
// Implemented by channels.Engine + the store; abstracted so the API doesn't
// couple to the reconcile internals.
type ChannelService interface {
	// Reconcile converges one channel on its selected playout backend (§9,
	// POST /v1/channels/{id}/reconcile).
	Reconcile(ctx context.Context, channelID string) error
	// Purge hard-deletes Loomarr's channel state plus any retained managed Tunarr projection —
	// the DELETE /v1/channels/{id}?purge=true path (§7).
	Purge(ctx context.Context, channelID string) error
	// CyclePreviewDraft computes "what would air at `at`" for one channel WITHOUT touching
	// Tunarr or the store beyond the read — the §8.1 time-travel preview. A draftLineup /
	// draftPolicy (nil = use the saved value) stand in for the channel's own, so the editor
	// previews what an edit WOULD air before applying it. A zero `at` means "now". Read-only;
	// nothing persists.
	//
	// ⚠ Both preview endpoints go through this ONE method — GET …/cycle passes nil drafts.
	// `Engine.CyclePreview` still exists for the playout adapter (which wants only the slots
	// and a cache around them), but the API does not declare it: it was the saved-preview path
	// until #263, and routing that through here is what lets the SAVED preview report what the
	// §4 filters refused, not just the mid-edit one.
	CyclePreviewDraft(ctx context.Context, channelID string, at time.Time,
		draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy) (channels.CycleResult, error)
	// ScheduleDiffDraft forecasts [from, from+horizon) for the saved channel and for the same
	// draft CyclePreviewDraft takes, and returns the slots where they differ (#1877). Both sides
	// share one instant, anchor and engine, so an untouched slot compares equal. A zero `from` is
	// "now"; horizon ≤ 0 is the channel's schedule window. Read-only.
	ScheduleDiffDraft(ctx context.Context, channelID string, from time.Time, horizon time.Duration,
		draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy) (channels.ScheduleDiff, error)
	// Calendar is the household's holiday calendar the engine arranges with (#2030), so an API
	// read of "which holiday is on" agrees with what airs.
	Calendar() schedule.Calendar
}
