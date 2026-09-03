package filler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	StructureSplitShadowSchemaVersion   = 1
	StructureSplitShadowContractVersion = "filler-structure-split-shadow-v1"
)

var ErrStructureSplitShadowConflict = errors.New("filler: structure split shadow decision conflicts with its content identity")

// StructureSplitShadowRepository is the append-only persistence seam. Production uses the shared
// store; tests use an in-memory adapter at the same seam.
type StructureSplitShadowRepository interface {
	PutStructureSplitShadowDecision(context.Context, StructureSplitShadowDecision) error
	GetStructureSplitShadowDecision(context.Context, string) (StructureSplitShadowDecision, bool, error)
}

// StructureSplitShadowObserver is the split stage's complete shadow interface. The stage supplies
// only the exact proposal and compatibility outcome; the module owns complete-plan evaluation,
// versioning, content identity, validation, and persistence.
type StructureSplitShadowObserver interface {
	NeedsStructureSplitObservation(context.Context, SplitProposal) (bool, error)
	ObserveStructureSplit(context.Context, SplitProposal, SplitPartition) error
}

type StructureSplitShadowSpan struct {
	StartMs    int64  `json:"startMs"`
	EndMs      int64  `json:"endMs"`
	HoldReason string `json:"holdReason,omitempty"`
}

type StructureSplitShadowOutcome struct {
	Verdict AutoSplitReject            `json:"verdict,omitempty"`
	Confirm []StructureSplitShadowSpan `json:"confirm,omitempty"`
	Hold    []StructureSplitShadowSpan `json:"hold,omitempty"`
	Discard []StructureSplitShadowSpan `json:"discard,omitempty"`
}

// StructureSplitShadowDecision preserves both decisions after the proposal is consumed. SHA256
// addresses the canonical record with ID and SHA256 empty; ID is derived from that digest.
type StructureSplitShadowDecision struct {
	SchemaVersion    int                         `json:"schemaVersion"`
	ContractVersion  string                      `json:"contractVersion"`
	ID               string                      `json:"id"`
	ProposalID       string                      `json:"proposalId"`
	ClipHash         string                      `json:"clipHash"`
	SourceSHA256     string                      `json:"sourceSha256,omitempty"`
	AssessmentSHA256 string                      `json:"assessmentSha256,omitempty"`
	PolicyVersion    string                      `json:"policyVersion"`
	Legacy           StructureSplitShadowOutcome `json:"legacy"`
	Certified        StructureSplitShadowOutcome `json:"certified"`
	ObservedAt       time.Time                   `json:"observedAt"`
	SHA256           string                      `json:"sha256"`
}

// StructureSplitShadow is the deep dual-evaluation module used during rollout.
type StructureSplitShadow struct {
	repository      StructureSplitShadowRepository
	auto            *AutoSplitPolicy
	certification   *StructureCertificationPolicy
	minClipDuration func() time.Duration
	policyVersion   string
}

func NewStructureSplitShadow(repository StructureSplitShadowRepository, auto *AutoSplitPolicy, certification *StructureCertificationPolicy, minClipDuration func() time.Duration, policyVersion string) (*StructureSplitShadow, error) {
	policyVersion = strings.TrimSpace(policyVersion)
	if repository == nil || auto == nil || certification == nil || certification.AssessmentCertified == nil || certification.ScreeningCertified == nil || minClipDuration == nil || policyVersion == "" || len(policyVersion) > 128 {
		return nil, fmt.Errorf("structure split shadow requires repository, complete policies, clip floor, and bounded policy identity")
	}
	return &StructureSplitShadow{
		repository: repository, auto: auto, certification: certification,
		minClipDuration: minClipDuration, policyVersion: policyVersion,
	}, nil
}

func (s *StructureSplitShadow) ObserveStructureSplit(ctx context.Context, proposal SplitProposal, legacy SplitPartition) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("structure split shadow is unavailable")
	}
	certified := CertifiedAutoConfirmable(proposal, s.auto, s.certification, s.minClipDuration())
	decision, err := newStructureSplitShadowDecision(proposal, legacy, certified, s.policyVersion)
	if err != nil {
		return err
	}
	if err := s.repository.PutStructureSplitShadowDecision(ctx, decision); err != nil {
		return fmt.Errorf("record structure split shadow: %w", err)
	}
	return nil
}

