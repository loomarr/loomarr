package records

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// AcquisitionRepairSummary projects durable unresolved repair rows for readiness. The newest
// reason is ordered by the same stable UpdatedAt/ID key used for artifact projections.
func (s *sqlStore) AcquisitionRepairSummary(ctx context.Context) (acquisition.RepairSummary, error) {
	const query = `SELECT COUNT(*), COALESCE((
		SELECT substr(repair_reason, 1, 512) FROM filler_acquisition_artifacts
		WHERE state = ? ORDER BY updated_at DESC, id DESC LIMIT 1
	), '') FROM filler_acquisition_artifacts WHERE state = ?`
	var summary acquisition.RepairSummary
	if err := s.db.QueryRowContext(ctx, s.ph(query), string(acquisition.ArtifactRepair), string(acquisition.ArtifactRepair)).Scan(&summary.Count, &summary.LatestReason); err != nil {
		return acquisition.RepairSummary{}, fmt.Errorf("summarize filler acquisition repairs: %w", err)
	}
	return summary, nil
}
