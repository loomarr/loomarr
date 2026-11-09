package split

import (
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

type structureDecisionProjectionInterval struct {
	role          Role
	disposition   Disposition
	observationID string
}

// structureDecisionProjectionAuthority is an in-memory capability derived only by replaying an
// attached artifact. It is never serialized and a complete-timeline observation cannot mint one.
type structureDecisionProjectionAuthority struct {
	artifactSHA256 string
	reducerVersion string
	intervals      map[[2]int64]structureDecisionProjectionInterval
}

func newStructureDecisionProjectionAuthority(source SourceAsset, artifact structure.Artifact) (*structureDecisionProjectionAuthority, error) {
	if err := source.validate(); err != nil {
		return nil, err
	}
	if err := structure.ValidateArtifact(artifact); err != nil {
		return nil, err
	}
	decision := artifact.Decision
	if decision.Status != structure.StatusConfirmed {
		return nil, errors.New("source structure decision is held")
	}
	if decision.Source.SHA256 != source.SHA256 || decision.Source.Bytes != source.Bytes || decision.Source.DurationMS != source.DurationMs {
		return nil, errors.New("source structure decision binds another source")
	}
	if _, err := projectedStructureKind(decision.Unit); err != nil {
		return nil, err
	}
	authority := &structureDecisionProjectionAuthority{
		artifactSHA256: artifact.SHA256,
		reducerVersion: artifact.ReducerVersion,
		intervals:      make(map[[2]int64]structureDecisionProjectionInterval, len(decision.Segments)),
	}
	end := int64(0)
	for index, segment := range decision.Segments {
		role := Role(segment.Role)
		disposition, err := projectedStructureDisposition(segment.Disposition)
		if err != nil || !ValidRole(role) || segment.StartMS != end || segment.EndMS <= segment.StartMS || segment.EndMS > source.DurationMs {
			return nil, fmt.Errorf("source structure decision interval %d is invalid", index)
		}
		if disposition == DispositionKeep && !CertifiedFillerRole(role) || disposition == DispositionDiscard && role != RoleProgrammeFragment && role != RoleNonFiller {
			return nil, fmt.Errorf("source structure decision interval %d role and disposition disagree", index)
		}
		span := [2]int64{segment.StartMS, segment.EndMS}
		if _, duplicate := authority.intervals[span]; duplicate {
			return nil, fmt.Errorf("source structure decision repeats interval %d", index)
		}
		authority.intervals[span] = structureDecisionProjectionInterval{
			role: role, disposition: disposition,
			observationID: fmt.Sprintf("%sinterval-%04d", structureDecisionObservationPrefix, index+1),
		}
		end = segment.EndMS
	}
	if len(decision.Segments) == 0 || end != source.DurationMs {
		return nil, errors.New("source structure decision intervals do not cover the source")
	}
	return authority, nil
}

// ValidateDecisionProjection proves that the proposal assessment is an exact projection
// of one replayable, confirmed independent-assessor decision. It grants no certification itself.
func ValidateDecisionProjection(assessment Assessment, artifact structure.Artifact) error {
	authority, err := newStructureDecisionProjectionAuthority(assessment.Source, artifact)
	if err != nil {
		return err
	}
	if err := validateAssessment(assessment, authority); err != nil {
		return err
	}
	decision := artifact.Decision
	kind, err := projectedStructureKind(decision.Unit)
	if err != nil || assessment.Kind != kind || len(decision.Segments) != len(assessment.Plan) {
		return errors.New("source structure decision unit or interval count does not match")
	}
	for index, decided := range decision.Segments {
		planned := assessment.Plan[index]
		disposition, err := projectedStructureDisposition(decided.Disposition)
		if err != nil || planned.StartMs != decided.StartMS || planned.EndMs != decided.EndMS ||
			planned.Role != Role(decided.Role) || planned.Disposition != disposition {
			return fmt.Errorf("source structure decision interval %d does not match", index)
		}
	}
	return nil
}

// ValidateAssessmentOrProjection accepts assessment when it either reduces strictly from its own
// observations or is an exact projection of artifact, which may be nil.
func ValidateAssessmentOrProjection(assessment Assessment, artifact *structure.Artifact) error {
	strictErr := ValidateAssessment(assessment)
	if strictErr == nil {
		return nil
	}
	if artifact != nil && ValidateDecisionProjection(assessment, *artifact) == nil {
		return nil
	}
	return strictErr
}

func projectedStructureKind(unit structure.Unit) (Kind, error) {
	switch unit {
	case structure.UnitStandalone, structure.UnitProgrammeExcerpt:
		return KindSingleUnit, nil
	case structure.UnitCompilation:
		return KindCompilationBreak, nil
	case structure.UnitProgrammeSpots:
		return KindProgrammeSpots, nil
	default:
		return "", fmt.Errorf("source structure decision unit %q cannot be projected", unit)
	}
}

func projectedStructureDisposition(disposition structure.Disposition) (Disposition, error) {
	switch disposition {
	case structure.DispositionFillerCandidate:
		return DispositionKeep, nil
	case structure.DispositionNonFiller:
		return DispositionDiscard, nil
	default:
		return "", fmt.Errorf("source structure decision disposition %q cannot be projected", disposition)
	}
}
