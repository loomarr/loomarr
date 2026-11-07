package fillerscreening

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLedgerContractAcceptsClosedAppendOnlyPayloads(t *testing.T) {
	hash := strings.Repeat("a", 64)
	run := SpokenSafetyLedgerRun{
		ID: "run-1", ClipHash: "clip-1", AuthoritySHA256: hash, SourceSHA256: hash,
		CertificationSHA256: hash, PolicySHA256: hash, ProposerSHA256: hash, Implementation: "spoken-safety-v1",
		SourceBytes: 1024, DurationMS: 1000, CreatedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := ValidateSpokenSafetyLedgerRun(run); err != nil {
		t.Fatal(err)
	}

	evidence := SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{}, Audio: []SpokenSafetyAudioAssessment{}, Video: VideoNoSignal}
	events := []SpokenSafetyLedgerEvent{
		{ID: "event-plan", RunID: run.ID, Ordinal: 0, Kind: LedgerSourcePlanned,
			Source: &SpokenSafetySourcePlanned{Audio: SpokenSafetySpan{EndMS: 1000}, Video: SpokenSafetySpan{EndMS: 1000}}, CreatedAt: run.CreatedAt},
		{ID: "event-proposal", RunID: run.ID, Ordinal: 1, Kind: LedgerProposalCompleted,
			Proposal: &SpokenSafetyProposalCompleted{State: ProposalComplete, ProposerSHA256: hash, Candidates: []SpokenSafetyCandidate{}}, CreatedAt: run.CreatedAt},
		{ID: "event-terminal", RunID: run.ID, Ordinal: 2, Kind: LedgerTerminal,
			Terminal: &SpokenSafetyTerminalResult{Evidence: evidence, Result: ReduceSpokenSafety(evidence), EventIDs: []string{"event-plan", "event-proposal"}}, CreatedAt: run.CreatedAt},
	}
	for _, event := range events {
		raw, err := CanonicalSpokenSafetyLedgerEvent(event)
		if err != nil {
			t.Fatalf("canonicalize %s: %v", event.Kind, err)
		}
		decoded, err := DecodeSpokenSafetyLedgerEvent(event.Kind, raw)
		if err != nil {
			t.Fatalf("decode %s: %v", event.Kind, err)
		}
		decoded.ID, decoded.RunID, decoded.Ordinal, decoded.CreatedAt = event.ID, event.RunID, event.Ordinal, event.CreatedAt
		if _, err := CanonicalSpokenSafetyLedgerEvent(decoded); err != nil {
			t.Fatalf("revalidate %s: %v", event.Kind, err)
		}
	}
}

func TestLedgerContractRejectsOpenOrSensitiveShapes(t *testing.T) {
	hash := strings.Repeat("b", 64)
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	valid := SpokenSafetyLedgerEvent{ID: "event-reserve", RunID: "run-1", Ordinal: 2, Kind: LedgerInferenceReserved,
		Reserve: &SpokenSafetyInferenceReserved{
			EvaluationID: "evaluation-1", RequestSHA256: hash, RequestedProvider: "openrouter",
			RequestedModel: "model-1", UpstreamProvider: "provider-1", CapabilitySHA256: hash,
			PromptSHA256: hash, CandidateID: "candidate-1", Modalities: []string{"audio"},
			RequestedNanoUSD: 100, ReservedNanoUSD: 100,
			State: ReservationAccepted,
		}, CreatedAt: at}

	cases := map[string]func(*SpokenSafetyLedgerEvent){
		"two payloads": func(event *SpokenSafetyLedgerEvent) {
			event.Source = &SpokenSafetySourcePlanned{Audio: SpokenSafetySpan{EndMS: 1}, Video: SpokenSafetySpan{EndMS: 1}}
		},
		"free form modality":   func(event *SpokenSafetyLedgerEvent) { event.Reserve.Modalities = []string{"transcript"} },
		"unsorted modalities":  func(event *SpokenSafetyLedgerEvent) { event.Reserve.Modalities = []string{"video", "audio"} },
		"path-shaped provider": func(event *SpokenSafetyLedgerEvent) { event.Reserve.RequestedProvider = "  provider  " },
		"invalid digest":       func(event *SpokenSafetyLedgerEvent) { event.Reserve.RequestSHA256 = "/private/source.mp4" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			event := valid
			reserve := *valid.Reserve
			event.Reserve = &reserve
			mutate(&event)
			if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
				t.Fatalf("error = %v, want ErrLedgerInvalid", err)
			}
		})
	}
}

