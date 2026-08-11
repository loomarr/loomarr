// Package binder materializes an APPROVED proposal onto a channel (§7): create it
// on first approval, patch it (preserving operator-owned fields) on re-approval or
// refine. It is the ONE implementation of that logic — extracted out of
// internal/api so both the manual-approve HTTP handler and the suggest worker's
// auto-approve path share it (previously only the HTTP path called it, so a
// per-user auto-approved refine enqueued acquisitions but never rebound its
// channel — a live channel with a stale lineup).
//
// binder is deliberately neutral: it imports store/schedule/suggest/channels, but
// nothing in those packages imports binder back, so nothing new is coupled to it.
// Consumers (internal/api, internal/suggest) declare the narrow interface they
// need and take a *Binder as a structural implementation — dependency inversion,
// not a shared concrete dependency.
package binder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// Store is the slice of store.Store the binder needs. Narrow, so a fake in tests
// doesn't have to satisfy the entire store.
type Store interface {
	// ListChannels backs nextFreeChannelNumber ONLY — that genuinely needs every number in
	// use. Looking a channel up BY something is GetChannelByIntentRef below.
	ListChannels(ctx context.Context) ([]store.Channel, error)
	GetChannelByIntentRef(ctx context.Context, intentRef string) (store.Channel, error)
	UpsertChannel(ctx context.Context, ch store.Channel) error
	NewestProposalByStatusForJob(ctx context.Context, jobID, status string) (store.Proposal, error)
	// GetTitle backs the additive-merge availability check (§8.2): re-curation may keep a
	// title only if it's still available, so it needs to read each title's state.
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// Reconciler pushes a channel's desired state to Tunarr (§9). Satisfied by
// *channels.Engine. A nil Reconciler (channels not wired) is fine — BindApprovedChannel
// then just skips the "go live immediately" push and leaves the channel for the sweep,
// matching today's best-effort reconcile behavior.
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
// A channel stranded pre-Tunarr is exactly the kind of thing an operator finds hours later, and
// "why" is unanswerable once the line has scrolled. Optional and nil-safe: an install with no
// recorder wired behaves as before.
type ActivityRecorder interface {
	Error(ctx context.Context, kind, subjectID, text string)
}

// Binder materializes approved proposals onto channels (§7). Holds the store + an
// optional Reconciler + a logger.
type Binder struct {
	store Store
	rec   Reconciler
	codec CodecComputer
	acts  ActivityRecorder
	log   *slog.Logger
}

// New builds a Binder. rec may be nil (no Tunarr wired yet); the bind still
// creates/patches the channel row, it just skips the immediate reconcile push.
// codec may be nil (no playout wiring); the bind then leaves the channel's stored
// broadcast codec (h264 default) untouched — see CodecComputer.
func New(st store.Store, rec Reconciler, codec CodecComputer, log *slog.Logger) *Binder {
	return &Binder{store: st, rec: rec, codec: codec, log: log}
}

// WithActivity attaches the durable recorder. Chained rather than added to New's signature so
// every existing caller and test keeps compiling — the same nil-means-skip idiom `rec`/`codec` use.
func (b *Binder) WithActivity(a ActivityRecorder) *Binder { b.acts = a; return b }

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
// path) that need the same id scheme without going through BindApprovedChannel.
func NewChannelID() string { return newChannelID() }

// BindApprovedChannel creates — or, on re-approval/refine, patches — the channel an
// approved proposal describes (§7). Idempotent on IntentRef (the suggestion job
// id), so approving the same intent twice never mints a second channel.
//
// THIS IS THE ONE IMPLEMENTATION shared by the manual-approve HTTP handler and the
// suggest worker's auto-approve path (§8/§11) — the two can never disagree about
// what "approve → channel" means.
func (b *Binder) BindApprovedChannel(ctx context.Context, p store.Proposal) (string, error) {
	intentRef := p.JobID
	if intentRef == "" {
		return "", fmt.Errorf("proposal %s has no job id to bind a channel to", p.ID)
	}

	lineup, err := b.LineupFromIntent(ctx, intentRef)
	if err != nil {
		return "", fmt.Errorf("resolve lineup: %w", err)
	}
	policy, err := b.PolicyFromIntent(ctx, intentRef)
	if err != nil {
		return "", fmt.Errorf("resolve policy: %w", err)
	}

	existing, err := b.channelByIntent(ctx, intentRef)
	if err != nil {
		return "", err
	}

	ch := existing
	if ch.ID == "" { // first approval of this intent → a new channel
		ch.ID = newChannelID()
		ch.IntentRef = intentRef
		// Sequential is the channel Strategy that decides ordering ONLY when the grounded
		// policy leaves Ordering == OrderInherit — i.e. a single-series channel, where
		// episodes-in-order is correct. A multi-series channel's grounded policy carries
		// OrderSyndication explicitly (groundPolicy multiSeries default), and policy ordering
		// WINS over the strategy inherit (policy.Resolved) — so this does NOT force a Star
		// Trek channel chronological. Don't "simplify" this to syndication: that would make a
		// genuinely single-series channel intermix nothing-but-itself and lose episode order.
		ch.Strategy = schedule.Sequential
		ch.Name = channelNameFromIntent(p)
		ch.Number, err = b.nextFreeChannelNumber(ctx)
		if err != nil {
			return "", err
		}
	}
	// Lineup binding differs by WHO approved (§8.2). A human-in-the-loop approval (manual
	// approve, or a manual refine the operator drove) REPLACES the lineup — a person decided,
	// including to remove titles. Scheduled AUTO-CURATE runs unattended, so it must be
	// NON-DESTRUCTIVE: it ADDS the refreshed picks onto the existing lineup and only drops a
	// title that's clearly off-intent (genuinely gone from the library, or explicitly excluded)
	// — never a still-available title the stochastic LLM just didn't re-pick this run. Without
	// this, weekly re-curation would churn a channel, silently dropping good titles nobody chose
	// to remove (the §8.2 "never drop a currently-airing available title just for churn" rule).
	// Both branches go through the one lineup primitive (schedule.ApplyLineup, §9): the human
	// path is a plain Replace, auto-curate is Additive with the store-backed Drop predicate.
	if isAutoCurate(p) && existing.ID != "" {
		// ⚠ `retiredKeys(p)` carries the turnstile's rotate-out decisions (§8.2a). They arrive
		// as an INPUT to the union rather than as a channel write `recurate` made moments
		// earlier — which is what stops the union from re-adding what the turnstile removed.
		ch.Lineup = schedule.ApplyLineup(existing.Lineup, lineup, schedule.LineupAdditive,
			schedule.ApplyOpts{Drop: b.dropPredicate(ctx, mustExcludeKeys(p), retiredKeys(p))})
	} else {
		ch.Lineup = schedule.ApplyLineup(existing.Lineup, lineup, schedule.LineupReplace, schedule.ApplyOpts{})
	}
	// Policy ownership is enforced in ONE place (schedule.MergeFromProposal, §2.1/§8.2): the
	// fresh proposal refreshes what it owns (scope/audience/separation/ordering/seasonal) —
	// EXCEPT any field the operator pinned — while operator-owned fields (filler/window/
	// autoCurate), operator-pinned fields, and the reconcile-owned Applied are preserved, and
	// rules merge by provenance. This replaces the old capture-then-restore block; a refine can
	// no longer silently revert an operator's era/ceiling edit or turn off its own AutoCurate.
	ch.Policy = existing.Policy.MergeFromProposal(policy)
	// On a FIRST approval the channel has no filler yet → seed it from the program scope era so
	// a "90s action" channel gets 90s ads out of the box (audience/category/kinds stay "any").
	// A re-approval keeps the operator's tuned filler (MergeFromProposal already preserved it).
	if ch.Policy.Filler == nil && ch.Policy.Scope.Era != nil {
		ch.Policy.Filler = &schedule.FillerSelection{Era: ch.Policy.Scope.Era}
	}
	ch.Status = schedule.StatusBuilding
	// ⚠ **Due NOW, so the sweep owns this channel from the moment it exists** (§9 V54). The
	// immediate reconcile below is best-effort; this is what makes the "the sweep retries"
	// promise true rather than aspirational. Belt-and-braces with the claim predicate (a zero
	// deadline is also due now) — but stamping it here means the row never depends on that
	// reading, and a future writer that re-adds a `> 0` guard breaks one defence, not both.
	ch.ReconcileDeadline = time.Now().UTC()

	if err := ch.Validate(); err != nil {
		return "", fmt.Errorf("invalid channel: %w", err)
	}
	if err := b.store.UpsertChannel(ctx, ch); err != nil {
		return "", err
	}

	// Compute + store the channel's uniform broadcast codec from the just-bound content (§9.1 V50):
	// the majority codec of the lineup's landed programs, which the timeline normalizes to. Runs here
	// so the codec is set BEFORE the first play (no default-h264 window). Best-effort — a probe
	// failure logs and leaves the stored codec at its current value (h264 default on a first bind),
	// never failing the bind: the channel must go on air even if the codec measurement can't.
	if b.codec != nil {
		if _, cerr := b.codec.ComputeChannelCodec(ctx, ch.ID); cerr != nil && b.log != nil {
			b.log.Warn("broadcast codec not computed (channel keeps stored codec)",
				"channel", ch.ID, "err", cerr)
		}
	}

	// Go live immediately (§9 "never dead air"); best-effort, the sweep retries — which is true
	// only because the channel was stamped due-now above. See the ⚠ on `sqliteChannelClaimSQL`.
	if b.rec != nil {
		if err := b.rec.Reconcile(ctx, ch.ID); err != nil {
			// ⚠ ERROR, not Warn, and it names the CONSEQUENCE. This leaves a channel that exists,
			// has a lineup and shows a full schedule in the guide, but has never been pushed to
			// Tunarr — an operator reading "warning" would reasonably scroll past the one line
			// explaining why their new channel is stuck on "Creating".
			b.log.Error("initial reconcile of an approved channel FAILED — it is not on Tunarr yet; the next channel sweep will retry",
				"channel", ch.ID, "name", ch.Name, "number", ch.Number, "err", err)
			// And durably, because the log line above is gone the moment the terminal scrolls.
			if b.acts != nil {
				b.acts.Error(ctx, "channel.reconcile", ch.ID,
					fmt.Sprintf("%q couldn't be pushed to Tunarr yet: %v. Loomarr will keep retrying.", ch.Name, err))
			}
		}
	}
	return ch.ID, nil
}

// channelByIntent finds the channel already bound to this intent, if any. Returns a
// zero Channel when none exists — the caller distinguishes "rebind an existing channel" from
// "create a new one" on `ch.ID == ""`, so not-found is an ordinary answer here, not an error.
//
// ⚠ An INDEXED lookup since V41 (00037). This was `ListChannels` plus a linear walk, duplicated
// byte-for-byte in `recurate.channelForJob` — two packages reading the whole channel table to
// find one row, which is what a missing store method leaves behind.
func (b *Binder) channelByIntent(ctx context.Context, intentRef string) (store.Channel, error) {
	ch, err := b.store.GetChannelByIntentRef(ctx, intentRef)
	if errors.Is(err, store.ErrNotFound) {
		return store.Channel{}, nil
	}
	if err != nil {
		return store.Channel{}, err
	}
	return ch, nil
}

// nextFreeChannelNumber returns the lowest unused positive channel number, so an
// operator never has to pick one to get on air (§7). Channel counts here are
// household-scale, so a scan is cheaper and clearer than tracking a counter.
func (b *Binder) nextFreeChannelNumber(ctx context.Context) (int, error) {
	all, err := b.store.ListChannels(ctx)
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool, len(all))
	for _, c := range all {
		used[c.Number] = true
	}
	for n := 1; ; n++ {
		if !used[n] {
			return n, nil
		}
	}
}

