package screening

import "slices"

// ReduceSpokenSafety applies the asymmetric spoken-safety state machine. Valid presence
// outranks holds; holds outrank a candidate rejection. Invalid input is itself
// a hold and can never be reduced to a negative decision.
func ReduceSpokenSafety(evidence SpokenSafetyEvidence) SpokenSafetyResult {
	reasons, valid := validateEvidence(evidence)
	if !valid {
		return result(SpokenSafetyOutcomeHold, reasons...)
	}
	if evidence.ProposalState == ProposalFailed {
		return result(SpokenSafetyOutcomeHold, SpokenSafetyReasonProposalFailure)
	}

	var quarantine, holds []SpokenSafetyReason
	for _, assessment := range evidence.Audio {
		switch assessment.State {
		case AudioDetected:
			quarantine = append(quarantine, SpokenSafetyReasonAudioProhibitedSignal)
		case AudioDetectedUnprojectable:
			holds = append(holds, SpokenSafetyReasonPresenceUnprojectable)
		case AudioAbsent:
		case AudioUnclear:
			holds = append(holds, SpokenSafetyReasonAudioUnclear)
		case AudioFailed, AudioInvalidResponse:
			holds = append(holds, SpokenSafetyReasonAudioFailure)
		}
	}
	if len(quarantine) > 0 {
		return result(SpokenSafetyOutcomeQuarantine, append(quarantine, holds...)...)
	}
	if len(holds) > 0 {
		return result(SpokenSafetyOutcomeHold, holds...)
	}

	switch evidence.Video {
	case VideoProhibited:
		return result(SpokenSafetyOutcomeQuarantine, SpokenSafetyReasonVideoProhibitedSignal)
	case VideoProhibitedUnprojectable:
		return result(SpokenSafetyOutcomeHold, SpokenSafetyReasonPresenceUnprojectable)
	case VideoNoSignal:
		return result(SpokenSafetyOutcomeCandidateRejected)
	case VideoIncomplete:
		return result(SpokenSafetyOutcomeHold, SpokenSafetyReasonVideoIncomplete)
	case VideoFailed, VideoInvalidResponse:
		return result(SpokenSafetyOutcomeHold, SpokenSafetyReasonVideoFailure)
	default:
		return result(SpokenSafetyOutcomeHold, SpokenSafetyReasonInvalidEvidence)
	}
}

func result(outcome SpokenSafetyOutcome, reasons ...SpokenSafetyReason) SpokenSafetyResult {
	slices.Sort(reasons)
	reasons = slices.Compact(reasons)
	if reasons == nil {
		reasons = []SpokenSafetyReason{}
	}
	return SpokenSafetyResult{Outcome: outcome, Reasons: reasons}
}
