package binder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/proposal"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// BindInput is everything Bind needs, read by the caller beforehand. Bind itself touches no
// store, clock, network or logger: the same input always gives the same Binding.
type BindInput struct {
	// Proposal is the exact candidate being bound: its job, requester, approver and body.
	Proposal store.Proposal
	// Existing is the channel the proposal's job changes. The zero value means the job has no
	// channel yet, so Bind creates one.
	Existing store.Channel
	// Number is the channel number a NEW channel gets. The caller chooses a free one, since
	// "free" is a question for the store and the backend. Ignored when Existing is set.
	Number int
	// Unavailable holds the keys of existing lineup titles that have left the acquisition
	// pipeline entirely. It is read only for an unattended auto-curate bind, which drops such a
	// title; a key absent from it is kept.
	Unavailable map[provision.Key]bool
	// Now stamps the entries added and the channel's due-now reconcile deadline.
	Now time.Time
}

// Binding is the result of Bind.
type Binding struct {
	// Channel is the complete channel state the approval commits.
	Channel store.Channel
	// Retired are existing titles an auto-curate bind dropped because the proposal's turnstile
	// rotated them out to make room (§8.2a).
	Retired []schedule.LineupEntry
	// Dropped are existing titles an auto-curate bind dropped as clearly off-intent: named by
	// the intent's MustExclude, or gone from the library pipeline.
	Dropped []schedule.LineupEntry
}

// Bind is the pure core of the binder: it turns one proposal, and the channel it changes, into the
// channel the approval commits.
//
// Lineup binding differs by WHO approved (§8.2). A human-in-the-loop approval (manual approve, or a
// manual refine the operator drove) REPLACES the lineup — a person decided, including to remove
// titles. Scheduled AUTO-CURATE runs unattended, so it is NON-DESTRUCTIVE: it ADDS the refreshed
// picks onto the existing lineup and drops only a title that is clearly off-intent (rotated out by
// the turnstile, named by MustExclude, or gone from the library) — never a still-available title the
// stochastic LLM merely did not re-pick this run. Both branches go through the one lineup primitive
// (schedule.ApplyLineup, §9).
//
// Policy ownership is enforced in ONE place (schedule.MergeFromProposal, §2.1/§8.2): the proposal
// refreshes what it owns EXCEPT any field the operator pinned, while operator-owned fields (filler,
// window, autoCurate), pinned fields and the reconcile-owned Applied are preserved.
func Bind(in BindInput) (Binding, error) {
	p := in.Proposal
	if p.JobID == "" {
		return Binding{}, fmt.Errorf("proposal %s has no job id to bind a channel to", p.ID)
	}
	lineup, err := lineupEntries(p.Body, in.Now)
	if err != nil {
		return Binding{}, fmt.Errorf("resolve lineup: %w", err)
	}

	existing := in.Existing
	ch := existing
	if ch.ID == "" { // first approval of this intent → a new channel
		identity := sha256.Sum256([]byte(p.JobID))
		ch.ID = "ch_" + hex.EncodeToString(identity[:8])
		// The job that creates a channel is its origin and stays its intent_ref for life; a later
		// refine or re-curation is a different job that names the channel instead.
		ch.IntentRef = p.JobID
		// Whoever asked is recorded once. A refine or re-curation never changes it, so the
		// requester stays the person who made the request after housekeeping purges the job.
		ch.RequestedBy = p.CreatedBy
		// Sequential is the channel Strategy that decides ordering ONLY when the grounded policy
		// leaves Ordering == OrderInherit — i.e. a single-series channel, where episodes-in-order
		// is correct. A multi-series channel's grounded policy carries OrderSyndication explicitly,
		// and policy ordering WINS over the strategy inherit (policy.Resolved), so this does NOT
		// force a Star Trek channel chronological. Don't "simplify" this to syndication: that
		// would make a genuinely single-series channel intermix nothing-but-itself and lose
		// episode order.
		ch.Strategy = schedule.Sequential
		ch.Name = channelNameFromIntent(p)
		ch.Number = in.Number
	}

	var out Binding
	if isAutoCurate(p) && existing.ID != "" {
		// ⚠ The turnstile's rotate-out decisions (§8.2a) arrive as an INPUT to the union rather
		// than as a channel write `recurate` made earlier — which is what stops the union from
		// re-adding what the turnstile removed. Retirement is checked FIRST: it is a decision
		// already made, not a property of the title to re-derive, and a retired title is usually
		// still perfectly available.
		retired, mustExclude := retiredKeys(p.Body), mustExcludeKeys(p.Body)
		drop := func(e schedule.LineupEntry) bool {
			if _, gone := retired[e.Key]; gone {
				out.Retired = append(out.Retired, e)
				return true
			}
			if _, excluded := mustExclude[strings.ToLower(strings.TrimSpace(e.Title))]; excluded || in.Unavailable[e.Key] {
				out.Dropped = append(out.Dropped, e)
				return true
			}
			return false
		}
		ch.Lineup = schedule.ApplyLineup(existing.Lineup, lineup, schedule.LineupAdditive, schedule.ApplyOpts{Drop: drop})
	} else {
		ch.Lineup = schedule.ApplyLineup(existing.Lineup, lineup, schedule.LineupReplace, schedule.ApplyOpts{})
	}

	ch.Policy = existing.Policy.MergeFromProposal(p.Body.Policy)
	// A generated channel hands grounded era + audience facts to its operator-owned filler
	// selection once. A present selection—including one whose fields are all unset—is
	// operator-owned and never re-seeded by refine or auto-curation. Unset era retains its
	// documented live scope inheritance in channels.SelectionFrom.
	if ch.Policy.Filler == nil {
		ch.Policy.Filler = schedule.SeedFillerSelection(ch.Policy.ProposalPolicy)
	}
	// The channel keeps the approved request itself, so a refine or a re-curation can be seeded
	// from it long after the job that produced it is purged.
	ch.IntentJSON, err = channelIntentJSON(p.Body)
	if err != nil {
		return Binding{}, fmt.Errorf("proposal %s: %w", p.ID, err)
	}
	ch.Status = schedule.StatusBuilding
	// ⚠ **Due NOW, so the sweep owns this channel from the moment it exists** (§9 V54). The
	// immediate reconcile after commit is best-effort; this is what makes the "the sweep retries"
	// promise true rather than aspirational. Belt-and-braces with the claim predicate (a zero
	// deadline is also due now) — but stamping it here means the row never depends on that
	// reading, and a future writer that re-adds a `> 0` guard breaks one defence, not both.
	ch.ReconcileDeadline = in.Now

	if err := ch.Validate(); err != nil {
		return Binding{}, fmt.Errorf("invalid channel: %w", err)
	}
	if err := ch.Policy.Validate(); err != nil {
		return Binding{}, fmt.Errorf("invalid channel policy: %w", err)
	}
	out.Channel = ch
	return out, nil
}

