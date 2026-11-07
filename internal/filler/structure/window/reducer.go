package window

import (
	"github.com/loomarr/loomarr/internal/filler/structure"
)

// ReducerInput projects a validated media set into the provider-neutral assessment-input manifest.
func ReducerInput(set MediaSet) (structure.AssessmentInput, error) {
	if err := ValidateMediaSet(set); err != nil {
		return structure.AssessmentInput{}, err
	}
	media := make([]structure.AssessmentMedia, len(set.Windows))
	for ordinal := range set.Windows {
		media[ordinal] = set.Windows[ordinal].Media
	}
	return structure.NewWindowMediaSetInput(set.Plan.Source, set.Plan.SHA256, media)
}

// ReducerCandidate turns one replay-validated family stitch into one source-level candidate.
// It does not treat the constituent windows as independent votes.
func ReducerCandidate(result StitchResult) (structure.AssessmentInput, structure.Candidate, error) {
	if err := ValidateStitchResult(result); err != nil {
		return structure.AssessmentInput{}, structure.Candidate{}, err
	}
	input, err := ReducerInput(result.MediaSet)
	if err != nil {
		return structure.AssessmentInput{}, structure.Candidate{}, err
	}
	profile := result.Assessor
	assessor := structure.Assessor{
		ID: profile.ID, ModelFamily: profile.ModelFamily, Provider: profile.Provider,
		Model: profile.Model, ModelDigest: profile.ModelDigest, CapabilitySHA256: profile.CapabilitySHA256,
		PromptVersion: profile.PromptVersion, EvidenceContract: profile.EvidenceContract,
		AssessmentSHA256: result.SHA256,
	}
	failure := ""
	segments := result.Segments
	if result.Status == StitchHeld {
		failure, segments = result.HoldReason, nil
	}
	candidate, err := structure.NewCandidate(result.MediaSet.Plan.Source, input.SHA256, assessor, failure, segments)
	if err != nil {
		return structure.AssessmentInput{}, structure.Candidate{}, err
	}
	return input, candidate, nil
}
