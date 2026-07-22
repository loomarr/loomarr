package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// lineupFromIntent resolves the approved proposal identified by intentRef (the
// suggestion job id) and maps its lineup into the scheduler's []LineupEntry —
// the "create a channel from an approved proposal (intent + lineup)" flow the
// API contract promises (§7 POST /v1/channels, §9). Without this, a channel
// created from an approved intent builds EMPTY (the intentRef would be inert).
//
// Returns (nil, nil) when intentRef is "" — a hand-made channel legitimately has
// no source proposal and gets its lineup via PUT /v1/channels/{id} instead.
func (s *Server) lineupFromIntent(ctx context.Context, intentRef string) ([]schedule.LineupEntry, error) {
	if intentRef == "" {
		return nil, nil // hand-made channel; no proposal to bind
	}
	prop, err := s.approvedProposalForJob(ctx, intentRef)
	if err != nil {
		return nil, err
	}
	var p suggest.Proposal
	if err := json.Unmarshal([]byte(prop.ProposalJSON), &p); err != nil {
		return nil, fmt.Errorf("decode proposal %s: %w", prop.ID, err)
	}
	return lineupEntries(p)
}

// policyFromIntent resolves the approved proposal's grounded ChannelPolicy
// (programming-design §8) so it lands on the channel row at create time. Returns a
// zero policy (⇒ built-in defaults) for a hand-made channel (empty intentRef) or a
// proposal that carried no policy. Mirrors lineupFromIntent — same approved-proposal
// gate, so an unapproved intent never brings a policy onto a live channel.
func (s *Server) policyFromIntent(ctx context.Context, intentRef string) (schedule.ChannelPolicy, error) {
	if intentRef == "" {
		return schedule.ChannelPolicy{}, nil
	}
	prop, err := s.approvedProposalForJob(ctx, intentRef)
	if err != nil {
		return schedule.ChannelPolicy{}, err
	}
	var p suggest.Proposal
	if err := json.Unmarshal([]byte(prop.ProposalJSON), &p); err != nil {
		return schedule.ChannelPolicy{}, fmt.Errorf("decode proposal %s: %w", prop.ID, err)
	}
	return p.Policy, nil
}

// approvedProposalForJob finds the proposal for a suggestion job.
//
// DECISION (business logic — see the TODO): which proposal counts, and must it
// already be approved? A channel materializes real content; binding an
// unapproved (or denied) proposal's lineup would drive acquisitions/streaming
// off content that never cleared the human-in-the-loop gate (§8). This resolver
// is the last checkpoint before a proposal's picks become a live channel.
func (s *Server) approvedProposalForJob(ctx context.Context, jobID string) (store.Proposal, error) {
	// Only APPROVED proposals qualify: this is the last checkpoint before a
	// proposal's picks become a live channel, so we enforce the §8 gate by only
	// ever looking at status "approved". An intent that was never approved (or was
	// denied) simply has no match here → a hard error, not an empty channel.
	// Ordered created_at DESC (store.ListProposalsByStatus), so the FIRST match is the
	// NEWEST approved proposal for this job. That is deliberate and load-bearing for
	// refine (§7): a refine re-runs the channel's own job, producing a newer approved
	// proposal, and the channel must bind to THAT — the latest approved lineup — not the
	// original. (A job can therefore have several approved proposals over its life; newest
	// wins.) Asserted by TestRefine_NewestApprovedWins.
	approved, err := s.store.ListProposalsByStatus(ctx, "approved")
	if err != nil {
		return store.Proposal{}, err
	}
	for _, p := range approved {
		if p.JobID == jobID {
			return p, nil // newest approved proposal for this job (list is created_at DESC)
		}
	}
	// No approved proposal for this intent. Refuse to build — don't let unapproved
	// content reach a live channel (prime directive #3). createChannel maps this to
	// a 422 so the caller sees a clear "approve the proposal first" signal.
	return store.Proposal{}, fmt.Errorf("no approved proposal for intent %q", jobID)
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
func lineupEntries(p suggest.Proposal) ([]schedule.LineupEntry, error) {
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
			})
		}
	}
	return out, nil
}
