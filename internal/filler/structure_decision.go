package filler

import (
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

// ValidateStructureDecisionProjection proves that the proposal assessment is an exact projection
// of one replayable, confirmed independent-assessor decision. It grants no certification itself.
func ValidateStructureDecisionProjection(assessment SourceStructureAssessment, artifact fillerstructure.Artifact) error {
	if err := ValidateSourceStructureAssessment(assessment); err != nil {
		return err
	}
	if err := fillerstructure.ValidateArtifact(artifact); err != nil {
		return err
	}
	decision := artifact.Decision
	if decision.Status != fillerstructure.StatusConfirmed {
		return errors.New("source structure decision is held")
	}
	if decision.Source.SHA256 != assessment.Source.SHA256 || decision.Source.Bytes != assessment.Source.Bytes || decision.Source.DurationMS != assessment.DurationMs {
		return errors.New("source structure decision binds another source")
	}
	kind, err := projectedStructureKind(decision.Unit)
	if err != nil || assessment.Kind != kind || len(decision.Segments) != len(assessment.Plan) {
		return errors.New("source structure decision unit or interval count does not match")
	}
	for index, decided := range decision.Segments {
		planned := assessment.Plan[index]
		disposition, err := projectedStructureDisposition(decided.Disposition)
		if err != nil || planned.StartMs != decided.StartMS || planned.EndMs != decided.EndMS ||
			planned.Role != StructureSegmentRole(decided.Role) || planned.Disposition != disposition {
			return fmt.Errorf("source structure decision interval %d does not match", index)
		}
	}
	return nil
}

func projectedStructureKind(unit fillerstructure.Unit) (SourceStructureKind, error) {
	switch unit {
	case fillerstructure.UnitStandalone, fillerstructure.UnitProgrammeExcerpt:
		return StructureSingleUnit, nil
	case fillerstructure.UnitCompilation:
		return StructureCompilationBreak, nil
	case fillerstructure.UnitProgrammeSpots:
		return StructureProgrammeSpots, nil
	default:
		return "", fmt.Errorf("source structure decision unit %q cannot be projected", unit)
	}
}

func projectedStructureDisposition(disposition fillerstructure.Disposition) (StructureSegmentDisposition, error) {
	switch disposition {
	case fillerstructure.DispositionFillerCandidate:
		return StructureKeep, nil
	case fillerstructure.DispositionNonFiller:
		return StructureDiscard, nil
	default:
		return "", fmt.Errorf("source structure decision disposition %q cannot be projected", disposition)
	}
}