// NeedsStructureSplitObservation lets startup requeue an older review proposal exactly when its
// current proposal/policy decision is absent. The full document is compared when the id exists so
// corrupt or conflicting evidence fails closed instead of suppressing a fresh observation.
func (s *StructureSplitShadow) NeedsStructureSplitObservation(ctx context.Context, proposal SplitProposal) (bool, error) {
	if s == nil || s.repository == nil {
		return false, fmt.Errorf("structure split shadow is unavailable")
	}
	legacy := AutoConfirmable(proposal, s.auto, s.minClipDuration())
	certified := CertifiedAutoConfirmable(proposal, s.auto, s.certification, s.minClipDuration())
	expected, err := newStructureSplitShadowDecision(proposal, legacy, certified, s.policyVersion)
	if err != nil {
		return false, err
	}
	existing, found, err := s.repository.GetStructureSplitShadowDecision(ctx, expected.ID)
	if err != nil {
		return false, fmt.Errorf("read structure split shadow: %w", err)
	}
	if !found {
		return true, nil
	}
	if !reflect.DeepEqual(existing, expected) {
		return false, ErrStructureSplitShadowConflict
	}
	return false, nil
}

func newStructureSplitShadowDecision(proposal SplitProposal, legacy, certified SplitPartition, policyVersion string) (StructureSplitShadowDecision, error) {
	if strings.TrimSpace(proposal.ID) == "" || strings.TrimSpace(proposal.ClipHash) == "" || proposal.CreatedAt.IsZero() || len(proposal.Segments) == 0 {
		return StructureSplitShadowDecision{}, fmt.Errorf("structure split shadow requires a complete proposal identity and segments")
	}
	legacyOutcome := structureSplitShadowOutcome(legacy)
	certifiedOutcome := structureSplitShadowOutcome(certified)
	if err := structureSplitShadowOutcomeCoversProposal(legacyOutcome, proposal.Segments); err != nil {
		return StructureSplitShadowDecision{}, fmt.Errorf("legacy structure split outcome: %w", err)
	}
	if err := structureSplitShadowOutcomeCoversProposal(certifiedOutcome, proposal.Segments); err != nil {
		return StructureSplitShadowDecision{}, fmt.Errorf("certified structure split outcome: %w", err)
	}
	observedAt := proposal.CreatedAt.UTC()
	sourceSHA, assessmentSHA := proposal.Source.SHA256, ""
	if proposal.Structure != nil {
		if err := ValidateSourceStructureAssessment(*proposal.Structure); err != nil || proposal.Structure.Source != proposal.Source {
			return StructureSplitShadowDecision{}, fmt.Errorf("structure split shadow proposal assessment is invalid")
		}
		assessmentSHA = proposal.Structure.SHA256
		observedAt = proposal.Structure.AssessedAt.UTC()
	}
	decision := StructureSplitShadowDecision{
		SchemaVersion: StructureSplitShadowSchemaVersion, ContractVersion: StructureSplitShadowContractVersion,
		ProposalID: proposal.ID, ClipHash: proposal.ClipHash, SourceSHA256: sourceSHA,
		AssessmentSHA256: assessmentSHA, PolicyVersion: strings.TrimSpace(policyVersion),
		Legacy: legacyOutcome, Certified: certifiedOutcome, ObservedAt: observedAt,
	}
	decision.SHA256 = StructureSplitShadowDecisionSHA256(decision)
	decision.ID = "split-shadow-" + decision.SHA256
	if err := ValidateStructureSplitShadowDecision(decision); err != nil {
		return StructureSplitShadowDecision{}, err
	}
	return decision, nil
}

func structureSplitShadowOutcome(partition SplitPartition) StructureSplitShadowOutcome {
	outcome := StructureSplitShadowOutcome{Verdict: partition.Verdict()}
	outcome.Confirm = structureSplitShadowSpans(partition.Confirm, "", false)
	outcome.Hold = structureSplitShadowSpans(partition.Hold, string(outcome.Verdict), partition.Reject != AutoSplitOK)
	outcome.Discard = structureSplitShadowSpans(partition.Discard, "", false)
	return outcome
}

func structureSplitShadowSpans(segments []SplitSegment, fallbackReason string, overrideReason bool) []StructureSplitShadowSpan {
	spans := make([]StructureSplitShadowSpan, 0, len(segments))
	for _, segment := range segments {
		reason := strings.TrimSpace(segment.HoldReason)
		if overrideReason || reason == "" {
			reason = fallbackReason
		}
		spans = append(spans, StructureSplitShadowSpan{StartMs: segment.StartMs, EndMs: segment.EndMs, HoldReason: reason})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].StartMs != spans[j].StartMs {
			return spans[i].StartMs < spans[j].StartMs
		}
		return spans[i].EndMs < spans[j].EndMs
	})
	return spans
}