func TestLedgerOpaqueIdentityPositionsRejectSourceShapes(t *testing.T) {
	hash := strings.Repeat("c", 64)
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	badIDs := []string{"/private/source.mp4", "../source.mp4", "source.mp4", "https://source.invalid/a", `source\\file`, "source id", "source\nიდენტ", "café"}

	validRun := func() SpokenSafetyLedgerRun {
		return SpokenSafetyLedgerRun{ID: "run-1", ClipHash: "clip-1", AuthoritySHA256: hash, SourceSHA256: hash,
			CertificationSHA256: hash, PolicySHA256: hash, ProposerSHA256: hash, Implementation: "spoken-safety-v1",
			SourceBytes: 1, DurationMS: 1, CreatedAt: at}
	}
	for _, field := range []struct {
		name string
		set  func(*SpokenSafetyLedgerRun, string)
	}{
		{"run", func(run *SpokenSafetyLedgerRun, value string) { run.ID = value }},
		{"clip", func(run *SpokenSafetyLedgerRun, value string) { run.ClipHash = value }},
		{"implementation", func(run *SpokenSafetyLedgerRun, value string) { run.Implementation = value }},
	} {
		for _, value := range badIDs {
			t.Run("run/"+field.name, func(t *testing.T) {
				run := validRun()
				field.set(&run, value)
				if !errors.Is(ValidateSpokenSafetyLedgerRun(run), ErrSpokenSafetyLedgerInvalid) {
					t.Fatalf("accepted %q", value)
				}
			})
		}
	}

	validEvent := func() SpokenSafetyLedgerEvent {
		return SpokenSafetyLedgerEvent{ID: "event-1", RunID: "run-1", Ordinal: 1, Kind: LedgerInferenceReserved, CreatedAt: at,
			Reserve: &SpokenSafetyInferenceReserved{EvaluationID: "evaluation-1", RequestSHA256: hash,
				RequestedProvider: "OpenRouter", RequestedModel: "openai/gpt-5-mini.2026-08-07", UpstreamProvider: "OpenAI Provider",
				CapabilitySHA256: hash, PromptSHA256: hash, CandidateID: "candidate-1", Modalities: []string{"audio"},
				RequestedNanoUSD: 1, ReservedNanoUSD: 1, State: ReservationAccepted}}
	}
	for _, field := range []struct {
		name string
		set  func(*SpokenSafetyLedgerEvent, string)
	}{
		{"event", func(event *SpokenSafetyLedgerEvent, value string) { event.ID = value }},
		{"run", func(event *SpokenSafetyLedgerEvent, value string) { event.RunID = value }},
		{"evaluation", func(event *SpokenSafetyLedgerEvent, value string) { event.Reserve.EvaluationID = value }},
		{"candidate", func(event *SpokenSafetyLedgerEvent, value string) { event.Reserve.CandidateID = value }},
	} {
		for _, value := range badIDs {
			t.Run("reservation/"+field.name, func(t *testing.T) {
				event := validEvent()
				field.set(&event, value)
				if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
					t.Fatalf("accepted %q", value)
				}
			})
		}
	}

	for _, value := range badIDs {
		t.Run("proposal/candidate", func(t *testing.T) {
			event := SpokenSafetyLedgerEvent{ID: "event-proposal", RunID: "run-1", Ordinal: 1, Kind: LedgerProposalCompleted, CreatedAt: at,
				Proposal: &SpokenSafetyProposalCompleted{State: ProposalComplete, ProposerSHA256: hash, Candidates: []SpokenSafetyCandidate{{ID: value, EndMS: 1}}}}
			if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
				t.Fatalf("accepted %q", value)
			}
		})
		t.Run("settlement/opaque", func(t *testing.T) {
			event := SpokenSafetyLedgerEvent{ID: "event-settle", RunID: "run-1", Ordinal: 2, Kind: LedgerInferenceSettled, CreatedAt: at,
				Settle: &SpokenSafetyInferenceSettled{ReservationEventID: "reserve-1", EvaluationID: "evaluation-1", ResponseSHA256: hash,
					ResolvedProvider: "OpenRouter", ResolvedModel: "openai/gpt-5-mini.2026-08-07", UpstreamProvider: "OpenAI Provider", GenerationID: "generation-1",
					State: SettlementCompleted, Outcome: string(AudioAbsent), ChargedAmountUSD: "0", ChargeKnown: true}}
			for _, set := range []func(*SpokenSafetyInferenceSettled){
				func(settlement *SpokenSafetyInferenceSettled) { settlement.ReservationEventID = value },
				func(settlement *SpokenSafetyInferenceSettled) { settlement.EvaluationID = value },
				func(settlement *SpokenSafetyInferenceSettled) { settlement.GenerationID = value },
			} {
				copy := event
				settlement := *event.Settle
				copy.Settle = &settlement
				set(copy.Settle)
				if _, err := CanonicalSpokenSafetyLedgerEvent(copy); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
					t.Fatalf("accepted %q", value)
				}
			}
		})
		t.Run("terminal/nested", func(t *testing.T) {
			evidence := SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{{ID: value, EndMS: 1}}, Audio: []SpokenSafetyAudioAssessment{{CandidateID: value, State: AudioAbsent}}, Video: VideoNoSignal}
			event := SpokenSafetyLedgerEvent{ID: "event-terminal", RunID: "run-1", Ordinal: 2, Kind: LedgerTerminal, CreatedAt: at,
				Terminal: &SpokenSafetyTerminalResult{Evidence: evidence, Result: ReduceSpokenSafety(evidence), EventIDs: []string{"event-1"}}}
			if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
				t.Fatalf("accepted %q", value)
			}
			evidence.Candidates[0].ID, evidence.Audio[0].CandidateID = "candidate-1", "candidate-1"
			event.Terminal.Evidence, event.Terminal.Result, event.Terminal.EventIDs = evidence, ReduceSpokenSafety(evidence), []string{value}
			if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
				t.Fatalf("accepted event ID %q", value)
			}
		})
	}
}

