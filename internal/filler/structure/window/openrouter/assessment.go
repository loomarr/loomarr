package openrouter

import (
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
	"github.com/loomarr/loomarr/internal/platform/openroutermedia"
)

func (a *Assessor) recordAssessment(set window.MediaSet, ordinal int, assessedAt time.Time, reservation window.CallReservationState, result openroutermedia.Result, callErr error) (window.RecordedAssessment, error) {
	win := set.Plan.Windows[ordinal]
	durationMS := win.MediaEndMS - win.MediaStartMS
	input := window.CallRecordInput{
		MediaSet: set, WindowOrdinal: ordinal, Assessor: a.config.Profile,
		MetadataSnapshotSHA256: a.config.MetadataSnapshotSHA256,
		PromptSHA256:           window.DirectVideoPromptSHA256(durationMS),
		SchemaSHA256:           window.DirectVideoSchemaSHA256(durationMS),
		RequestSHA256:          result.RequestSHA256, RawResponse: result.RawResponse, StructuredOutput: result.StructuredOutput,
		UpstreamProvider: a.config.UpstreamProvider, UpstreamProviderSlug: a.config.UpstreamProviderSlug,
		GenerationID:     result.GenerationID,
		Tokens:           structure.AssessmentTokenUsage{Prompt: result.PromptTokens, Completion: result.CompletionTokens},
		RequestedNanoUSD: a.config.ReservationNanoUSD, AssessedAt: assessedAt,
	}
	if reservation == window.CallReservationHeldBudget {
		if !errors.Is(callErr, errReservationHeld) || len(result.RawResponse) != 0 {
			return window.RecordedAssessment{}, errors.New("filler structure window OpenRouter budget hold is inconsistent")
		}
		input.State, input.Failure = structure.AssessmentRecordHeldBudget, structure.AssessmentFailureBudget
		return a.newAssessmentRecord(input)
	}
	if reservation != window.CallReservationAccepted {
		return window.RecordedAssessment{}, errors.New("filler structure window OpenRouter reservation was not accepted")
	}
	input.ReservedNanoUSD = a.config.ReservationNanoUSD
	input.ChargeKnown, input.ChargedAmountUSD, input.ChargedNanoUSD = result.ChargeKnown, result.ChargedAmountUSD, result.ChargedNanoUSD
	if result.ChargeKnown {
		input.AccountedNanoUSD = result.ChargedNanoUSD
	} else {
		input.AccountedNanoUSD = a.config.ReservationNanoUSD
	}
	switch {
	case result.OverReservationNanoUSD > 0 || errors.Is(callErr, openroutermedia.ErrChargeExceedsReservation):
		input.State, input.Failure = structure.AssessmentRecordOverReservation, structure.AssessmentFailureOverReservation
	case callErr != nil && !result.ChargeKnown:
		input.State = structure.AssessmentRecordUnsettled
		if len(result.RawResponse) == 0 {
			input.Failure = structure.AssessmentFailureTransport
		} else {
			input.Failure = structure.AssessmentFailureUnsettled
		}
	case errors.Is(callErr, openroutermedia.ErrRouteMismatch):
		input.State, input.Failure = structure.AssessmentRecordFailed, structure.AssessmentFailureRouteMismatch
	case callErr != nil:
		input.State, input.Failure = structure.AssessmentRecordFailed, structure.AssessmentFailureProvider
	case !validStructuredOutput(set, ordinal, result.StructuredOutput):
		input.State, input.Failure = structure.AssessmentRecordFailed, structure.AssessmentFailureInvalidResponse
		input.ResolvedProvider, input.ResolvedModel = a.config.Profile.Provider, a.config.ResolvedModel
	default:
		input.State = structure.AssessmentRecordAccepted
		input.ResolvedProvider, input.ResolvedModel = a.config.Profile.Provider, a.config.ResolvedModel
	}
	return a.newAssessmentRecord(input)
}

func (a *Assessor) newAssessmentRecord(input window.CallRecordInput) (window.RecordedAssessment, error) {
	recorded, err := window.NewRecordedAssessment(input)
	if err != nil {
		return window.RecordedAssessment{}, fmt.Errorf("build filler structure window OpenRouter assessment record: %w", err)
	}
	return recorded, nil
}

func validStructuredOutput(set window.MediaSet, ordinal int, output string) bool {
	_, err := window.ParseDirectVideoResponse(set, ordinal, output)
	return err == nil
}
