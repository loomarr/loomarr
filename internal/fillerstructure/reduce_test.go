package fillerstructure

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestReduceConfirmsOnlyIndependentCompleteTimelineAgreement(t *testing.T) {
	request := fixtureRequest()
	request.Candidates[0].Segments[0].EndMS = 4_000
	request.Candidates[0].Segments[1].StartMS = 4_000
	request.Candidates[1].Segments[0].EndMS = 5_000
	request.Candidates[1].Segments[1].StartMS = 5_000
	decision := Reduce(request)
	if decision.Status != StatusConfirmed || decision.Unit != UnitCompilation || len(decision.Segments) != 2 || decision.Segments[0].EndMS != 4_500 || decision.Segments[0].Disposition != DispositionFillerCandidate || decision.Segments[1].Role != RolePromo || !slices.Equal(decision.ReasonCodes, []string{ReasonAgreement}) {
		t.Fatalf("decision=%+v", decision)
	}
}

// The measured #1069 case: both families find the same cuts and agree the span is filler, but one
// calls a PSA a commercial. Structure confirms; the disputed role stays absent, never voted.
func TestReduceConfirmsSubtypeDisagreementWithoutChoosingARole(t *testing.T) {
	request := fixtureRequest()
	request.Candidates[1].Segments[0].Role = RolePSA
	decision := Reduce(request)
	if decision.Status != StatusConfirmed || len(decision.Segments) != 2 || !slices.Equal(decision.ReasonCodes, []string{ReasonAgreement}) {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.Segments[0].Disposition != DispositionFillerCandidate || decision.Segments[0].Role != "" {
		t.Fatalf("disputed interval = %+v, want a filler candidate with no role", decision.Segments[0])
	}
	if decision.Segments[1].Role != RolePromo {
		t.Fatalf("unanimous interval = %+v, want its role kept", decision.Segments[1])
	}
	if decision.Candidates[0].Segments[0].Role != RoleCommercial || decision.Candidates[1].Segments[0].Role != RolePSA {
		t.Fatalf("candidate observations were rewritten: %+v", decision.Candidates)
	}
}

func TestReduceConfirmsProgrammeAgainstNonFillerAsADiscardWithoutARole(t *testing.T) {
	request := fixtureRequest()
	request.Candidates[0].Unit, request.Candidates[1].Unit = UnitProgrammeSpots, UnitProgrammeSpots
	request.Candidates[0].Segments[1].Role = RoleProgrammeFragment
	request.Candidates[1].Segments[1].Role = RoleNonFiller
	decision := Reduce(request)
	if decision.Status != StatusConfirmed || decision.Segments[1].Disposition != DispositionNonFiller || decision.Segments[1].Role != "" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestReduceConfirmsStandaloneSubtypeDisagreementWithoutARole(t *testing.T) {
	request := fixtureRequest()
	for index, role := range []Role{RoleCommercial, RolePSA} {
		candidate := &request.Candidates[index]
		candidate.Unit, candidate.Role = UnitStandalone, role
		candidate.Segments = []Segment{{StartMS: 0, EndMS: 10_000, Role: role}}
	}
	decision := Reduce(request)
	if decision.Status != StatusConfirmed || decision.Unit != UnitStandalone || decision.Role != "" ||
		len(decision.Segments) != 1 || decision.Segments[0].Role != "" || decision.Segments[0].Disposition != DispositionFillerCandidate {
		t.Fatalf("decision=%+v", decision)
	}
	request.Candidates[1].Role, request.Candidates[1].Segments[0].Role = RoleCommercial, RoleCommercial
	if decision := Reduce(request); decision.Status != StatusConfirmed || decision.Role != RoleCommercial || decision.Segments[0].Role != RoleCommercial {
		t.Fatalf("unanimous standalone = %+v", decision)
	}
}

func TestReduceRoleDisagreementReplaysThroughTheArtifact(t *testing.T) {
	request := fixtureRequest()
	request.Candidates[1].Segments[0].Role = RolePSA
	artifact, err := NewArtifact(request, time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Decision.Status != StatusConfirmed || ValidateArtifact(artifact) != nil {
		t.Fatalf("artifact = %+v", artifact.Decision)
	}
	artifact.Decision.Segments[0].Role = RoleCommercial
	artifact.SHA256 = ArtifactSHA256(artifact)
	if ValidateArtifact(artifact) == nil {
		t.Fatal("an artifact that chose the first assessor's role replayed as valid")
	}
}

func TestReduceInvalidInputHoldsWithoutPanicking(t *testing.T) {
	decision := Reduce(Request{})
	if decision.Status != StatusHeld || !slices.Equal(decision.ReasonCodes, []string{ReasonInvalidCandidate}) || len(decision.Segments) != 0 {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestReduceHoldsRatherThanVotingAcrossConflicts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		reason string
	}{
		{name: "same family", mutate: func(request *Request) { request.Candidates[1].Assessor.ModelFamily = "family-a" }, reason: ReasonInvalidCandidate},
		{name: "wrong source", mutate: func(request *Request) { request.Candidates[1].Source.SHA256 = strings.Repeat("c", 64) }, reason: ReasonInvalidCandidate},
		{name: "wrong input", mutate: func(request *Request) { request.Candidates[1].InputSHA256 = strings.Repeat("d", 64) }, reason: ReasonInvalidCandidate},
		{name: "operational failure", mutate: func(request *Request) {
			request.Candidates[1].Failure, request.Candidates[1].Unit, request.Candidates[1].Segments = "timeout", "", nil
		}, reason: ReasonOperationalFailure},
		{name: "unit", mutate: func(request *Request) { request.Candidates[1].Unit = UnitProgrammeSpots }, reason: ReasonUnitDisagreement},
		{name: "filler against programme", mutate: func(request *Request) {
			request.Candidates[1].Segments[0].Role = RoleProgrammeFragment
		}, reason: ReasonIntervalDisposition},
		{name: "filler against non-filler", mutate: func(request *Request) { request.Candidates[1].Segments[1].Role = RoleNonFiller }, reason: ReasonIntervalDisposition},
		{name: "unresolved role", mutate: func(request *Request) { request.Candidates[1].Segments[0].Role = RoleAmbiguous }, reason: ReasonUnresolvedInterval},
		{name: "both unresolved", mutate: func(request *Request) {
			request.Candidates[0].Segments[1].Role = RoleUnusable
			request.Candidates[1].Segments[1].Role = RoleUnusable
		}, reason: ReasonUnresolvedInterval},
		{name: "count", mutate: func(request *Request) {
			request.Candidates[1].Segments = []Segment{{StartMS: 0, EndMS: 10_000, Role: RoleCommercial}}
		}, reason: ReasonIntervalCount},
		{name: "boundary", mutate: func(request *Request) {
			request.Candidates[1].Segments[0].EndMS = 7_001
			request.Candidates[1].Segments[1].StartMS = 7_001
		}, reason: ReasonBoundary},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := fixtureRequest()
			test.mutate(&request)
			decision := Reduce(request)
			if decision.Status != StatusHeld || !slices.Contains(decision.ReasonCodes, test.reason) || len(decision.Segments) != 0 {
				t.Fatalf("decision=%+v", decision)
			}
		})
	}
}

func TestReduceGivesOneBoundaryVotePerModelFamily(t *testing.T) {
	request := fixtureRequest()
	duplicate := request.Candidates[0]
	duplicate.Segments = slices.Clone(duplicate.Segments)
	duplicate.Assessor.ID = "assessor-c"
	duplicate.Assessor.AssessmentSHA256 = strings.Repeat("d", 64)
	duplicate.Segments[0].EndMS = 6_000
	duplicate.Segments[1].StartMS = 6_000
	request.Candidates = append(request.Candidates, duplicate)
	decision := Reduce(request)
	if decision.Status != StatusConfirmed || decision.Segments[0].EndMS != 5_250 {
		t.Fatalf("same-family route became an extra vote: %+v", decision)
	}
}

func fixtureRequest() Request {
	source := Source{SHA256: strings.Repeat("a", 64), Bytes: 2_048, DurationMS: 10_000}
	media := AssessmentMedia{SHA256: strings.Repeat("e", 64), Bytes: 1_024, DurationMS: 10_000, ProfileSHA256: strings.Repeat("f", 64), LineageSHA256: strings.Repeat("d", 64)}
	input, err := NewCompleteVideoInput(source, media)
	if err != nil {
		panic(err)
	}
	candidate := func(id, family, digest string) Candidate {
		return Candidate{
			Source: source, InputSHA256: input.SHA256,
			Assessor: Assessor{
				ID: id, ModelFamily: family, Provider: "provider", Model: "model",
				ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
				PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
				AssessmentSHA256: strings.Repeat(digest, 64),
			},
			Unit:     UnitCompilation,
			Segments: []Segment{{StartMS: 0, EndMS: 5_000, Role: RoleCommercial}, {StartMS: 5_000, EndMS: 10_000, Role: RolePromo}},
		}
	}
	return Request{Source: source, Input: input, BoundaryToleranceMS: 2_000, Candidates: []Candidate{candidate("assessor-a", "family-a", "1"), candidate("assessor-b", "family-b", "2")}}
}
