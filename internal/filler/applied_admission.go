package filler

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
)

// AppliedAdmissionMediaResolver resolves the immutable catalog identity to the current absolute
// playback path. The production adapter owns the store lookup and clip-root containment check.
type AppliedAdmissionMediaResolver interface {
	ResolveAppliedAdmissionMedia(context.Context, string) (string, error)
}

// AppliedAdmission is the deep terminal module between an applied semantic decision and catalog
// publication. Its one interface owns current playback reprojection, complete release replay,
// live-rights revalidation, and the final atomic persistence handoff.
type AppliedAdmission struct {
	resolver      AppliedAdmissionMediaResolver
	summary       *SegmentScreeningSummaryService
	evidence      SegmentScreeningSummaryEvidenceReader
	certification *SegmentScreeningCertification
	committer     fillerdecision.AppliedActionRepository
}

func NewAppliedAdmission(
	resolver AppliedAdmissionMediaResolver,
	evidence SegmentScreeningSummaryEvidenceReader,
	certification *SegmentScreeningCertification,
	committer fillerdecision.AppliedActionRepository,
) (*AppliedAdmission, error) {
	if resolver == nil || evidence == nil || certification == nil ||
		certification.AuthoritySHA256() == "" || committer == nil {
		return nil, fmt.Errorf("%w: complete terminal admission authority is required", fillerdecision.ErrAppliedUnavailable)
	}
	summary, err := NewSegmentScreeningSummaryService(evidence)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", fillerdecision.ErrAppliedUnavailable, err)
	}
	return &AppliedAdmission{
		resolver: resolver, summary: summary, evidence: evidence,
		certification: certification, committer: committer,
	}, nil
}

func (a *AppliedAdmission) ActOnAppliedFillerDecision(
	ctx context.Context,
	record fillerdecision.Record,
	action fillerdecision.Action,
) error {
	if a == nil || a.resolver == nil || a.summary == nil || a.evidence == nil ||
		a.certification == nil || a.committer == nil {
		return fillerdecision.ErrAppliedUnavailable
	}
	if err := fillerdecision.ValidateRecord(record); err != nil {
		return err
	}
	if err := fillerdecision.ValidateAction(action); err != nil {
		return err
	}
	if record.ApplicationMode != fillerdecision.ApplicationModeApplied || action.DecisionID != record.ID {
		return fillerdecision.ErrActionMode
	}
	if appliedActionPublishes(action) {
		if err := a.verifyCurrentRelease(ctx, record); err != nil {
			return fmt.Errorf("%w: %v", fillerdecision.ErrAppliedUnavailable, err)
		}
	}
	return a.committer.CommitAppliedFillerDecisionAction(ctx, action)
}

func (a *AppliedAdmission) verifyCurrentRelease(ctx context.Context, record fillerdecision.Record) error {
	if a.certification.AuthoritySHA256() != record.ReleaseAuthoritySHA256 {
		return fmt.Errorf("release authority does not match the applied decision")
	}
	mediaPath, err := a.resolver.ResolveAppliedAdmissionMedia(ctx, record.ClipHash)
	if err != nil {
		return fmt.Errorf("resolve current playback: %w", err)
	}
	summary, err := a.summary.ReadSegmentScreeningSummary(ctx, record.ClipHash, mediaPath)
	if err != nil {
		return fmt.Errorf("reproduce current screening: %w", err)
	}
	if summary.State != ScreeningSummaryAvailable || summary.Outcome != ScreenPass ||
		summary.EvidenceSHA256 != record.ScreeningEvidenceSHA256 {
		return fmt.Errorf("current playback does not reproduce the applied screening pass")
	}
	aggregate, err := a.evidence.GetSegmentScreeningEvidence(ctx, record.ScreeningEvidenceSHA256)
	if err != nil {
		return fmt.Errorf("reopen applied screening aggregate: %w", err)
	}
	if aggregate.SHA256 != record.ScreeningEvidenceSHA256 || !aggregate.Passes() {
		return fmt.Errorf("applied screening aggregate is not an exact five-axis pass")
	}
	if err := a.certification.Verify(ctx, aggregate); err != nil {
		return fmt.Errorf("replay terminal release: %w", err)
	}
	return nil
}

func appliedActionPublishes(action fillerdecision.Action) bool {
	return action.Kind == fillerdecision.ActionAdmit || action.Kind == fillerdecision.ActionRestore ||
		action.Kind == fillerdecision.ActionCorrect && action.CorrectedVerdict == filleradmission.VerdictAdmit
}

var _ fillerdecision.AppliedActionExecutor = (*AppliedAdmission)(nil)
