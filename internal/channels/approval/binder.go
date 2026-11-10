// Package approval plans how an APPROVED proposal changes a channel (§7): create it
// on first approval, patch it (preserving operator-owned fields) on re-approval or
// refine. Local persistence belongs to the approval transaction; codec measurement
// and backend reconciliation run only after that transaction commits.
//
// approval is deliberately neutral: it imports the store, schedule, proposals/accept and
// provision packages, but nothing in those imports it back, so nothing new is coupled to it.
// Consumers (internal/api, internal/proposals/accept) declare the narrow interface they
// need and take a *Binder as a structural implementation — dependency inversion,
// not a shared concrete dependency.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/activitystore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals/accept"
)

// Store is the slice of store.Store the binder needs. Narrow, so a fake in tests
// doesn't have to satisfy the entire store.
type Store interface {
	// ListChannels backs nextFreeChannelNumber ONLY — that genuinely needs every number in
	// use. Looking a channel up BY something is GetChannelForJob below.
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	GetChannel(ctx context.Context, id string) (channelstore.Channel, error)
	GetChannelForJob(ctx context.Context, jobID string) (channelstore.Channel, error)
	GetJob(ctx context.Context, id string) (proposalstore.Job, error)
	NewestProposalByStatusForJob(ctx context.Context, jobID, status string) (proposalstore.Proposal, error)
	// GetTitle backs the additive-merge availability check (§8.2): re-curation may keep a
	// title only if it's still available, so it needs to read each title's state.
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// Reconciler converges a channel's effective playout backend (§9). Satisfied by
// *channels.Engine. A nil Reconciler (channels not wired) is fine — the committed,
// due-now channel remains available to the sweep.
type Reconciler interface {
	Reconcile(ctx context.Context, channelID string) error
}

// CodecComputer derives + stores a channel's uniform broadcast codec from its content (§9.1 V50):
// it samples the just-bound lineup's files, probes their codecs, and persists the majority. The
// binder stays codec-agnostic (it has no library/probe access by design) and only TRIGGERS this
// after the lineup is written — the probe itself lives in the composition root's playout resolver,
// which already owns library resolution + ffprobe. Nil ⇒ the channel keeps its stored codec (h264
// by migration default), so an install without playout wiring binds exactly as before.
type CodecComputer interface {
	ComputeChannelCodec(ctx context.Context, channelID string) (string, error)
}

// ActivityRecorder gives a failed first reconcile a DURABLE home (§9 V54).
//
// ⚠ It exists because a `log.Warn` was the only record, and terminal scrollback is not a record.
// A channel stranded before its first successful convergence is exactly the kind of thing an operator finds hours later, and
// "why" is unanswerable once the line has scrolled. Optional and nil-safe: an install with no
// recorder wired behaves as before.
type ActivityRecorder interface {
	Error(ctx context.Context, kind, subjectID, text string)
}

// NumberSource reports the channel numbers TUNARR already uses, so numbering can avoid them
// (§9 V54).
//
// ⚠ A SET of numbers, not a list of channels, and deliberately not `programmer.Programmer`. The
// binder wants to answer "is 1 taken?" — handing it a programmer would couple this package to the
// Tunarr adapter for a question that is one integer wide, and would import a package this one has
// stayed clear of. The composition root adapts. Optional and nil-safe: without it, numbering sees
// Loomarr's store only, exactly as before.
type NumberSource interface {
	TakenChannelNumbers(ctx context.Context) (map[int]bool, error)
}

// Binder plans approved proposals onto channels (§7), then triggers optional
// post-commit convergence work.
type Binder struct {
	store   Store
	rec     Reconciler
	codec   CodecComputer
	acts    ActivityRecorder
	numbers NumberSource
	log     *slog.Logger
}

// New builds a Binder. rec may be nil (channel convergence not wired); approval still
// creates/patches the due-now channel row, it just skips immediate reconciliation.
// codec may be nil (no playout wiring); the bind then leaves the channel's stored
// broadcast codec (h264 default) untouched — see CodecComputer.
func New(st Store, rec Reconciler, codec CodecComputer, log *slog.Logger) *Binder {
	return &Binder{store: st, rec: rec, codec: codec, log: log}
}

// WithActivity attaches the durable recorder. Chained rather than added to New's signature,
// with the same nil-means-skip idiom `rec`/`codec` use.
func (b *Binder) WithActivity(a ActivityRecorder) *Binder { b.acts = a; return b }

// WithChannelNumbers lets numbering see Tunarr's occupied numbers as well as Loomarr's own.
func (b *Binder) WithChannelNumbers(n NumberSource) *Binder { b.numbers = n; return b }

// newChannelID mints the id for a channel created by approving an intent. The
// `ch_` prefix matches the ids the API documents and that callers assign by hand
// via POST /v1/channels, so an approved channel is indistinguishable from one made
// deliberately — it IS one.
func newChannelID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "ch_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "ch_" + hex.EncodeToString(b[:])
}

