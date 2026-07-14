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
	return lineupEntries(p.Lineup)
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
	approved, err := s.store.ListProposalsByStatus(ctx, "approved")
	if err != nil {
		return store.Proposal{}, err
	}
	for _, p := range approved {
		if p.JobID == jobID {
			return p, nil // a job yields one proposal; first match is the binding
		}
	}
	// No approved proposal for this intent. Refuse to build — don't let unapproved
	// content reach a live channel (prime directive #3). createChannel maps this to
	// a 422 so the caller sees a clear "approve the proposal first" signal.
	return store.Proposal{}, fmt.Errorf("no approved proposal for intent %q", jobID)
}

// lineupEntries maps a proposal's in-library lineup to scheduler entries. Only
// in-library items become lineup entries here; missing titles are acquisitions
// (a separate path) and only join the lineup once they land and backfill runs.
func lineupEntries(items []suggest.ProposalItem) ([]schedule.LineupEntry, error) {
	out := make([]schedule.LineupEntry, 0, len(items))
	for _, it := range items {
		if !it.InLibrary {
			continue // acquisitions backfill in later; not a playable slot yet
		}
		key, err := provision.KeyFromWebhook(it.MediaType, it.TMDBID, it.TVDBID)
		if err != nil {
			return nil, fmt.Errorf("lineup key for %q: %w", it.Name, err)
		}
		out = append(out, schedule.LineupEntry{
			Key:   key,
			Title: it.Name,
			// DurationMs is left 0 (unknown) here; the reconciler resolves real
			// runtime from the library when it computes desired slots (§9).
		})
	}
	return out, nil
}
