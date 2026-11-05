package fillerscreening

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
)

// AirworthinessEvaluator is the pure policy seam. It owns the audience rules and the exact
// axis profiles whose certification claims are allowed to satisfy coverage.
type AirworthinessEvaluator struct {
	profile         AirworthinessProfile
	axisProfiles    map[AirworthinessAxis]AirworthinessAxisProfile
	authoritySHA256 string
}

// NewAirworthinessEvaluator constructs one evaluator from the intended audience profile and the
// three immutable safety-axis profiles selected by a release authority.
func NewAirworthinessEvaluator(profile AirworthinessProfile, profiles []AirworthinessAxisProfile) (*AirworthinessEvaluator, error) {
	if !validProfile(profile) || len(profiles) != maximumEvidenceAxes {
		return nil, fmt.Errorf("airworthiness requires one known audience profile and three axis profiles")
	}
	byAxis := make(map[AirworthinessAxis]AirworthinessAxisProfile, len(profiles))
	for _, value := range profiles {
		normalized, err := normalizeAxisProfile(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := byAxis[normalized.Axis]; duplicate {
			return nil, fmt.Errorf("airworthiness repeats an axis profile")
		}
		byAxis[normalized.Axis] = normalized
	}
	canonical := make([]AirworthinessAxisProfile, 0, len(axisOrder))
	for _, axis := range axisOrder {
		value, exists := byAxis[axis]
		if !exists {
			return nil, fmt.Errorf("airworthiness is missing an axis profile")
		}
		canonical = append(canonical, value)
	}
	identity := struct {
		PolicyVersion     string                     `json:"policyVersion"`
		VocabularyVersion string                     `json:"vocabularyVersion"`
		Profile           AirworthinessProfile       `json:"profile"`
		Axes              []AirworthinessAxisProfile `json:"axes"`
	}{AirworthinessPolicyVersion, AirworthinessVocabularyVersion, profile, canonical}
	raw, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("marshal airworthiness authority identity: %w", err)
	}
	digest := sha256.Sum256(raw)
	return &AirworthinessEvaluator{profile: profile, axisProfiles: byAxis, authoritySHA256: hex.EncodeToString(digest[:])}, nil
}

// AuthoritySHA256 identifies the exact audience rules, vocabulary, and axis
// profiles consumed by this evaluator.
func (e *AirworthinessEvaluator) AuthoritySHA256() string {
	if e == nil {
		return ""
	}
	return e.authoritySHA256
}

type evaluatedObservation struct {
	axisEvidence AirworthinessAxisEvidence
	observation  AirworthinessObservation
	axis         AirworthinessAxis
}