// NewChannelID is the exported form, for callers (e.g. the API's hand-made-channel
// path) that need the same id scheme without going through approval planning.
func NewChannelID() string { return newChannelID() }

// PlanApprovedChannel derives the complete local channel state for one approval.
// It reads the current channel/title/number snapshots but performs no writes. The
// caller commits the returned channel in the SAME transaction as the proposal CAS
// and title inserts, so "approved without a local channel" is not a representable
// successful outcome.
//
// The supplied proposal is the exact candidate being decided. In particular, this
// method does not query "newest approved" by job: before the approval transaction
// commits, the candidate is necessarily not visible through that query, and resolving
// again could pair the candidate's audit identity with another proposal's content.
func (b *Binder) PlanApprovedChannel(ctx context.Context, p proposalstore.Proposal) (channelstore.Channel, error) {
	if p.Status != "approved" {
		return channelstore.Channel{}, fmt.Errorf("proposal %s has status %q, want approved", p.ID, p.Status)
	}
	return b.planChannel(ctx, p)
}

// PlanSubmittedChannel derives an unsaved channel for a pending proposal outlook.
// It shares approval planning while preserving the submitted status and performs
// no writes or post-commit work. Callers first use accept.PrepareApproval.
func (b *Binder) PlanSubmittedChannel(ctx context.Context, p proposalstore.Proposal) (channelstore.Channel, error) {
	if p.Status != "submitted" {
		return channelstore.Channel{}, fmt.Errorf("proposal %s has status %q, want submitted", p.ID, p.Status)
	}
	return b.planChannel(ctx, p)
}

// planChannel is the impure shell around Bind: it reads the channel the job changes, the job
// itself, a free number and the availability of the existing lineup, hands them to the pure Bind,
// and logs what an unattended auto-curate bind dropped.
func (b *Binder) planChannel(ctx context.Context, p proposalstore.Proposal) (channelstore.Channel, error) {
	intentRef := p.JobID
	if intentRef == "" {
		return channelstore.Channel{}, fmt.Errorf("proposal %s has no job id to bind a channel to", p.ID)
	}
	existing, err := b.channelForJob(ctx, intentRef)
	if err != nil {
		return channelstore.Channel{}, err
	}

	in := BindInput{Proposal: p, Existing: existing, Now: time.Now().UTC()}
	if existing.ID == "" {
		// A first approval always has its job. A job that is gone (purged, or never existed)
		// may have been a refine or re-curation that lost its link to a channel, and creating
		// one here would duplicate that channel. This is a plain error for the approver to
		// surface, not a "no channel yet" answer that replans.
		if _, jerr := b.store.GetJob(ctx, intentRef); errors.Is(jerr, store.ErrNotFound) {
			return channelstore.Channel{}, fmt.Errorf("%w: proposal %s, job %s, so no channel can be created from it",
				accept.ErrProposalJobGone, p.ID, intentRef)
		} else if jerr != nil {
			return channelstore.Channel{}, fmt.Errorf("proposal %s: read job %s: %w", p.ID, intentRef, jerr)
		}
		if in.Number, err = b.nextFreeChannelNumber(ctx); err != nil {
			return channelstore.Channel{}, err
		}
	} else if isAutoCurate(p) {
		in.Unavailable = b.unavailableTitles(ctx, existing.Lineup)
	}

	bound, err := Bind(in)
	if err != nil {
		return channelstore.Channel{}, err
	}
	if b.log != nil {
		for _, e := range bound.Retired {
			b.log.Info("auto-curate retired a title to make room", "title", e.Title, "key", e.Key)
		}
		for _, e := range bound.Dropped {
			b.log.Info("auto-curate dropped a clearly-off-intent title", "title", e.Title, "key", e.Key)
		}
	}
	return bound.Channel, nil
}

