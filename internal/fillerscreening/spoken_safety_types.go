package fillerscreening

// The spoken-safety cascade is the fail-closed complete-source shadow evaluation and its
// append-only execution ledger. It never grants filler admission, ingestion, scheduling, or
// training authority.

// SpokenSafetyOutcome is the closed shadow disposition of one complete-source evaluation.
type SpokenSafetyOutcome string

const (
	SpokenSafetyOutcomeQuarantine        SpokenSafetyOutcome = "quarantine"
	SpokenSafetyOutcomeHold              SpokenSafetyOutcome = "hold"
	SpokenSafetyOutcomeCandidateRejected SpokenSafetyOutcome = "candidate_rejected"
)

// SpokenSafetyReason identifies why the reducer selected an outcome without carrying
// source text or restricted policy values.
type SpokenSafetyReason string

const (
	SpokenSafetyReasonAudioProhibitedSignal SpokenSafetyReason = "audio_prohibited_signal"
	SpokenSafetyReasonVideoProhibitedSignal SpokenSafetyReason = "video_prohibited_signal"
	SpokenSafetyReasonProposalFailure       SpokenSafetyReason = "proposal_failure"
	SpokenSafetyReasonAudioUnclear          SpokenSafetyReason = "audio_unclear"
	SpokenSafetyReasonAudioFailure          SpokenSafetyReason = "audio_failure"
	SpokenSafetyReasonVideoIncomplete       SpokenSafetyReason = "video_incomplete"
	SpokenSafetyReasonVideoFailure          SpokenSafetyReason = "video_failure"
	SpokenSafetyReasonPresenceUnprojectable SpokenSafetyReason = "presence_unprojectable"
	SpokenSafetyReasonInvalidEvidence       SpokenSafetyReason = "invalid_evidence"
)

// SpokenSafetyProposalState is the terminal state of the local complete-source proposal step.
type SpokenSafetyProposalState string

const (
	ProposalComplete SpokenSafetyProposalState = "complete"
	ProposalFailed   SpokenSafetyProposalState = "failed"
)

// SpokenSafetyAudioState is the closed result of adjudicating one proposed interval.
type SpokenSafetyAudioState string

const (
	AudioDetected              SpokenSafetyAudioState = "detected"
	AudioDetectedUnprojectable SpokenSafetyAudioState = "detected_unprojectable"
	AudioAbsent                SpokenSafetyAudioState = "absent"
	AudioUnclear               SpokenSafetyAudioState = "unclear"
	AudioFailed                SpokenSafetyAudioState = "failed"
	AudioInvalidResponse       SpokenSafetyAudioState = "invalid_response"
)

// SpokenSafetyVideoState is the closed result of complete-source video/audio corroboration.
type SpokenSafetyVideoState string

const (
	VideoNotRun                  SpokenSafetyVideoState = "not_run"
	VideoProhibited              SpokenSafetyVideoState = "prohibited"
	VideoProhibitedUnprojectable SpokenSafetyVideoState = "prohibited_unprojectable"
	VideoNoSignal                SpokenSafetyVideoState = "no_signal"
	VideoIncomplete              SpokenSafetyVideoState = "incomplete"
	VideoFailed                  SpokenSafetyVideoState = "failed"
	VideoInvalidResponse         SpokenSafetyVideoState = "invalid_response"
)

// SpokenSafetyCandidate is one opaque source-relative interval from the local proposer.
type SpokenSafetyCandidate struct {
	ID      string
	StartMS int64
	EndMS   int64
}

// SpokenSafetyAudioAssessment binds one adjudication result to its proposed candidate.
type SpokenSafetyAudioAssessment struct {
	CandidateID    string
	State          SpokenSafetyAudioState
	MatchedRuleIDs []string
}

// SpokenSafetyEvidence is the complete ordered input to deterministic cascade reduction.
type SpokenSafetyEvidence struct {
	ProposalState SpokenSafetyProposalState
	Candidates    []SpokenSafetyCandidate
	Audio         []SpokenSafetyAudioAssessment
	Video         SpokenSafetyVideoState
}

// SpokenSafetyResult is evidence only. It deliberately contains no admission permission.
type SpokenSafetyResult struct {
	Outcome SpokenSafetyOutcome
	Reasons []SpokenSafetyReason
}
