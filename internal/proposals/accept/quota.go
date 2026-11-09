package accept

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// Usage is a user's standing against their pending-acquisition cap (§11).
type Usage struct {
	// Pending is the deduplicated count of acquisition keys across the user's APPROVED
	// proposals whose titles have not reached a terminal state.
	Pending int
	// Limit is the effective cap: the user's own quota, or the suggest.max_acquisitions
	// default when theirs is 0 (§11: "0 ⇒ the default").
	Limit int
}

// Exceeded reports whether adding `n` more acquisitions would break the cap. A Limit of
// zero after defaulting means "no cap configured anywhere", which is treated as
// unlimited rather than as a cap of zero — a misread that would silently stop every
// auto-approval on a fresh install.
func (u Usage) Exceeded(n int) bool {
	if u.Limit <= 0 {
		return false
	}
	return u.Pending+n > u.Limit
}

// QuotaStore is the slice of the store quota accounting needs.
type QuotaStore interface {
	ListProposalsByCreator(ctx context.Context, userID string) ([]store.Proposal, error)
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// PendingFor computes a user's usage (§11).
//
// Titles carry no requester — a title is keyed by identity, so two users wanting the same
// film is ONE row — so attribution runs through proposals, which carry created_by. A
// title both users asked for counts against both, which is the honest reading: each of
// them caused it.
//
// This walks the user's proposals rather than issuing one query, because the acquisition
// list lives inside proposal_json. At household scale (§1: ≤20 users) that is a handful
// of rows, and the alternative — a denormalized ledger — is a second source of truth for
// something the proposals already record.
func PendingFor(ctx context.Context, st QuotaStore, userID string, limit int) (Usage, error) {
	usage := Usage{Limit: limit}
	if userID == "" {
		return usage, nil
	}

	proposals, err := st.ListProposalsByCreator(ctx, userID)
	if err != nil {
		return Usage{}, fmt.Errorf("quota: list proposals for %s: %w", userID, err)
	}

	seen := map[string]bool{}
	for _, p := range proposals {
		// Only APPROVED proposals have spent anything. A submitted one is a plan, and a
		// denied one never became an acquisition.
		if p.Status != "approved" {
			continue
		}
		// The cap is a safety boundary. An approved audit row the store cannot decode fails the
		// list above, so its pending spend is never guessed and unattended approval fails closed.
		for _, a := range p.Body.Acquisitions {
			key, kerr := acquisitionKey(a)
			if kerr != nil || seen[string(key)] {
				continue
			}
			seen[string(key)] = true

			rec, gerr := st.GetTitle(ctx, key)
			if errors.Is(gerr, store.ErrNotFound) {
				// Not found means the title was never created or has been pruned;
				// either way it is not pending against this user.
				continue
			}
			if gerr != nil {
				return Usage{}, fmt.Errorf("quota: read title %s: %w", key, gerr)
			}
			if !rec.State.Terminal() {
				usage.Pending++
			}
		}
	}
	return usage, nil
}

// acquisitionKey derives the provisioning key for a proposed acquisition.
func acquisitionKey(a proposals.ProposalItem) (provision.Key, error) {
	title := provision.Title{
		MediaType: provision.MediaType(a.MediaType),
		TMDBID:    a.TMDBID, TVDBID: a.TVDBID, Name: a.Name, Year: a.Year, Seasons: a.Seasons,
	}
	return title.Key()
}
