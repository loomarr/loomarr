package window

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

// RetryableCallRecord reports whether a settled call produced no answer at all: the budget held it
// before any request left, or the request got no response back. Neither is a fact about the
// window, so neither is replayed; the next pass calls again. A transport record keeps its full
// reservation in the shared budget because the provider may have charged the lost request.
// Every outcome that carries response bytes (accepted, failed, unsettled with a response,
// over-reservation) is a known outcome and replays, so a billed call is never repeated.
func RetryableCallRecord(record CallRecord) bool {
	switch record.State {
	case structure.AssessmentRecordHeldBudget:
		return true
	case structure.AssessmentRecordUnsettled:
		return record.Failure == structure.AssessmentFailureTransport
	default:
		return false
	}
}

func NewRecordedAssessment(input CallRecordInput) (RecordedAssessment, error) {
	failure := strings.TrimSpace(input.Failure)
	assessmentInput := AssessmentInput{
		MediaSet: input.MediaSet, WindowOrdinal: input.WindowOrdinal, Assessor: input.Assessor,
		Failure: failure, AssessedAt: input.AssessedAt,
	}
	if input.State == structure.AssessmentRecordAccepted {
		segments, err := ParseDirectVideoResponse(input.MediaSet, input.WindowOrdinal, input.StructuredOutput)
		if err != nil {
			return RecordedAssessment{}, err
		}
		assessmentInput.Segments = segments
	}
	assessment, err := NewAssessment(assessmentInput)
	if err != nil {
		return RecordedAssessment{}, err
	}
	record := CallRecord{
		SchemaVersion: CallRecordSchemaVersion, ContractVersion: CallRecordContractVersion,
		MediaSet: input.MediaSet, WindowOrdinal: input.WindowOrdinal, Assessor: input.Assessor,
		MetadataSnapshotSHA256: input.MetadataSnapshotSHA256,
		PromptSHA256:           input.PromptSHA256, SchemaSHA256: input.SchemaSHA256,
		RequestSHA256: input.RequestSHA256, ResponseSHA256: optionalCallDigest(input.RawResponse),
		StructuredOutputSHA256: optionalCallDigest([]byte(input.StructuredOutput)), AssessmentSHA256: assessment.SHA256,
		ResolvedProvider: strings.TrimSpace(input.ResolvedProvider), ResolvedModel: strings.TrimSpace(input.ResolvedModel),
		UpstreamProvider: strings.TrimSpace(input.UpstreamProvider), UpstreamProviderSlug: strings.TrimSpace(input.UpstreamProviderSlug),
		GenerationID: strings.TrimSpace(input.GenerationID), Tokens: input.Tokens,
		RequestedNanoUSD: input.RequestedNanoUSD, ReservedNanoUSD: input.ReservedNanoUSD,
		ChargedAmountUSD: strings.TrimSpace(input.ChargedAmountUSD), ChargedNanoUSD: input.ChargedNanoUSD,
		AccountedNanoUSD: input.AccountedNanoUSD, ChargeKnown: input.ChargeKnown,
		State: input.State, Failure: failure, AssessedAt: input.AssessedAt.UTC().Round(0),
	}
	record.SHA256 = CallRecordSHA256(record)
	recorded := RecordedAssessment{
		Record: record, Assessment: assessment, RawResponse: bytes.Clone(input.RawResponse),
		StructuredOutput: input.StructuredOutput,
	}
	return recorded, ValidateRecordedAssessment(recorded)
}

func CallRecordSHA256(record CallRecord) string {
	record.SHA256 = ""
	raw, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func optionalCallDigest(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
