package fillersafety

import (
	"strings"
	"testing"
	"time"
)

// validHostedCallCommands are one reservation and its settlement that both validators accept, so each
// mutation below is the only reason a command is rejected.
func validHostedCallCommands() (HostedCallReservation, HostedCallSettlement) {
	digest := strings.Repeat("a", 64)
	at := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	reservation := HostedCallReservation{
		EventID: "run-1-reserve-0", RunID: "run-1", EvaluationID: "evaluation-1", ClipHash: "clip-1",
		CandidateID: "candidate-1", RequestSHA256: digest, Role: "adjudicator", Rung: "audio",
		RequestedProvider: "openrouter", RequestedModel: "audio-model", UpstreamProvider: "upstream",
		Modalities:      []string{"audio"},
		DerivativeBytes: 4096, DerivativeDurationMS: 700, RequestedNanoUSD: 1_000,
		Budget: HostedCallBudget{PerClipNanoUSD: 10_000, PerDayNanoUSD: 100_000, PerRunNanoUSD: 10_000},
		Versions: HostedCallVersions{
			EvidenceSHA256: digest, ExtractorSHA256: digest, PromptSHA256: digest, SchemaSHA256: digest,
			TaxonomySHA256: digest, CertificationSHA256: digest, PolicySHA256: digest, CapabilitySHA256: digest,
		},
		CreatedAt: at,
	}
	settlement := HostedCallSettlement{
		EventID: "run-1-settle-0", RunID: "run-1", ReservationEventID: reservation.EventID,
		ResponseSHA256: digest, ResolvedProvider: "openrouter", ResolvedModel: "audio-model",
		UpstreamProvider: "upstream", GenerationID: "generation-1", Outcome: string(AudioAbsent),
		ChargedAmountUSD: "0.000001", ChargedNanoUSD: 1_000, ChargeKnown: true,
		PromptTokens: 10, CompletionTokens: 5, LatencyMS: 250, Ordinal: 1, CreatedAt: at,
	}
	return reservation, settlement
}

func TestHostedCallContractsRejectIncompleteOrFreeFormFacts(t *testing.T) {
	t.Parallel()
	reservation, settlement := validHostedCallCommands()
	if err := ValidateHostedCallReservation(reservation); err != nil {
		t.Fatalf("valid reservation rejected: %v", err)
	}
	if err := ValidateHostedCallSettlement(settlement); err != nil {
		t.Fatalf("valid settlement rejected: %v", err)
	}

	reservationTests := map[string]func(*HostedCallReservation){
		"path-shaped event":  func(value *HostedCallReservation) { value.EventID = "/private/event" },
		"missing candidate":  func(value *HostedCallReservation) { value.CandidateID = "" },
		"unordered modality": func(value *HostedCallReservation) { value.Modalities = []string{"video", "audio"} },
		"invalid version":    func(value *HostedCallReservation) { value.Versions.SchemaSHA256 = "invalid" },
		"zero reservation":   func(value *HostedCallReservation) { value.RequestedNanoUSD = 0 },
		"negative budget":    func(value *HostedCallReservation) { value.Budget.PerRunNanoUSD = -1 },
	}
	for name, mutate := range reservationTests {
		t.Run(name, func(t *testing.T) {
			changed := reservation
			mutate(&changed)
			if err := ValidateHostedCallReservation(changed); err == nil {
				t.Fatal("expected invalid reservation")
			}
		})
	}

	settlementTests := map[string]func(*HostedCallSettlement){
		"missing response":  func(value *HostedCallSettlement) { value.ResponseSHA256 = "" },
		"free form failure": func(value *HostedCallSettlement) { value.Failure = SettlementFailure("private detail") },
		"failure with outcome": func(value *HostedCallSettlement) {
			value.Failure, value.Outcome = FailureTransport, string(AudioAbsent)
		},
		"unknown charge amount": func(value *HostedCallSettlement) {
			value.Failure, value.Outcome, value.ChargeKnown = FailureTransport, "", false
		},
	}
	for name, mutate := range settlementTests {
		t.Run(name, func(t *testing.T) {
			changed := settlement
			mutate(&changed)
			if err := ValidateHostedCallSettlement(changed); err == nil {
				t.Fatal("expected invalid settlement")
			}
		})
	}
}
