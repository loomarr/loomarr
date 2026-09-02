package main

// This file is the portable part of the throwaway prototype. It models the
// cascade decision independently of the terminal and OpenRouter transport.

type decision string

const (
	decisionDetected decision = "detected"
	decisionAbsent   decision = "absent"
	decisionUnclear  decision = "unclear"
	decisionFailure  decision = "failure"
)

type observation struct {
	Alias             string       `json:"alias"`
	ExpectedLabel     string       `json:"expectedLabel"`
	Slices            []string     `json:"slices"`
	Decision          decision     `json:"decision"`
	Audibility        string       `json:"audibility,omitempty"`
	MatchedRuleIDs    []string     `json:"matchedRuleIds,omitempty"`
	Disposition       string       `json:"disposition"`
	Correct           bool         `json:"correct"`
	RequestSHA256     string       `json:"requestSha256,omitempty"`
	ResponseSHA256    string       `json:"responseSha256,omitempty"`
	GenerationIDHash  string       `json:"generationIdHash,omitempty"`
	SourceAudioSHA256 string       `json:"sourceAudioSha256,omitempty"`
	WindowAudioSHA256 string       `json:"windowAudioSha256,omitempty"`
	SourceVideoSHA256 string       `json:"sourceVideoSha256,omitempty"`
	VisualAssessment  string       `json:"visualAssessment,omitempty"`
	SpokenAssessment  string       `json:"spokenLanguageAssessment,omitempty"`
	ObservedFlags     []safetyFlag `json:"observedFlags,omitempty"`
	Intervals         []window     `json:"intervals,omitempty"`
	ChargedNanoUSD    int64        `json:"chargedNanoUsd"`
	FailureKind       string       `json:"failureKind,omitempty"`
	FailureDetail     string       `json:"failureDetail,omitempty"`
}

type prototypeState struct {
	Question              string        `json:"question"`
	Mode                  string        `json:"mode"`
	Model                 string        `json:"model"`
	CanonicalModel        string        `json:"canonicalModel"`
	ProviderSlug          string        `json:"providerSlug"`
	ReasoningEffort       string        `json:"reasoningEffort"`
	SnapshotSHA256        string        `json:"snapshotSha256"`
	AuthoritySHA256       string        `json:"authoritySha256"`
	PacketsSHA256         string        `json:"packetsSha256"`
	PolicySHA256          string        `json:"policySha256"`
	KWSIDsSHA256          string        `json:"kwsIdsSha256,omitempty"`
	KWSResultsSHA256      string        `json:"kwsResultsSha256,omitempty"`
	RealDiagnosticSHA256  string        `json:"realDiagnosticSha256,omitempty"`
	CascadeReportSHA256   string        `json:"cascadeReportSha256,omitempty"`
	PreparedPacketsSHA256 string        `json:"preparedPacketsSha256,omitempty"`
	VideoManifestSHA256   string        `json:"videoManifestSha256,omitempty"`
	ChallengeResultSHA256 string        `json:"challengeResultSha256,omitempty"`
	PromptSHA256          string        `json:"promptSha256,omitempty"`
	PlannedCases          int           `json:"plannedCases"`
	CompletedCases        int           `json:"completedCases"`
	Requests              int           `json:"requests"`
	ChargedNanoUSD        int64         `json:"chargedNanoUsd"`
	Detected              int           `json:"detected"`
	Absent                int           `json:"absent"`
	Unclear               int           `json:"unclear"`
	Failures              int           `json:"failures"`
	PositiveCases         int           `json:"positiveCases"`
	PositiveDetected      int           `json:"positiveDetected"`
	PositiveMissed        int           `json:"positiveMissed"`
	CleanCases            int           `json:"cleanCases"`
	CleanAbsent           int           `json:"cleanAbsent"`
	CleanHeld             int           `json:"cleanHeld"`
	KnownPositiveCases    int           `json:"knownPositiveCases"`
	KnownPositiveHeld     int           `json:"knownPositiveHeld"`
	KnownPositiveMissed   int           `json:"knownPositiveMissed"`
	UnlabelledCases       int           `json:"unlabelledCases"`
	UnlabelledRetained    int           `json:"unlabelledRetained"`
	UnlabelledRejected    int           `json:"unlabelledRejected"`
	UnlabelledHeld        int           `json:"unlabelledHeld"`
	SafeToExpand          bool          `json:"safeToExpand"`
	ProductionAuthority   bool          `json:"productionAuthority"`
	Observations          []observation `json:"observations"`
}

func reduce(state prototypeState, next observation) prototypeState {
	state.CompletedCases++
	state.Requests++
	state.ChargedNanoUSD += next.ChargedNanoUSD

	switch next.Decision {
	case decisionDetected:
		state.Detected++
		next.Disposition = "prohibited_hold"
	case decisionAbsent:
		state.Absent++
		next.Disposition = "candidate_rejected"
	case decisionUnclear:
		state.Unclear++
		next.Disposition = "coverage_hold"
	default:
		state.Failures++
		next.Decision = decisionFailure
		next.Disposition = "operational_hold"
	}

	switch next.ExpectedLabel {
	case "positive":
		state.PositiveCases++
		if next.Decision == decisionDetected {
			state.PositiveDetected++
			next.Correct = true
		} else {
			state.PositiveMissed++
		}
	case "clean":
		state.CleanCases++
		if next.Decision == decisionAbsent {
			state.CleanAbsent++
			next.Correct = true
		} else {
			state.CleanHeld++
		}
	case "known_positive":
		state.KnownPositiveCases++
		if next.Decision == decisionDetected {
			state.KnownPositiveHeld++
			next.Correct = true
		} else {
			state.KnownPositiveMissed++
		}
	default:
		state.UnlabelledCases++
		switch next.Decision {
		case decisionDetected:
			state.UnlabelledRetained++
		case decisionAbsent:
			state.UnlabelledRejected++
		default:
			state.UnlabelledHeld++
		}
	}

	state.Observations = append(state.Observations, next)
	state.SafeToExpand = state.Mode == "pilot" && state.CompletedCases == state.PlannedCases &&
		state.Failures == 0 && state.PositiveMissed == 0 && state.CleanHeld == 0
	state.ProductionAuthority = false
	return state
}