func TestLedgerPublicIdentitiesKeepValidatedNamespaces(t *testing.T) {
	hash := strings.Repeat("d", 64)
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	reservation := SpokenSafetyLedgerEvent{ID: "event-reserve", RunID: "run-1", Ordinal: 1, Kind: LedgerInferenceReserved, CreatedAt: at,
		Reserve: &SpokenSafetyInferenceReserved{EvaluationID: "evaluation-1", RequestSHA256: hash,
			RequestedProvider: "OpenAI Provider", RequestedModel: "openai/gpt-5-mini.2026-08-07", UpstreamProvider: "Fournisseur Étendu",
			CapabilitySHA256: hash, PromptSHA256: hash, CandidateID: "candidate-1", Modalities: []string{"audio"},
			RequestedNanoUSD: 1, ReservedNanoUSD: 1, State: ReservationAccepted}}
	if _, err := CanonicalSpokenSafetyLedgerEvent(reservation); err != nil {
		t.Fatalf("public route labels: %v", err)
	}
	for _, value := range []string{"/private/provider", `provider\\name`, "provider\nname"} {
		copy := reservation
		payload := *reservation.Reserve
		copy.Reserve = &payload
		copy.Reserve.RequestedProvider = value
		if _, err := CanonicalSpokenSafetyLedgerEvent(copy); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
			t.Fatalf("accepted unsafe public label %q", value)
		}
	}

	settlement := SpokenSafetyLedgerEvent{ID: "event-settle", RunID: "run-1", Ordinal: 2, Kind: LedgerInferenceSettled, CreatedAt: at,
		Settle: &SpokenSafetyInferenceSettled{ReservationEventID: "event-reserve", EvaluationID: "evaluation-1", ResponseSHA256: hash,
			ResolvedProvider: "OpenAI Provider", ResolvedModel: "openai/gpt-5-mini.2026-08-07", UpstreamProvider: "Fournisseur Étendu", GenerationID: "generation-1",
			State: SettlementCompleted, Outcome: string(AudioAbsent), ChargedAmountUSD: "0", ChargeKnown: true}}
	if _, err := CanonicalSpokenSafetyLedgerEvent(settlement); err != nil {
		t.Fatalf("public response labels: %v", err)
	}
	failed := settlement
	failed.Settle = &SpokenSafetyInferenceSettled{ReservationEventID: "event-reserve", EvaluationID: "evaluation-1",
		ResolvedProvider: "OpenAI Provider", ResolvedModel: "openai/gpt-5-mini.2026-08-07", UpstreamProvider: "Fournisseur Étendu",
		State: SettlementFailed, Failure: FailureTransport}
	if _, err := CanonicalSpokenSafetyLedgerEvent(failed); err != nil {
		t.Fatalf("optional failed response labels: %v", err)
	}
}

func TestTerminalLedgerResultMustMatchReducer(t *testing.T) {
	evidence := SpokenSafetyEvidence{ProposalState: ProposalComplete, Candidates: []SpokenSafetyCandidate{}, Audio: []SpokenSafetyAudioAssessment{}, Video: VideoNoSignal}
	event := SpokenSafetyLedgerEvent{
		ID: "event-terminal", RunID: "run-1", Ordinal: 2, Kind: LedgerTerminal,
		Terminal:  &SpokenSafetyTerminalResult{Evidence: evidence, Result: SpokenSafetyResult{Outcome: SpokenSafetyOutcomeQuarantine, Reasons: []SpokenSafetyReason{SpokenSafetyReasonVideoProhibitedSignal}}, EventIDs: []string{"event-plan"}},
		CreatedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
	}
	if _, err := CanonicalSpokenSafetyLedgerEvent(event); !errors.Is(err, ErrSpokenSafetyLedgerInvalid) {
		t.Fatalf("error = %v, want ErrLedgerInvalid", err)
	}
}
