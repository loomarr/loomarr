package screening

import "slices"

// ValidateSpokenSafetyHostedCallReservation validates the domain-owned command before a
// persistence adapter opens a transaction.
func ValidateSpokenSafetyHostedCallReservation(command SpokenSafetyHostedCallReservation) error {
	if !boundedLedgerID(command.EventID) || !boundedLedgerID(command.RunID) ||
		!boundedLedgerID(command.EvaluationID) || !boundedLedgerID(command.ClipHash) ||
		!validSHA256(command.RequestSHA256) || !boundedLedgerID(command.Role) || !boundedLedgerID(command.Rung) ||
		!boundedPublicIdentity(command.RequestedProvider) || !boundedPublicIdentity(command.RequestedModel) ||
		!boundedPublicIdentity(command.UpstreamProvider) || command.Ordinal < 0 || command.CreatedAt.IsZero() ||
		command.DerivativeBytes <= 0 || command.DerivativeDurationMS <= 0 || command.DerivativePixels < 0 ||
		command.RequestedNanoUSD <= 0 || command.Budget.PerClipNanoUSD < 0 ||
		command.Budget.PerDayNanoUSD < 0 || command.Budget.PerRunNanoUSD < 0 {
		return ErrSpokenSafetyEvaluationInvalid
	}
	versions := []string{
		command.Versions.EvidenceSHA256, command.Versions.ExtractorSHA256,
		command.Versions.PromptSHA256, command.Versions.SchemaSHA256,
		command.Versions.TaxonomySHA256, command.Versions.CertificationSHA256,
		command.Versions.PolicySHA256, command.Versions.CapabilitySHA256,
	}
	if slices.ContainsFunc(versions, func(value string) bool { return !validSHA256(value) }) {
		return ErrSpokenSafetyEvaluationInvalid
	}
	if slices.Equal(command.Modalities, []string{"audio"}) {
		if !boundedLedgerID(command.CandidateID) {
			return ErrSpokenSafetyEvaluationInvalid
		}
		return nil
	}
	if command.CandidateID == "" && slices.Equal(command.Modalities, []string{"audio", "video"}) {
		return nil
	}
	return ErrSpokenSafetyEvaluationInvalid
}

// ValidateSpokenSafetyHostedCallSettlement rejects free-form or incomplete settlement
// facts before they can reach either the V62 row or spoken-safety ledger.
func ValidateSpokenSafetyHostedCallSettlement(command SpokenSafetyHostedCallSettlement) error {
	if !boundedLedgerID(command.EventID) || !boundedLedgerID(command.RunID) ||
		!boundedLedgerID(command.ReservationEventID) || command.Ordinal < 0 || command.CreatedAt.IsZero() ||
		command.ChargedNanoUSD < 0 || command.PromptTokens < 0 || command.CompletionTokens < 0 || command.LatencyMS < 0 {
		return ErrSpokenSafetyEvaluationInvalid
	}
	if command.Failure == FailureNone {
		if !validSHA256(command.ResponseSHA256) || !boundedPublicIdentity(command.ResolvedProvider) ||
			!boundedPublicIdentity(command.ResolvedModel) || !boundedPublicIdentity(command.UpstreamProvider) ||
			!boundedLedgerID(command.GenerationID) || !validInferenceOutcome(command.Outcome) ||
			!command.ChargeKnown || !validUSD(command.ChargedAmountUSD) {
			return ErrSpokenSafetyEvaluationInvalid
		}
		return nil
	}
	if !validSettlementFailure(command.Failure) || command.Failure == FailureInterrupted || command.Failure == FailureBudget ||
		command.Outcome != "" || !optionalSHA256(command.ResponseSHA256) ||
		!optionalPublicIdentity(command.ResolvedProvider) || !optionalPublicIdentity(command.ResolvedModel) ||
		!optionalPublicIdentity(command.UpstreamProvider) || !optionalLedgerID(command.GenerationID) {
		return ErrSpokenSafetyEvaluationInvalid
	}
	if command.ChargeKnown {
		if !validUSD(command.ChargedAmountUSD) {
			return ErrSpokenSafetyEvaluationInvalid
		}
		return nil
	}
	if command.ChargedAmountUSD != "" || command.ChargedNanoUSD != 0 {
		return ErrSpokenSafetyEvaluationInvalid
	}
	return nil
}
