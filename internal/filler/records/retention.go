package records

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/research"
)

// The filler tables' retention (#2528): fixed product policy, like notifications, so there is no
// setting to get wrong. Each window outlasts every read of the rows it removes.
const (
	// OrphanGrace is how old a row must be before it may go with its clip, so work written while a
	// clip is being re-keyed or re-filed is never caught mid-move. It is measured from the row,
	// not from when the clip left: the catalog keeps no record of that.
	OrphanGrace = 30 * 24 * time.Hour
	// FillerAiringHistory matches the per-clip trim (eight weeks): an orphan's airings older than
	// this are what that trim would have removed had the clip aired again.
	FillerAiringHistory = 8 * 7 * 24 * time.Hour
	// DownloadHistory is how long a finished download run with nothing left to show is kept.
	DownloadHistory = 90 * 24 * time.Hour
	// DecisionHistory is how long a dismissed pull, or the evidence of a split proposal that is
	// gone, is kept.
	DecisionHistory = 180 * 24 * time.Hour
	// CallHistory is how long a settled model call is kept. Older than this, a source assessed
	// again starts a fresh lifetime budget, and the daily ceiling only ever reads today.
	CallHistory = 180 * 24 * time.Hour
	// ResearchMonths is how many months of web-search counts are kept; only this month's is read.
	ResearchMonths = 12
)

// PurgeSpentFillerRecords deletes the filler rows nothing will read again and returns how many.
// Every statement keeps what a live read needs:
//
//   - a pass, report or ready event stays while its clip exists (it is what stops paid work, or a
//     publication, from happening twice), and a superseded research report goes only once a newer
//     one of the same kind exists, since a cited report outranks a newer guess;
//   - a download artifact with the provider's id is the fetcher's memory of what it already has,
//     so it is never deleted; nor is one still staged, published or under repair;
//   - a run goes only when nothing points at it, and an approved pull only with its run;
//   - a reserved evaluation, or an open or budget-held window call, is in flight and stays.
//
// Nothing is deleted for leaving the catalog while the catalog is empty. The sync prunes every
// clip when the drop folder lists nothing, which is what an unmounted drive looks like, and the
// clips come back under the same hashes once it is mounted; their passes and tags must still be
// there.
//
// Children go before parents: none of these foreign keys cascades.
func (s *sqlStore) PurgeSpentFillerRecords(ctx context.Context, now time.Time) (int, error) {
	orphan, download := now.Add(-OrphanGrace).Unix(), now.Add(-DownloadHistory).Unix()
	decision, call := now.Add(-DecisionHistory).Unix(), now.Add(-CallHistory).Unix()
	airedMs := now.Add(-FillerAiringHistory).UnixMilli()
	month := now.UTC().AddDate(0, -ResearchMonths, 0).Format("2006-01")
	gone := func(column string) string {
		return `EXISTS (SELECT 1 FROM clips) AND NOT EXISTS (SELECT 1 FROM clips c WHERE c.hash = ` + column + `)`
	}
	clipGone := func(table string) string { return gone(table + `.clip_hash`) }
	doomedRun := `status IN ('success', 'error') AND completed_at < ?
		AND NOT EXISTS (SELECT 1 FROM filler_acquisition_artifacts a WHERE a.acquisition_id = filler_acquisition_runs.id)
		AND NOT EXISTS (SELECT 1 FROM filler_clip_pipeline p WHERE p.acquisition_id = filler_acquisition_runs.id)`
	steps := []struct {
		what string
		sql  string
		args []any
	}{
		{"web-search months", `DELETE FROM filler_research_web_usage WHERE month < ?`, []any{month}},
		{"enrichment passes", `DELETE FROM filler_enrichment_passes WHERE completed_at < ? AND ` +
			clipGone("filler_enrichment_passes"), []any{orphan}},
		{"orphan research reports", `DELETE FROM filler_context_reports WHERE completed_at < ? AND ` +
			clipGone("filler_context_reports"), []any{orphan}},
		{"superseded research reports", `DELETE FROM filler_context_reports
			WHERE input_revision <> (SELECT c.enrichment_revision FROM clips c WHERE c.hash = filler_context_reports.clip_hash)
			  AND EXISTS (SELECT 1 FROM filler_context_reports n
			      WHERE n.clip_hash = filler_context_reports.clip_hash
			        AND (n.producer = ?) = (filler_context_reports.producer = ?)
			        AND (n.completed_at > filler_context_reports.completed_at
			             OR (n.completed_at = filler_context_reports.completed_at
			                 AND n.producer_version > filler_context_reports.producer_version)))`,
			[]any{research.ProducerObservation, research.ProducerObservation}},
		{"ready events", `DELETE FROM filler_ready_events WHERE created_at < ? AND ` +
			clipGone("filler_ready_events"), []any{orphan}},
		{"enrichment facts", `DELETE FROM filler_enrichment_axes WHERE updated_at < ? AND ` +
			clipGone("filler_enrichment_axes"), []any{orphan}},
		{"clip tags", `DELETE FROM clip_tags WHERE ` + clipGone("clip_tags"), nil},
		{"filler airings", `DELETE FROM filler_airings WHERE recorded_at_ms < ? AND ` +
			clipGone("filler_airings"), []any{airedMs}},
		{"clip variant pairs", `DELETE FROM filler_clip_variants WHERE ` + gone("filler_clip_variants.a_hash") +
			` AND NOT EXISTS (SELECT 1 FROM clips c WHERE c.hash = filler_clip_variants.b_hash)`, nil},
		{"split shadow decisions", `DELETE FROM filler_split_shadow_decisions WHERE observed_at < ?
			AND NOT EXISTS (SELECT 1 FROM filler_split_proposals p WHERE p.id = filler_split_shadow_decisions.proposal_id)`,
			[]any{decision}},
		{"structure window calls", `DELETE FROM filler_structure_window_call_ledger
			WHERE state = 'settled' AND requested_at < ?`, []any{call}},
		{"inference evaluations", `DELETE FROM filler_inference_evaluations WHERE state <> ? AND created_at < ?
			AND NOT EXISTS (SELECT 1 FROM filler_structure_window_call_ledger l
			    WHERE l.evaluation_id = filler_inference_evaluations.id)`, []any{string(InferenceReserved), call}},
		{"download artifacts", `DELETE FROM filler_acquisition_artifacts WHERE state = 'consumed' AND remote_id = ''
			AND updated_at < ? AND ` + clipGone("filler_acquisition_artifacts"), []any{download}},
		{"pull commits", `DELETE FROM filler_pull_commits WHERE acquisition_id IN (
			SELECT id FROM filler_acquisition_runs WHERE ` + doomedRun + `)`, []any{download}},
		{"approved pulls", `DELETE FROM filler_pulls WHERE status = 'approved' AND id IN (
			SELECT pull_id FROM filler_acquisition_runs WHERE pull_id <> '' AND ` + doomedRun + `)
			AND NOT EXISTS (SELECT 1 FROM filler_pull_commits c WHERE c.pull_id = filler_pulls.id)`, []any{download}},
		{"download runs", `DELETE FROM filler_acquisition_runs WHERE ` + doomedRun, []any{download}},
		{"dismissed pulls", `DELETE FROM filler_pulls WHERE status = 'dismissed' AND decided_at < ?`, []any{decision}},
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("purge spent filler records: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	total := 0
	for _, step := range steps {
		res, err := tx.ExecContext(ctx, s.ph(step.sql), step.args...)
		if err != nil {
			return 0, fmt.Errorf("purge spent %s: %w", step.what, err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("purge spent filler records: %w", err)
	}
	return total, nil
}