// Evaluate returns one canonical public decision. A valid rejecting
// observation wins before incomplete coverage or review-level observations;
// only full certified negative coverage can pass.
func (e *AirworthinessEvaluator) Evaluate(document AirworthinessDocument) AirworthinessDecision {
	if e == nil {
		return finalizeDecision(AirworthinessDecision{
			SchemaVersion: AirworthinessDecisionSchemaVersion, ContractVersion: AirworthinessDecisionContractVersion,
			SubjectSHA256: document.SubjectSHA256, PolicyVersion: AirworthinessPolicyVersion,
			VocabularyVersion: AirworthinessVocabularyVersion, Verdict: VerdictHold,
			ReasonCodes: []AirworthinessReason{ReasonEvidenceInvalid},
		})
	}
	decision := AirworthinessDecision{
		SchemaVersion: AirworthinessDecisionSchemaVersion, ContractVersion: AirworthinessDecisionContractVersion,
		SubjectSHA256: document.SubjectSHA256, Profile: e.profile, PolicyVersion: AirworthinessPolicyVersion,
		VocabularyVersion: AirworthinessVocabularyVersion, AuthoritySHA256: e.authoritySHA256,
	}
	if !validProfile(e.profile) || !validSHA256(e.authoritySHA256) {
		decision.Profile = ""
		decision.Verdict = VerdictHold
		decision.ReasonCodes = []AirworthinessReason{ReasonEvidenceInvalid}
		return finalizeDecision(decision)
	}
	if document.SchemaVersion != AirworthinessEvidenceSchemaVersion || document.ContractVersion != AirworthinessEvidenceContractVersion ||
		!validSHA256(document.SubjectSHA256) || document.DurationMS <= 0 ||
		document.DurationMS > maximumRenderedDurationMS || len(document.Axes) != maximumEvidenceAxes {
		decision.Verdict = VerdictHold
		decision.ReasonCodes = []AirworthinessReason{ReasonEvidenceInvalid}
		return finalizeDecision(decision)
	}

	byAxis := make(map[AirworthinessAxis]AirworthinessAxisEvidence, len(document.Axes))
	structureInvalid := false
	for _, axisEvidence := range document.Axes {
		if !validAxis(axisEvidence.Profile.Axis) {
			structureInvalid = true
			continue
		}
		if _, duplicate := byAxis[axisEvidence.Profile.Axis]; duplicate {
			structureInvalid = true
			continue
		}
		byAxis[axisEvidence.Profile.Axis] = axisEvidence
	}

	seenObservationIDs := make(map[string]struct{})
	var observations []evaluatedObservation
	invalidAxes := make(map[AirworthinessAxis]struct{})
	coverageAxes := make(map[AirworthinessAxis]struct{})
	evidenceSHA256s := make([]string, 0, len(axisOrder))
	for _, axis := range axisOrder {
		axisEvidence, exists := byAxis[axis]
		if !exists {
			structureInvalid = true
			invalidAxes[axis] = struct{}{}
			coverageAxes[axis] = struct{}{}
			continue
		}
		if validSHA256(axisEvidence.EvidenceSHA256) {
			evidenceSHA256s = append(evidenceSHA256s, axisEvidence.EvidenceSHA256)
		}
		inspected, invalid := e.inspectAxis(document, axis, axisEvidence, seenObservationIDs)
		if invalid {
			invalidAxes[axis] = struct{}{}
		}
		observations = append(observations, inspected...)
		if axisEvidence.Coverage != CoverageComplete {
			coverageAxes[axis] = struct{}{}
		}
	}
	decision.EvidenceSHA256s = evidenceSHA256s
	for _, item := range observations {
		decision.ObservedFlags = append(decision.ObservedFlags, item.observation.Flag)
		action := policyActionFor(e.profile, item.observation)
		if action == policyReject {
			decision.Triggers = append(decision.Triggers, triggerFor(item, EffectReject))
		}
	}
	if len(decision.Triggers) > 0 {
		decision.Verdict = VerdictReject
		decision.ReasonCodes = []AirworthinessReason{ReasonProhibitedObservation}
		return finalizeDecision(decision)
	}

	reasons := make([]AirworthinessReason, 0, 5)
	heldAxes := make(map[AirworthinessAxis]struct{})
	if structureInvalid || len(invalidAxes) > 0 {
		reasons = append(reasons, ReasonEvidenceInvalid)
		copyAxisSet(heldAxes, invalidAxes)
	}
	if len(coverageAxes) > 0 {
		reasons = append(reasons, ReasonCoverageIncomplete)
		copyAxisSet(heldAxes, coverageAxes)
	}
	certificationAxes := e.incompleteCertificationAxes()
	if len(certificationAxes) > 0 {
		reasons = append(reasons, ReasonCertificationIncomplete)
		copyAxisSet(heldAxes, certificationAxes)
	}
	for _, item := range observations {
		if policyActionFor(e.profile, item.observation) == policyReview {
			decision.Triggers = append(decision.Triggers, triggerFor(item, EffectReview))
		}
	}
	if len(decision.Triggers) > 0 {
		reasons = append(reasons, ReasonObservationRequiresReview)
	}
	if e.profile == ProfileRestrictedArchive {
		reasons = append(reasons, ReasonRestrictedArchive)
	}
	if len(reasons) > 0 {
		decision.Verdict = VerdictHold
		decision.ReasonCodes = reasons
		for _, axis := range axisOrder {
			if _, held := heldAxes[axis]; held {
				decision.HeldAxes = append(decision.HeldAxes, axis)
			}
		}
		return finalizeDecision(decision)
	}
	decision.Verdict = VerdictPass
	decision.ReasonCodes = []AirworthinessReason{ReasonEvidenceSatisfied}
	return finalizeDecision(decision)
}

func (e *AirworthinessEvaluator) inspectAxis(document AirworthinessDocument, axis AirworthinessAxis, evidence AirworthinessAxisEvidence, seenIDs map[string]struct{}) ([]evaluatedObservation, bool) {
	expected := e.axisProfiles[axis]
	baseValid := evidence.SubjectSHA256 == document.SubjectSHA256 && sameAxisProfile(evidence.Profile, expected) &&
		validSHA256(evidence.EvidenceSHA256) && validCoverage(evidence.Coverage) &&
		len(evidence.Observations) <= maximumObservations
	if !baseValid || (evidence.Coverage == CoverageFailed || evidence.Coverage == CoverageConflict) && len(evidence.Observations) > 0 {
		return nil, true
	}
	observations := make([]evaluatedObservation, 0, len(evidence.Observations))
	invalid := false
	for _, observation := range evidence.Observations {
		_, duplicate := seenIDs[observation.ID]
		if !validObservation(observation, axis, document.DurationMS) || duplicate ||
			!slices.Contains(expected.CertifiedFlags, observation.Flag) {
			invalid = true
			continue
		}
		seenIDs[observation.ID] = struct{}{}
		observations = append(observations, evaluatedObservation{axisEvidence: evidence, observation: observation, axis: axis})
	}
	return observations, invalid
}

func (e *AirworthinessEvaluator) incompleteCertificationAxes() map[AirworthinessAxis]struct{} {
	result := make(map[AirworthinessAxis]struct{})
	for _, flag := range policyFlags {
		for _, axis := range flagOwners(flag) {
			if !slices.Contains(e.axisProfiles[axis].CertifiedFlags, flag) {
				result[axis] = struct{}{}
			}
		}
	}
	return result
}

func triggerFor(item evaluatedObservation, effect AirworthinessEffect) AirworthinessTrigger {
	value := item.observation
	return AirworthinessTrigger{
		ObservationID: value.ID, Axis: item.axis, Flag: value.Flag, Severity: value.Severity,
		Context: value.Context, StartMS: value.StartMS, EndMS: value.EndMS,
		EvidenceSHA256: item.axisEvidence.EvidenceSHA256, Effect: effect,
	}
}

func copyAxisSet(destination, source map[AirworthinessAxis]struct{}) {
	for axis := range source {
		destination[axis] = struct{}{}
	}
}
