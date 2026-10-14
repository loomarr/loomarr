package fillerstore

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// CommitConditioningPublication closes the catalog half of the owner-bound filesystem saga. The
// pending sidecar has already proved the exact source/target byte pair; this checks that the
// publication is still pending under an owner and names this target, then has the core adopt the
// target in one transaction (store.AdoptConditionedClip). Any mismatch, here or in the catalog
// rows, is filler.ErrConditioningOwnershipMismatch, which holds the clip for review.
func (s *sqlStore) CommitConditioningPublication(ctx context.Context, publication filler.ConditioningPublication, target store.Clip) error {
	if publication.State != "pending" || publication.Owner == "" || publication.SourceHash == "" ||
		publication.TargetHash == "" || publication.TargetHash != target.Hash {
		return filler.ErrConditioningOwnershipMismatch
	}
	err := s.core.AdoptConditionedClip(ctx, publication.SourceHash, target)
	if errors.Is(err, store.ErrConditionedClipMismatch) {
		return filler.ErrConditioningOwnershipMismatch
	}
	return err
}
