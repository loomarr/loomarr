package fillerscreening

import (
	"reflect"
	"testing"
)

func TestReduceAppliesAsymmetricCascade(t *testing.T) {
	t.Parallel()
	candidates := []SpokenSafetyCandidate{{ID: "candidate-a", StartMS: 100, EndMS: 300}, {ID: "candidate-b", StartMS: 500, EndMS: 800}}
	tests := []struct {
		name     string
		evidence SpokenSafetyEvidence
		want     SpokenSafetyResult
	}{
		{name: "no candidates still requires video", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Video: VideoNoSignal}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeCandidateRejected, Reasons: []SpokenSafetyReason{}}},
		{name: "all absences reject candidate", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoNoSignal}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeCandidateRejected, Reasons: []SpokenSafetyReason{}}},
		{name: "audio detection quarantines", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioDetected, AudioAbsent), Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeQuarantine, Reasons: []SpokenSafetyReason{SpokenSafetyReasonAudioProhibitedSignal}}},
		{name: "valid presence outranks another hold", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioFailed, AudioDetected), Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeQuarantine, Reasons: []SpokenSafetyReason{SpokenSafetyReasonAudioFailure, SpokenSafetyReasonAudioProhibitedSignal}}},
		{name: "video detection quarantines", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoProhibited}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeQuarantine, Reasons: []SpokenSafetyReason{SpokenSafetyReasonVideoProhibitedSignal}}},
		{name: "audio unclear holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioUnclear, AudioAbsent), Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonAudioUnclear}}},
		{name: "audio failure holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioInvalidResponse, AudioAbsent), Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonAudioFailure}}},
		{name: "unprojectable audio presence holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioDetectedUnprojectable, AudioAbsent), Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonPresenceUnprojectable}}},
		{name: "unprojectable video presence holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoProhibitedUnprojectable}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonPresenceUnprojectable}}},
		{name: "incomplete video holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoIncomplete}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonVideoIncomplete}}},
		{name: "video failure holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoFailed}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonVideoFailure}}},
		{name: "proposal failure holds", evidence: SpokenSafetyEvidence{ProposalState: ProposalFailed, Video: VideoNotRun}, want: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonProposalFailure}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ReduceSpokenSafety(test.evidence); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Reduce()=%+v want=%+v", got, test.want)
			}
		})
	}
}

func TestReduceHoldsInvalidOrOutOfOrderEvidence(t *testing.T) {
	t.Parallel()
	valid := SpokenSafetyCandidate{ID: "candidate-a", StartMS: 100, EndMS: 300}
	tests := []SpokenSafetyEvidence{
		{},
		{ProposalState: ProposalFailed, Candidates: []SpokenSafetyCandidate{valid}, Video: VideoNotRun},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{{ID: "", StartMS: 100, EndMS: 300}}, Audio: []SpokenSafetyAudioAssessment{{State: AudioAbsent}}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{{ID: "candidate-a", StartMS: 300, EndMS: 100}}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: "candidate-a", State: AudioAbsent}}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{valid}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{valid}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: "other", State: AudioAbsent}}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{valid}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: valid.ID, State: "future"}}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{valid}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: valid.ID, State: AudioDetected}}, Video: VideoNoSignal},
		{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{valid}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: valid.ID, State: AudioAbsent}}, Video: VideoNotRun},
	}
	for index, evidence := range tests {
		if got := ReduceSpokenSafety(evidence); !reflect.DeepEqual(got, SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}}) {
			t.Fatalf("case %d: Reduce()=%+v", index, got)
		}
	}
}

func TestReduceRequiresCanonicalCandidateOrder(t *testing.T) {
	t.Parallel()
	candidates := []SpokenSafetyCandidate{{ID: "candidate-b", StartMS: 500, EndMS: 800}, {ID: "candidate-a", StartMS: 100, EndMS: 300}}
	got := ReduceSpokenSafety(SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: candidates, Audio: audio(candidates, AudioAbsent, AudioAbsent), Video: VideoNoSignal})
	if !reflect.DeepEqual(got, SpokenSafetyResult{Outcome: SpokenSafetyOutcomeHold, Reasons: []SpokenSafetyReason{SpokenSafetyReasonInvalidEvidence}}) {
		t.Fatalf("Reduce()=%+v", got)
	}
}

func audio(candidates []SpokenSafetyCandidate, states ...SpokenSafetyAudioState) []SpokenSafetyAudioAssessment {
	assessments := make([]SpokenSafetyAudioAssessment, len(candidates))
	for index := range candidates {
		assessments[index] = SpokenSafetyAudioAssessment{CandidateID: candidates[index].ID, State: states[index]}
	}
	return assessments
}