// unavailableTitles reads which titles of an existing lineup have left the acquisition pipeline
// entirely (§8.2 conservative pruning). An in-flight title (wanted, requested, downloading) is on
// its way, not off the channel. A read error is treated as "keep": never drop a title on a
// transient error.
func (b *Binder) unavailableTitles(ctx context.Context, lineup []schedule.LineupEntry) map[provision.Key]bool {
	out := make(map[provision.Key]bool)
	for _, e := range lineup {
		if rec, err := b.store.GetTitle(ctx, e.Key); err == nil && rec.State == provision.Unavailable {
			out[e.Key] = true
		}
	}
	return out
}

// AfterApprovalCommitted performs the non-transactional half of approval. Both
// operations are deliberately best-effort: the local channel already committed as
// building + due-now, so the channel sweep is the durable reconcile retry and the
// stored codec's h264 default is always safe. External or derived work must never
// retroactively turn a successful local approval into an error.
func (b *Binder) AfterApprovalCommitted(ctx context.Context, channelID string) {
	// Compute + store the channel's uniform broadcast codec from the just-bound content (§9.1 V50):
	// the majority codec of the lineup's landed programs, which the timeline normalizes to. Runs here
	// so the codec is set BEFORE the first play (no default-h264 window). Best-effort — a probe
	// failure logs and leaves the stored codec at its current value (h264 default on a first bind),
	// never failing the bind: the channel must go on air even if the codec measurement can't.
	if b.codec != nil {
		if _, cerr := b.codec.ComputeChannelCodec(ctx, channelID); cerr != nil && b.log != nil {
			b.log.Warn("broadcast codec not computed (channel keeps stored codec)",
				"channel", channelID, "err", cerr)
		}
	}

	// Go live immediately (§9 "never dead air"); best-effort, the sweep retries — which is true
	// only because the channel was stamped due-now above. See the ⚠ on `sqliteChannelClaimSQL`.
	if b.rec != nil {
		if err := b.rec.Reconcile(ctx, channelID); err != nil {
			ch, _ := b.store.GetChannel(ctx, channelID)
			// ⚠ ERROR, not Warn, and it names the CONSEQUENCE. This leaves a channel that exists,
			// has a lineup but has never completed its selected backend convergence — an operator
			// reading "warning" would reasonably scroll past the one line
			// explaining why their new channel is stuck on "Creating".
			if b.log != nil {
				b.log.Error("initial reconcile of an approved channel FAILED — its backend has not converged; the next channel sweep will retry",
					"channel", channelID, "name", ch.Name, "number", ch.Number, "err", err)
			}
			// And durably, because the log line above is gone the moment the terminal scrolls.
			if b.acts != nil {
				b.acts.Error(ctx, activitystore.ActivityKindChannel, channelID,
					fmt.Sprintf("%q couldn't converge its playout backend yet: %v. Loomarr will keep retrying.", ch.Name, err))
			}
		}
	}
}

