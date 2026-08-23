// Package recurate implements scheduled channel re-curation (programming-design §8.2): a
// self-updating channel that periodically re-evaluates its intent against the current library
// and evolves its lineup — preferring in-library matches, weighting net-new acquisitions by
// quality + intent, and NEVER bypassing the approval gate.
//
// Two pieces live here:
//   - Curator: the channel-scoped auto-curate GRANT. Given a freshly-produced re-curation
//     proposal, it approves it because the CHANNEL opted in (schedule.AutoCurate), after
//     filtering the proposal's net-new acquisitions to those clearing the quality bar and the
//     title cap. It approves through the ONE suggest.Approver implementation (audit
//     "auto-curate") — never a raw `wanted` write. Wired into the suggest worker as its
//     ChannelAutoCurator, so a re-curation proposal flows through the same persist→approve→bind
//     pipeline every other proposal does.
//   - Runner: the `channel-recurate` scheduler job. It lists channels, filters to the
//     live + intent-backed + auto-curate ones, and triggers a refresh refine on each; the
//     worker then produces the proposal the Curator considers.
package recurate

import (
	"context"
	"errors"
	"log/slog"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// Thresholds supplies the global re-curation knobs, read per call so a settings change takes
// effect without a restart (config-design §3). MinScorePct is the 0–100 quality bar; MaxTitles
// is the growth cap. A channel's schedule.AutoCurate may override either (stricter or looser).
type Thresholds interface {
	MinScorePct(ctx context.Context) int
	MaxTitles(ctx context.Context) int
}

// CuratorStore is the channel lookup the grant needs before handing an authorized, filtered
// proposal to the shared approval gate.
type CuratorStore interface {
	// GetChannelByIntentRef identifies WHICH channel a re-curation proposal authorizes: the
	// proposal's JobID is the channel's IntentRef. Indexed (00037) — it was `ListChannels`
	// plus a linear walk, duplicated byte-for-byte in `binder`.
	GetChannelByIntentRef(ctx context.Context, intentRef string) (store.Channel, error)
	// ⚠ NO SaveChannel, deliberately. This package decides retirements (§8.2a) but must not
	// APPLY them: it records them on the proposal and the binder — the single writer of a
	// channel's lineup — applies them through schedule.ApplyLineup. The method used to be here,
	// and the resulting two-writer arrangement was held together by a comment explaining that
	// this write had to land before the binder's additive union or the union would undo it.
	// Leaving the method off the interface makes that ordering unexpressible rather than merely
	// discouraged.
}

// Curator is the channel-scoped auto-curate grant (§8.2). It approves a re-curation proposal
// only when its channel opted in, and only for acquisitions clearing the quality bar within the
// title cap — everything else is dropped from the proposal before approval, so an over-bar or
// over-cap title is never requested. It is the auto-curate analogue of suggest.AutoApprover,
// but authorized by the channel, not a user, and bounded by quality/cap, not a user quota.
type Curator struct {
	store      CuratorStore
	approver   *suggest.Approver
	thresholds Thresholds
	log        *slog.Logger
}

// NewCurator wires the grant.
func NewCurator(st CuratorStore, approver *suggest.Approver, th Thresholds, log *slog.Logger) *Curator {
	if log == nil {
		log = slog.Default()
	}
	return &Curator{store: st, approver: approver, thresholds: th, log: log}
}

// Consider applies the auto-curate grant to a just-produced re-curation proposal. It returns
// (approved, reason) so the worker can log why a proposal stayed in the queue. The gate is
// closed by default: any channel that is not opted in, not found, or a proposal that isn't a
// re-curation of an auto-curate channel simply gets Reason and stays `submitted` for an admin.
func (c *Curator) Consider(ctx context.Context, p store.Proposal) (suggest.Decision, error) {
	if c == nil || p.JobID == "" {
		return suggest.Decision{Reason: "not a re-curation proposal"}, nil
	}
	ch, err := c.channelForJob(ctx, p.JobID)
	if err != nil {
		// An unreadable channel is not a reason to auto-approve. Fail CLOSED (this gate spends
		// on acquisitions): the proposal waits for an admin, the safe direction.
		return suggest.Decision{Reason: "channel unavailable"}, nil
	}
	if ch.ID == "" || ch.Policy.AutoCurate == nil {
		return suggest.Decision{Reason: "channel not opted into auto-curate"}, nil
	}
	// A paused/detached channel is not being managed — don't grow it unattended. Every other
	// state (live/building/DRIFTED) is managed and curatable — drifted is a transient reconcile
	// state, not a hands-off one (mirrors runner.eligible's deny-list).
	if ch.Status == schedule.StatusPaused || ch.Status == schedule.StatusDetached {
		return suggest.Decision{Reason: "channel paused or detached"}, nil
	}

	minScore := effectiveMinScore(ctx, ch.Policy.AutoCurate, c.thresholds)
	maxTitles := effectiveMaxTitles(ctx, ch.Policy.AutoCurate, c.thresholds)

	// Filter the proposal's net-new acquisitions to those clearing the quality bar, within the
	// growth cap. In-library lineup picks are NOT gated by score — they're already available and
	// cost nothing; they're added regardless (they extend the lineup, no acquisition). Rewrite
	// the proposal's acquisition list to exactly the survivors so Approve requests ONLY those.
	protected, err := c.protectedTitles(ctx, ch.ID)
	if err != nil {
		// Keep is an explicit operator promise. If its state cannot be read, do not
		// guess which titles are retirable; leave the proposal for manual review.
		return suggest.Decision{Reason: "discovery feedback unavailable"}, nil
	}
	res, err := filterAcquisitionsProtected(p, ch, minScore, maxTitles, protected)
	if err != nil {
		return suggest.Decision{Reason: "proposal unreadable"}, err
	}
	filtered := res.Proposal

	// ⚠ Retirements are RECORDED ON THE PROPOSAL (`filterAcquisitions` wrote them into the
	// body), not applied to the channel here. This block used to trim `ch.Lineup` and call
	// UpsertChannel, which made this package a second lineup writer: the binder's additive
	// union (§8.2) runs moments later and would re-add whatever was removed, so the two were
	// sequenced against each other by a comment rather than by any checkable contract. The
	// binder now consumes `Retired` through the same `ApplyOpts.Drop` seam it already uses for
	// off-intent titles — one writer, one primitive, and the retirement is an input to it.
	// Approve through the ONE gate (audit "auto-curate"). In-library adds land as `available`
	// records; the surviving acquisitions land as `wanted` — the same code the admin's manual
	// approve runs, so re-curation can never enqueue by a path the gate doesn't see.
	// nil edit: re-curation's narrowing is a POLICY filter (quality bar, title cap) applied by
	// this subsystem, not an approver's judgement — so it is `filtered` above rather than an
	// ApprovalEdit, which models a human's choices at the gate. Different things, deliberately
	// not conflated.
	if c.approver == nil {
		return suggest.Decision{Reason: "approval unavailable"}, errors.New("auto-curate: approval gate is not configured")
	}
	approval, err := c.approver.Approve(ctx, filtered, nil, suggest.AutoCuratedBy)
	if err != nil {
		return suggest.Decision{Reason: "approval failed"}, err
	}
	c.log.Info("auto-curated a channel",
		"channel", ch.ID, "enqueued", approval.Enqueued,
		"dropped_below_bar", len(res.BelowBar), "dropped_over_cap", len(res.OverCap),
		"min_score_pct", minScore, "max_titles", maxTitles)
	// Name what was rejected, and the score it was rejected for. A bare count says a decision
	// happened; it cannot say whether the bar is tuned correctly, and the stored proposal is
	// post-filter so the evidence is otherwise gone. Info, not Debug: these are acquisitions
	// the system declined to make on the operator's behalf, which is worth a line by default.
	for _, d := range res.BelowBar {
		c.log.Info("auto-curate: title below the quality bar",
			"channel", ch.ID, "title", d.Name, "confidence", d.Confidence, "min_score_pct", minScore)
	}
	for _, d := range res.OverCap {
		c.log.Info("auto-curate: title dropped for the title cap",
			"channel", ch.ID, "title", d.Name, "confidence", d.Confidence, "max_titles", maxTitles)
	}
	// A retirement rotates a title OUT of a channel the operator is watching. Both sides and
	// both scores, so "why did that disappear?" is answerable from the log.
	for _, r := range res.Retired {
		c.log.Info("auto-curate: retired a title to make room",
			"channel", ch.ID, "retired", r.Out, "retired_confidence", r.OutScore,
			"replaced_by", r.In, "replacement_confidence", r.InScore)
	}
	return suggest.Decision{Approved: true, Enqueued: approval.Enqueued}, nil
}

type discoveryFeedbackReader interface {
	ListDiscoveryFeedback(context.Context, store.FeedbackFilter) ([]store.DiscoveryFeedback, error)
}

func (c *Curator) protectedTitles(ctx context.Context, channelID string) (map[provision.Key]bool, error) {
	reader, ok := c.store.(discoveryFeedbackReader)
	if !ok {
		return nil, nil
	}
	household, err := reader.ListDiscoveryFeedback(ctx, store.FeedbackFilter{Scope: store.FeedbackHousehold})
	if err != nil {
		return nil, err
	}
	channel, err := reader.ListDiscoveryFeedback(ctx, store.FeedbackFilter{Scope: store.FeedbackChannel, ScopeID: channelID})
	if err != nil {
		return nil, err
	}
	events := append(channel, household...)
	seen := map[provision.Key]bool{}
	protected := map[provision.Key]bool{}
	for _, event := range events {
		if seen[event.Target] {
			continue
		}
		seen[event.Target] = true
		protected[event.Target] = event.Action == store.FeedbackKeep
	}
	return protected, nil
}

// channelForJob finds the channel bound to a suggestion job (its IntentRef). A re-curation
// proposal's JobID IS the channel's IntentRef, so this identifies which channel to authorize.
//
// ⚠ An INDEXED lookup since V41 (00037). This was `ListChannels` plus a linear walk — the same
// function, byte for byte, as `binder.channelByIntent`, because neither package had a store
// method to reach for. A zero Channel still means "no channel for this job", which `Consider`
// reads as not-opted-in and fails closed on.
func (c *Curator) channelForJob(ctx context.Context, jobID string) (store.Channel, error) {
	ch, err := c.store.GetChannelByIntentRef(ctx, jobID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Channel{}, nil
	}
	if err != nil {
		return store.Channel{}, err
	}
	return ch, nil
}