// channelNameFromIntent derives a channel-sized label for a newly-approved channel. It
// prefers the LLM's proposed channelName (a real network-style name — "Springfield
// Classics" — §8), falling back to a truncated intent description, then a generic label.
// A starting point, not a decision: name is an ordinary editable field (§7 PATCH).
func channelNameFromIntent(p store.Proposal) string {
	var parsed suggest.Proposal
	if err := json.Unmarshal([]byte(p.ProposalJSON), &parsed); err == nil {
		if n := strings.TrimSpace(parsed.ChannelName); n != "" {
			return truncateLabel(n, 60)
		}
		if d := strings.TrimSpace(parsed.Intent.Description); d != "" {
			return truncateLabel(d, 60)
		}
	}
	return "Suggested channel"
}

// ChannelNameFromIntent is the exported form, kept for callers/tests outside this
// package that need the same derivation (mirrors the unexported helper 1:1).
func ChannelNameFromIntent(p store.Proposal) string { return channelNameFromIntent(p) }

// truncateLabel shortens on a word boundary so a long intent doesn't produce a name
// cut mid-word in the middle of a TV guide.
func truncateLabel(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-") + "…"
}

// LineupFromIntent resolves the approved proposal identified by intentRef (the
// suggestion job id) and maps its lineup into the scheduler's []LineupEntry —
// the "create a channel from an approved proposal (intent + lineup)" flow the
// API contract promises (§7 POST /v1/channels, §9). Without this, a channel
// created from an approved intent builds EMPTY (the intentRef would be inert).
//
// Returns (nil, nil) when intentRef is "" — a hand-made channel legitimately has
// no source proposal and gets its lineup via PUT /v1/channels/{id} instead.
func (b *Binder) LineupFromIntent(ctx context.Context, intentRef string) ([]schedule.LineupEntry, error) {
	if intentRef == "" {
		return nil, nil // hand-made channel; no proposal to bind
	}
	prop, err := b.ApprovedProposalForJob(ctx, intentRef)
	if err != nil {
		return nil, err
	}
	var p suggest.Proposal
	if err := json.Unmarshal([]byte(prop.ProposalJSON), &p); err != nil {
		return nil, fmt.Errorf("decode proposal %s: %w", prop.ID, err)
	}
	return LineupEntries(p)
}