func structureSplitShadowOutcomeCoversProposal(outcome StructureSplitShadowOutcome, segments []SplitSegment) error {
	expected := make(map[[2]int64]struct{}, len(segments))
	for _, segment := range segments {
		key := [2]int64{segment.StartMs, segment.EndMs}
		if _, duplicate := expected[key]; duplicate {
			return fmt.Errorf("proposal repeats span %d..%d", segment.StartMs, segment.EndMs)
		}
		expected[key] = struct{}{}
	}
	seen := make(map[[2]int64]struct{}, len(segments))
	for _, group := range [][]StructureSplitShadowSpan{outcome.Confirm, outcome.Hold, outcome.Discard} {
		for _, span := range group {
			key := [2]int64{span.StartMs, span.EndMs}
			if _, exists := expected[key]; !exists {
				return fmt.Errorf("span %d..%d is absent from the proposal", span.StartMs, span.EndMs)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("span %d..%d appears more than once", span.StartMs, span.EndMs)
			}
			seen[key] = struct{}{}
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("outcome covers %d of %d proposal spans", len(seen), len(expected))
	}
	return nil
}

func ValidateStructureSplitShadowDecision(decision StructureSplitShadowDecision) error {
	if decision.SchemaVersion != StructureSplitShadowSchemaVersion || decision.ContractVersion != StructureSplitShadowContractVersion || decision.ID == "" || decision.ProposalID == "" || decision.ClipHash == "" || strings.TrimSpace(decision.PolicyVersion) != decision.PolicyVersion || decision.PolicyVersion == "" || len(decision.PolicyVersion) > 128 || decision.ObservedAt.IsZero() || decision.ObservedAt != decision.ObservedAt.UTC() {
		return fmt.Errorf("structure split shadow decision identity is invalid")
	}
	if decision.SourceSHA256 != "" && !isContentHash(decision.SourceSHA256) || decision.AssessmentSHA256 != "" && !isContentHash(decision.AssessmentSHA256) {
		return fmt.Errorf("structure split shadow decision source or assessment digest is invalid")
	}
	if err := validateStructureSplitShadowOutcome(decision.Legacy); err != nil {
		return fmt.Errorf("structure split shadow legacy outcome: %w", err)
	}
	if err := validateStructureSplitShadowOutcome(decision.Certified); err != nil {
		return fmt.Errorf("structure split shadow certified outcome: %w", err)
	}
	if decision.SHA256 == "" || decision.SHA256 != StructureSplitShadowDecisionSHA256(decision) || decision.ID != "split-shadow-"+decision.SHA256 {
		return fmt.Errorf("structure split shadow decision digest is invalid")
	}
	return nil
}

func validateStructureSplitShadowOutcome(outcome StructureSplitShadowOutcome) error {
	count := 0
	seen := make(map[[2]int64]struct{})
	for _, group := range [][]StructureSplitShadowSpan{outcome.Confirm, outcome.Hold, outcome.Discard} {
		for index, span := range group {
			if span.StartMs < 0 || span.EndMs <= span.StartMs || index > 0 && (span.StartMs < group[index-1].StartMs || span.StartMs == group[index-1].StartMs && span.EndMs < group[index-1].EndMs) {
				return fmt.Errorf("contains an invalid or non-canonical span")
			}
			key := [2]int64{span.StartMs, span.EndMs}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("repeats a span")
			}
			seen[key] = struct{}{}
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("contains no spans")
	}
	return nil
}

func StructureSplitShadowDecisionSHA256(decision StructureSplitShadowDecision) string {
	decision.ID, decision.SHA256 = "", ""
	decision.Legacy.Confirm = slices.Clone(decision.Legacy.Confirm)
	decision.Legacy.Hold = slices.Clone(decision.Legacy.Hold)
	decision.Legacy.Discard = slices.Clone(decision.Legacy.Discard)
	decision.Certified.Confirm = slices.Clone(decision.Certified.Confirm)
	decision.Certified.Hold = slices.Clone(decision.Certified.Hold)
	decision.Certified.Discard = slices.Clone(decision.Certified.Discard)
	raw, err := json.Marshal(decision)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var _ StructureSplitShadowObserver = (*StructureSplitShadow)(nil)
