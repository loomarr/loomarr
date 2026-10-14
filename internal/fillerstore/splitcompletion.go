package fillerstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// CompleteSplitConfirmation is V65's single durable commit. Reversible media publication happens
// before this call; proposal consumption, parent completion, child activation, and generation selection
// either all commit or all remain at their pre-confirm values. The clip and pipeline writes go
// through the core's store.ClipTx, inside this transaction.
func (s *sqlStore) CompleteSplitConfirmation(ctx context.Context, completion filler.SplitCompletion) (int, error) {
	if completion.ProposalID == "" || completion.ClaimToken == "" || completion.ParentHash == "" || len(completion.ChildHashes) == 0 {
		return 0, errors.New("complete split confirmation: proposal, claim token, parent, and children are required")
	}
	seen := make(map[string]struct{}, len(completion.ChildHashes))
	for _, hash := range completion.ChildHashes {
		if hash == "" {
			return 0, errors.New("complete split confirmation: child hash is required")
		}
		if _, duplicate := seen[hash]; duplicate {
			return 0, fmt.Errorf("complete split confirmation: duplicate child %s", hash)
		}
		seen[hash] = struct{}{}
	}
	activate := make(map[string]struct{}, len(completion.ActivateHashes))
	for _, hash := range completion.ActivateHashes {
		if _, selected := seen[hash]; hash == "" || !selected {
			return 0, fmt.Errorf("complete split confirmation: activated child %q is not selected", hash)
		}
		if _, duplicate := activate[hash]; duplicate {
			return 0, fmt.Errorf("complete split confirmation: duplicate activated child %s", hash)
		}
		activate[hash] = struct{}{}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s: %w", completion.ProposalID, err)
	}
	defer func() { _ = tx.Rollback() }()

	var proposalParent, claimToken string
	if err := tx.QueryRowContext(ctx, s.ph(
		`SELECT clip_hash, claim_token FROM filler_split_proposals WHERE id = ?`), completion.ProposalID).
		Scan(&proposalParent, &claimToken); err != nil {
		return 0, fmt.Errorf("complete split confirmation %s read proposal: %w", completion.ProposalID, err)
	}
	if claimToken != completion.ClaimToken {
		return 0, filler.ErrProposalClaimed
	}
	if proposalParent != completion.ParentHash {
		return 0, fmt.Errorf("complete split confirmation %s: proposal parent changed", completion.ProposalID)
	}

	clips := s.db.Clips(tx)
	released, err := clips.ReleaseSplitParent(ctx, completion.ParentHash, completion.At)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s release parent: %w", completion.ProposalID, err)
	}
	if !released {
		return 0, store.ErrNotFound
	}

	settled, err := s.advancePipelineTx(ctx, tx, completion.ParentHash, filler.DispositionReview, filler.DispositionComplete, completion.At)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s settle parent pipeline: %w", completion.ProposalID, err)
	}
	if !settled {
		return 0, fmt.Errorf("complete split confirmation %s: parent pipeline is not awaiting review", completion.ProposalID)
	}
	for _, hash := range completion.ActivateHashes {
		activated, err := s.advancePipelineTx(ctx, tx, hash, filler.DispositionReview, filler.DispositionRunning, completion.At)
		if err != nil {
			return 0, fmt.Errorf("complete split confirmation %s activate child %s: %w", completion.ProposalID, hash, err)
		}
		if !activated {
			return 0, fmt.Errorf("complete split confirmation %s: child %s is not staged for review", completion.ProposalID, hash)
		}
	}

	res, err := tx.ExecContext(ctx, s.ph(`DELETE FROM filler_split_proposals WHERE id = ? AND claim_token = ?`), completion.ProposalID, completion.ClaimToken)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s consume proposal: %w", completion.ProposalID, err)
	}
	if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return 0, fmt.Errorf("complete split confirmation %s count proposal: %w", completion.ProposalID, countErr)
		}
		return 0, store.ErrNotFound
	}

	retired, err := clips.ReplaceSplitChildren(ctx, completion.ParentHash, completion.ChildHashes, completion.At)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("complete split confirmation %s: %w", completion.ProposalID, err)
	}
	return retired, nil
}