// PolicyFromIntent resolves the approved proposal's grounded ChannelPolicy
// (programming-design §8) so it lands on the channel row at create time. Returns a
// zero policy (⇒ built-in defaults) for a hand-made channel (empty intentRef) or a
// proposal that carried no policy. Mirrors LineupFromIntent — same approved-proposal
// gate, so an unapproved intent never brings a policy onto a live channel.
func (b *Binder) PolicyFromIntent(ctx context.Context, intentRef string) (schedule.ChannelPolicy, error) {
	if intentRef == "" {
		return schedule.ChannelPolicy{}, nil
	}
	prop, err := b.ApprovedProposalForJob(ctx, intentRef)
	if err != nil {
		return schedule.ChannelPolicy{}, err
	}
	var p suggest.Proposal
	if err := json.Unmarshal([]byte(prop.ProposalJSON), &p); err != nil {
		return schedule.ChannelPolicy{}, fmt.Errorf("decode proposal %s: %w", prop.ID, err)
	}
	return p.Policy, nil
}

// ApprovedProposalForJob finds the proposal for a suggestion job.
//
// DECISION (business logic — see the TODO): which proposal counts, and must it
// already be approved? A channel materializes real content; binding an
// unapproved (or denied) proposal's lineup would drive acquisitions/streaming
// off content that never cleared the human-in-the-loop gate (§8). This resolver
// is the last checkpoint before a proposal's picks become a live channel.
func (b *Binder) ApprovedProposalForJob(ctx context.Context, jobID string) (store.Proposal, error) {
	// Only APPROVED proposals qualify: this is the last checkpoint before a
	// proposal's picks become a live channel, so we enforce the §8 gate by only
	// ever looking at status "approved". An intent that was never approved (or was
	// denied) simply has no match here → a hard error, not an empty channel.
	//
	// ⚠ NEWEST wins, and the store method's ORDER BY created_at DESC is what guarantees it.
	// Load-bearing for refine (§7): a refine re-runs the channel's own job, producing a newer
	// approved proposal, and the channel must bind to THAT — the latest approved lineup — not
	// the original. A job therefore has several approved proposals over its life. Asserted by
	// TestRefine_NewestApprovedWins.
	//
	// ⚠ Filtered and LIMITed in SQL since V41. This read EVERY approved proposal in the install
	// and took the first match, relying on an ordering guaranteed by a different method — and
	// retention deliberately never purges approved proposals (they are the audit trail), so the
	// table it scanned grows monotonically while denied ones are swept. It ran on every bind,
	// including every scheduled auto-curate cycle for every channel.
	p, err := b.store.NewestProposalByStatusForJob(ctx, jobID, "approved")
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Proposal{}, err
	}
	// No approved proposal for this intent. Refuse to build — don't let unapproved
	// content reach a live channel (prime directive #3). createChannel maps this to
	// a 422 so the caller sees a clear "approve the proposal first" signal.
	return store.Proposal{}, fmt.Errorf("no approved proposal for intent %q", jobID)
}

