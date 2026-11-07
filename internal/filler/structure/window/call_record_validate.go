package window

import (
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

func ValidateRecordedAssessment(recorded RecordedAssessment) error {
	if err := ValidateCallRecord(recorded.Record); err != nil {
		return err
	}
	if err := ValidateAssessment(recorded.Record.MediaSet, recorded.Assessment); err != nil {
		return err
	}
	if len(recorded.RawResponse) > structure.AssessmentMaximumResponseBytes ||
		len(recorded.StructuredOutput) > structure.AssessmentMaximumResponseBytes ||
		optionalCallDigest(recorded.RawResponse) != recorded.Record.ResponseSHA256 ||
		optionalCallDigest([]byte(recorded.StructuredOutput)) != recorded.Record.StructuredOutputSHA256 ||
		recorded.Assessment.SHA256 != recorded.Record.AssessmentSHA256 ||
		recorded.Assessment.WindowOrdinal != recorded.Record.WindowOrdinal ||
		!reflect.DeepEqual(recorded.Assessment.Assessor, recorded.Record.Assessor) {
		return errors.New("structure window recorded assessment evidence does not match")
	}
	if recorded.Record.State == structure.AssessmentRecordAccepted {
		segments, err := ParseDirectVideoResponse(recorded.Record.MediaSet, recorded.Record.WindowOrdinal, recorded.StructuredOutput)
		if err != nil || recorded.Assessment.State != AssessmentAccepted ||
			!slices.Equal(segments, recorded.Assessment.Segments) {
			return errors.New("structure window recorded assessment does not reproduce")
		}
	} else if recorded.Assessment.State != AssessmentOperationalFailure ||
		recorded.Assessment.Failure != recorded.Record.Failure || len(recorded.Assessment.Segments) != 0 {
		return errors.New("structure window operational assessment does not reproduce")
	}
	if recorded.Record.Failure == structure.AssessmentFailureInvalidResponse {
		if _, err := ParseDirectVideoResponse(recorded.Record.MediaSet, recorded.Record.WindowOrdinal, recorded.StructuredOutput); err == nil {
			return errors.New("structure window invalid-response call contains valid structured output")
		}
	}
	return nil
}

func ValidateCallRecord(record CallRecord) error {
	if record.SchemaVersion != CallRecordSchemaVersion || record.ContractVersion != CallRecordContractVersion ||
		ValidateMediaSet(record.MediaSet) != nil || record.WindowOrdinal < 0 || record.WindowOrdinal >= len(record.MediaSet.Windows) ||
		structure.ValidateAssessorProfile(record.Assessor) != nil || !contentHash(record.MetadataSnapshotSHA256) ||
		!contentHash(record.PromptSHA256) ||
		!contentHash(record.SchemaSHA256) || !contentHash(record.RequestSHA256) || !contentHash(record.AssessmentSHA256) ||
		!callIdentity(record.UpstreamProvider) || !callIdentity(record.UpstreamProviderSlug) ||
		record.RequestedNanoUSD <= 0 || record.ReservedNanoUSD < 0 || record.ChargedNanoUSD < 0 ||
		record.AccountedNanoUSD < 0 || !validCallTokens(record.Tokens) || record.AssessedAt.IsZero() ||
		record.AssessedAt != record.AssessedAt.UTC() || !contentHash(record.SHA256) || record.SHA256 != CallRecordSHA256(record) {
		return errors.New("structure window call record identity or accounting is invalid")
	}
	if record.ResponseSHA256 != "" && !contentHash(record.ResponseSHA256) ||
		record.StructuredOutputSHA256 != "" && !contentHash(record.StructuredOutputSHA256) {
		return errors.New("structure window call record response identity is invalid")
	}
	switch record.State {
	case structure.AssessmentRecordAccepted:
		if record.Failure != "" || record.ResponseSHA256 == "" || record.StructuredOutputSHA256 == "" ||
			!closedCallRoute(record) || !closedCallCharge(record) || !closedReservedCall(record) {
			return errors.New("structure window accepted call is incomplete")
		}
	case structure.AssessmentRecordFailed:
		if !slices.Contains([]string{structure.AssessmentFailureProvider, structure.AssessmentFailureInvalidResponse, structure.AssessmentFailureRouteMismatch}, record.Failure) ||
			!validFailedCallResponse(record) || !closedCallCharge(record) || !closedReservedCall(record) {
			return errors.New("structure window failed call is incomplete")
		}
	case structure.AssessmentRecordUnsettled:
		if !slices.Contains([]string{structure.AssessmentFailureUnsettled, structure.AssessmentFailureTransport}, record.Failure) ||
			record.ChargeKnown || record.ChargedAmountUSD != "" || record.ChargedNanoUSD != 0 ||
			record.ReservedNanoUSD != record.RequestedNanoUSD || record.AccountedNanoUSD != record.ReservedNanoUSD ||
			record.ResolvedProvider != "" || record.ResolvedModel != "" ||
			(record.Failure == structure.AssessmentFailureTransport && record.ResponseSHA256 != "") ||
			(record.Failure == structure.AssessmentFailureUnsettled && record.ResponseSHA256 == "") {
			return errors.New("structure window unsettled call is invalid")
		}
	case structure.AssessmentRecordHeldBudget:
		if record.Failure != structure.AssessmentFailureBudget || record.ResponseSHA256 != "" ||
			record.StructuredOutputSHA256 != "" || record.ResolvedProvider != "" || record.ResolvedModel != "" ||
			record.GenerationID != "" || record.ChargeKnown || record.ChargedAmountUSD != "" || record.ChargedNanoUSD != 0 ||
			record.ReservedNanoUSD != 0 || record.AccountedNanoUSD != 0 || record.Tokens != (structure.AssessmentTokenUsage{}) {
			return errors.New("structure window budget-held call is invalid")
		}
	case structure.AssessmentRecordOverReservation:
		if record.Failure != structure.AssessmentFailureOverReservation || record.ResponseSHA256 == "" ||
			!closedCallCharge(record) || record.ReservedNanoUSD != record.RequestedNanoUSD ||
			record.ChargedNanoUSD <= record.ReservedNanoUSD || record.AccountedNanoUSD != record.ChargedNanoUSD {
			return errors.New("structure window over-reservation call is invalid")
		}
	default:
		return errors.New("structure window call record state is invalid")
	}
	return nil
}

func closedReservedCall(record CallRecord) bool {
	return record.ReservedNanoUSD == record.RequestedNanoUSD && record.ChargedNanoUSD <= record.ReservedNanoUSD &&
		record.AccountedNanoUSD == record.ChargedNanoUSD
}

func validFailedCallResponse(record CallRecord) bool {
	if record.ResponseSHA256 == "" {
		return false
	}
	if record.Failure == structure.AssessmentFailureInvalidResponse {
		return closedCallRoute(record)
	}
	return record.ResolvedProvider == "" && record.ResolvedModel == ""
}

func closedCallRoute(record CallRecord) bool {
	return callIdentity(record.ResolvedProvider) && callIdentity(record.ResolvedModel) && callIdentity(record.GenerationID)
}

func closedCallCharge(record CallRecord) bool {
	return record.ChargeKnown && validCallUSD(record.ChargedAmountUSD)
}

func validCallTokens(tokens structure.AssessmentTokenUsage) bool {
	return tokens.Prompt >= 0 && tokens.Completion >= 0 && tokens.Reasoning >= 0 && tokens.Cached >= 0 &&
		tokens.CacheWrite >= 0 && tokens.Image >= 0 && tokens.Audio >= 0 && tokens.Video >= 0
}

func callIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 256 && !strings.ContainsAny(value, "\r\n\t")
}

func validCallUSD(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	dot := false
	for _, char := range value {
		if char == '.' && !dot {
			dot = true
			continue
		}
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != "."
}