// channelForJob finds the channel this job's proposal changes: the channel a refine or
// re-curation job names, or else the channel the job originated. Returns a zero Channel when
// none exists yet — the caller distinguishes "rebind an existing channel" from "create a new
// one" on `ch.ID == ""`, so not-found is an ordinary answer here, not an error.
//
// ⚠ A job that names a channel which was removed is an ERROR, never "create a new one": approving
// a refine of a deleted channel must not resurrect it as a second, differently numbered channel.
func (b *Binder) channelForJob(ctx context.Context, jobID string) (channelstore.Channel, error) {
	ch, err := b.store.GetChannelForJob(ctx, jobID)
	if errors.Is(err, store.ErrNotFound) {
		return channelstore.Channel{}, nil
	}
	if err != nil {
		return channelstore.Channel{}, err
	}
	return ch, nil
}

// NumberInUse reports whether a channel number is already taken — by one of Loomarr's own
// channels, or by a channel that exists only in TUNARR.
//
// ⚠ **There is deliberately no `exceptChannelID` escape.** Tunarr's channel list is a bare
// `map[int]bool` with no identity, so excluding a Loomarr row would do nothing about the same
// number appearing on the Tunarr side — which it always does for a live channel, because
// Loomarr pushed it there. Such a parameter would answer correctly for the store and wrongly
// for Tunarr, which is worse than not offering it. Callers must not ask about a number a channel
// already holds; `edit` only runs this check when the number actually changes.
//
// ⚠ **This exists so both places that assign a number agree.** `nextFreeChannelNumber` (the
// approve path) unions both sources, and a number an operator TYPES must be checked against both
// too: otherwise a number Tunarr already held is accepted and then renumbered underneath the
// operator by the reconcile (§9 V54), while a conflict with a Loomarr channel gets a clean 409.
//
// ⚠ Best-effort on the Tunarr side, matching `nextFreeChannelNumber`: an unreachable Tunarr
// reports NOT-in-use rather than propagating an error. Refusing to create a channel because
// Tunarr is briefly down would be a worse failure than the collision this prevents — and the
// collision is recoverable, since the reconcile re-checks occupancy at push time.
func (b *Binder) NumberInUse(ctx context.Context, number int) (bool, error) {
	all, err := b.store.ListChannels(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range all {
		if c.Number == number {
			return true, nil
		}
	}
	if b.numbers == nil {
		return false, nil
	}
	taken, terr := b.numbers.TakenChannelNumbers(ctx)
	if terr != nil {
		b.log.Warn("couldn't read Tunarr's channel numbers; checking the number against Loomarr's store alone",
			"number", number, "err", terr)
		return false, nil
	}
	return taken[number], nil
}

// nextFreeChannelNumber returns the lowest unused positive channel number, so an
// operator never has to pick one to get on air (§7). Channel counts here are
// household-scale, so a scan is cheaper and clearer than tracking a counter.
//
// ⚠ **"Free" means free on BOTH sides — Loomarr's store AND Tunarr** (§9 V54). Consulting only the
// store is how an approved channel got number 1 while Tunarr already had a channel there: the
// create then failed with `500` and an empty body, permanently, and the operator saw nothing but
// "Creating". Tunarr accumulates channels an earlier install or a reset database left behind, so
// "my store has no channel 1" says nothing about whether 1 is usable.
//
// ⚠ Read-only on the Tunarr side, and a failure to reach it is NOT fatal: numbering falls back to
// store-only. A channel that gets a possibly-colliding number is recoverable (the reconcile
// renumbers it); a bind that refuses to complete because Tunarr is briefly down is not.
func (b *Binder) nextFreeChannelNumber(ctx context.Context) (int, error) {
	all, err := b.store.ListChannels(ctx)
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool, len(all))
	for _, c := range all {
		used[c.Number] = true
	}
	if b.numbers != nil {
		taken, terr := b.numbers.TakenChannelNumbers(ctx)
		if terr != nil {
			b.log.Warn("couldn't read Tunarr's channel numbers; numbering from Loomarr's store alone",
				"err", terr)
		}
		for n := range taken {
			used[n] = true
		}
	}
	for n := 1; ; n++ {
		if !used[n] {
			return n, nil
		}
	}
}
