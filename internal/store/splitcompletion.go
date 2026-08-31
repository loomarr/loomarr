package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler"
)

// CompleteSplitConfirmation is V65's single durable commit. Reversible media publication happens
// before this call; proposal consumption, parent filing, child activation, and generation selection
// either all commit or all remain at their pre-confirm values.
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

	tx, err := s.db.BeginTx(ctx, nil)
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

	res, err := tx.ExecContext(ctx, s.ph(
		`UPDATE clips SET is_composite = ?, held = ?, auto_filed = ?, updated_at = ? WHERE hash = ? AND held = ?`),
		true, false, false, epoch(completion.At), completion.ParentHash, true)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s file parent: %w", completion.ProposalID, err)
	}
	if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return 0, fmt.Errorf("complete split confirmation %s count parent: %w", completion.ProposalID, countErr)
		}
		return 0, ErrNotFound
	}

	res, err = tx.ExecContext(ctx, s.ph(
		`UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ? WHERE clip_hash = ? AND disposition = ?`),
		string(filler.DispositionFiled), epoch(completion.At), completion.ParentHash, string(filler.DispositionReview))
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s file parent pipeline: %w", completion.ProposalID, err)
	}
	if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return 0, fmt.Errorf("complete split confirmation %s count parent pipeline: %w", completion.ProposalID, countErr)
		}
		return 0, fmt.Errorf("complete split confirmation %s: parent pipeline is not awaiting review", completion.ProposalID)
	}
	for _, hash := range completion.ActivateHashes {
		res, err := tx.ExecContext(ctx, s.ph(
			`UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ? WHERE clip_hash = ? AND disposition = ?`),
			string(filler.DispositionRunning), epoch(completion.At), hash, string(filler.DispositionReview))
		if err != nil {
			return 0, fmt.Errorf("complete split confirmation %s activate child %s: %w", completion.ProposalID, hash, err)
		}
		if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
			if countErr != nil {
				return 0, fmt.Errorf("complete split confirmation %s count child %s: %w", completion.ProposalID, hash, countErr)
			}
			return 0, fmt.Errorf("complete split confirmation %s: child %s is not staged for review", completion.ProposalID, hash)
		}
	}

	res, err = tx.ExecContext(ctx, s.ph(`DELETE FROM filler_split_proposals WHERE id = ? AND claim_token = ?`), completion.ProposalID, completion.ClaimToken)
	if err != nil {
		return 0, fmt.Errorf("complete split confirmation %s consume proposal: %w", completion.ProposalID, err)
	}
	if n, countErr := res.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return 0, fmt.Errorf("complete split confirmation %s count proposal: %w", completion.ProposalID, countErr)
		}
		return 0, ErrNotFound
	}

	retired, err := s.replaceSplitChildrenTx(ctx, tx, completion.ParentHash, completion.ChildHashes, completion.At)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("complete split confirmation %s: %w", completion.ProposalID, err)
	}
	return retired, nil
}