// isAutoCurate reports whether a proposal was approved by the scheduled re-curation loop
// (§8.2), as opposed to a human or a per-user auto-approve grant. The audit field is the
// discriminator, so the binder applies additive (non-destructive) lineup semantics only for
// the unattended path — every human-in-the-loop approval keeps full-replace (a person decided).
func isAutoCurate(p store.Proposal) bool {
	return p.ApprovedBy == suggest.AutoCuratedBy
}

// mustExcludeKeys is the set of lower-cased title terms the intent asked to exclude
// (Intent.MustExclude). Used as ONE of the two "clearly off-intent" drop signals — a matching
// existing title may be dropped even though it's still available. Empty when the proposal has
// no intent or no exclusions.
func mustExcludeKeys(body proposal.Proposal) map[string]struct{} {
	out := make(map[string]struct{}, len(body.Intent.MustExclude))
	for _, t := range body.Intent.MustExclude {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

// retiredKeys is the set of lineup keys the auto-curate turnstile rotated OUT to make room for
// this proposal's incoming titles (§8.2a), as recorded on the proposal by `recurate`.
//
// ⚠ A THIRD drop signal, and the one that is a decision rather than an observation. The other
// two ask "is this title still wanted?"; this one carries an answer another subsystem already
// computed — it weighed the outgoing title's score against the incoming one's and committed to
// the swap. Empty for every non-auto-curate proposal, and for auto-curate runs that had room.
func retiredKeys(body proposal.Proposal) map[provision.Key]struct{} {
	out := make(map[provision.Key]struct{}, len(body.Retired))
	for _, k := range body.Retired {
		out[k] = struct{}{}
	}
	return out
}

// channelNameFromIntent derives a channel-sized label for a newly-approved channel. It
// prefers the LLM's proposed channelName (a real network-style name — "Springfield
// Classics" — §8), falling back to a truncated intent description, then a generic label.
// A starting point, not a decision: name is an ordinary editable field (§7 PATCH).
func channelNameFromIntent(p store.Proposal) string {
	if n := strings.TrimSpace(p.Body.ChannelName); n != "" {
		return truncateLabel(n, 60)
	}
	if d := strings.TrimSpace(p.Body.Intent.Description); d != "" {
		return truncateLabel(d, 60)
	}
	return "Suggested channel"
}

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

// channelIntentJSON is the approved intent a channel stores: the proposal's request without the
// inputs that belong to the one run that produced it.
func channelIntentJSON(p proposal.Proposal) (string, error) {
	blob, err := json.Marshal(p.Intent.ChannelIntent())
	if err != nil {
		return "", fmt.Errorf("marshal intent: %w", err)
	}
	return string(blob), nil
}

// lineupEntries maps an approved proposal's picks — BOTH the in-library lineup
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
//
// Each entry records the pick's confidence as its Fit (schedule.StampFit) and `now` as AddedAt:
// re-curation retires the lowest fit × recency, and a re-pick of an existing entry re-scores it
// (schedule.ApplyLineup keeps the original AddedAt).
func lineupEntries(p proposal.Proposal, now time.Time) ([]schedule.LineupEntry, error) {
	out := make([]schedule.LineupEntry, 0, len(p.Lineup))
	seen := make(map[provision.Key]struct{}, len(p.Lineup))
	for _, items := range [][]proposal.ProposalItem{p.Lineup, p.Acquisitions} {
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
				SeasonMin:        it.SeasonMin,
				SeasonMax:        it.SeasonMax,
				EpisodeSelection: it.EpisodeSelection,
				Fit:              schedule.StampFit(it.Confidence),
				AddedAt:          now,
			})
		}
	}
	return out, nil
}
