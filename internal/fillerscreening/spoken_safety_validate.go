package fillerscreening

import (
	"encoding/hex"
	"slices"
	"strings"
)

// ValidSpokenSafetyPolicyRuleID reports whether value is an opaque 96-bit rule identifier.
func ValidSpokenSafetyPolicyRuleID(value string) bool {
	if len(value) != len("rule-")+24 || !strings.HasPrefix(value, "rule-") {
		return false
	}
	_, err := hex.DecodeString(value[len("rule-"):])
	return err == nil
}

func validateEvidence(evidence SpokenSafetyEvidence) ([]SpokenSafetyReason, bool) {
	if evidence.ProposalState != ProposalComplete && evidence.ProposalState != ProposalFailed {
		return []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}, false
	}
	if evidence.ProposalState == ProposalFailed {
		if len(evidence.Candidates) != 0 || len(evidence.Audio) != 0 || evidence.Video != VideoNotRun {
			return []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}, false
		}
		return nil, true
	}
	if !validCandidates(evidence.Candidates) || !validAudio(evidence.Candidates, evidence.Audio) {
		return []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}, false
	}

	allAbsent := true
	for _, assessment := range evidence.Audio {
		if assessment.State != AudioAbsent {
			allAbsent = false
			break
		}
	}
	if allAbsent {
		if !validCompletedVideoState(evidence.Video) {
			return []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}, false
		}
	} else if evidence.Video != VideoNotRun {
		return []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}, false
	}
	return nil, true
}

func validCandidates(candidates []SpokenSafetyCandidate) bool {
	seen := make(map[string]struct{}, len(candidates))
	for index, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" || candidate.StartMS < 0 || candidate.EndMS <= candidate.StartMS {
			return false
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return false
		}
		seen[candidate.ID] = struct{}{}
		if index > 0 {
			previous := candidates[index-1]
			if candidate.StartMS < previous.StartMS || candidate.StartMS == previous.StartMS && (candidate.EndMS < previous.EndMS || candidate.EndMS == previous.EndMS && candidate.ID <= previous.ID) {
				return false
			}
		}
	}
	return true
}

func validAudio(candidates []SpokenSafetyCandidate, audio []SpokenSafetyAudioAssessment) bool {
	if len(audio) != len(candidates) {
		return false
	}
	for index, assessment := range audio {
		if assessment.CandidateID != candidates[index].ID || !validAudioState(assessment.State) ||
			!validStoredMatchedRuleIDs(assessment.State, assessment.MatchedRuleIDs) {
			return false
		}
	}
	return true
}

// Historical terminal events predate rule attribution, so nil remains valid
// durable evidence. Certification treats nil attribution as a miss, never a hit.
func validStoredMatchedRuleIDs(state SpokenSafetyAudioState, ruleIDs []string) bool {
	if ruleIDs == nil {
		return true
	}
	if !slices.IsSorted(ruleIDs) || len(slices.Compact(slices.Clone(ruleIDs))) != len(ruleIDs) ||
		slices.ContainsFunc(ruleIDs, func(id string) bool { return !ValidSpokenSafetyPolicyRuleID(id) }) {
		return false
	}
	if state == AudioAbsent || state == AudioFailed || state == AudioInvalidResponse {
		return len(ruleIDs) == 0
	}
	return true
}

func validAudioState(state SpokenSafetyAudioState) bool {
	switch state {
	case AudioDetected, AudioDetectedUnprojectable, AudioAbsent, AudioUnclear, AudioFailed, AudioInvalidResponse:
		return true
	default:
		return false
	}
}

func validCompletedVideoState(state SpokenSafetyVideoState) bool {
	switch state {
	case VideoProhibited, VideoProhibitedUnprojectable, VideoNoSignal, VideoIncomplete, VideoFailed, VideoInvalidResponse:
		return true
	default:
		return false
	}
}