// LineupEntries maps an approved proposal's picks — BOTH the in-library lineup
// AND the acquisition list — to scheduler entries. This is the fix for the #9
// seam: acquisitions previously never entered ch.Lineup, so once a title landed
// `available` it had no entry to fill and was permanently unschedulable (the
// backfill sweep re-derives desired slots from ch.Lineup, so an absent key can
// never be recovered). Every approved pick is an entry; whether it renders as a
// program or a pending slot is decided at reconcile time by resolveEntry against
// live availability (§9), NOT by the proposal's (possibly stale) InLibrary flag.
//
// Ordering: lineup picks first (the human-curated order), then acquisitions —
// which start as pending slots and swap to programs in place as they land.
// Duplicate keys are collapsed so a title that appears in both lists (e.g. an
// acquisition the human also marked in-library) yields exactly one entry.
func LineupEntries(p suggest.Proposal) ([]schedule.LineupEntry, error) {
	out := make([]schedule.LineupEntry, 0, len(p.Lineup)+len(p.Acquisitions))
	seen := make(map[provision.Key]struct{}, len(p.Lineup)+len(p.Acquisitions))
	for _, items := range [][]suggest.ProposalItem{p.Lineup, p.Acquisitions} {
		for _, it := range items {
			key, err := provision.KeyFromWebhook(it.MediaType, it.TMDBID, it.TVDBID)
			if err != nil {
				return nil, fmt.Errorf("lineup key for %q: %w", it.Name, err)
			}
			if _, dup := seen[key]; dup {
				continue // same title in both lists → one entry
			}
			seen[key] = struct{}{}
			out = append(out, schedule.LineupEntry{
				Key:   key,
				Title: it.Name,
				// DurationMs is left 0 (unknown) here; the reconciler resolves real
				// runtime from the library when it computes desired slots (§9). An
				// acquisition not yet in the library resolves to a pending slot.
				//
				// Policy-enforcement metadata is stamped from the grounded pick
				// (programming-design §4): the full ProposalItem is in hand here and
				// currently the only place it is, so the audience/era/genre filters
				// enforce off the entry without a per-reconcile library hit.
				OfficialRating: schedule.NormalizeRating(it.OfficialRating),
				Genres:         it.Genres,
				Year:           it.Year,
				// Airing season window (§8): a series pick the suggester scoped to an
				// era ("classic Simpsons" → 1–10) carries its window onto the entry, so
				// series expansion airs only those seasons (§9 inSeasonRange). 0 = all.
				SeasonMin: it.SeasonMin,
				SeasonMax: it.SeasonMax,
			})
		}
	}
	return out, nil
}
