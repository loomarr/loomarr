package fillerscreening

import (
	"bytes"
	"encoding/json"
	"io"
)

// CanonicalSpokenSafetyLedgerEvent validates an event and returns the exact payload bytes
// used for idempotency and conflict detection by every store backend.
func CanonicalSpokenSafetyLedgerEvent(event SpokenSafetyLedgerEvent) ([]byte, error) {
	if !boundedLedgerID(event.ID) || !boundedLedgerID(event.RunID) || event.Ordinal < 0 || event.CreatedAt.IsZero() {
		return nil, ErrSpokenSafetyLedgerInvalid
	}
	payloads := []any{event.Source, event.Proposal, event.Reserve, event.Settle, event.Terminal}
	present := 0
	for _, payload := range payloads {
		if !nilPayload(payload) {
			present++
		}
	}
	if present != 1 || !validEventPayload(event) {
		return nil, ErrSpokenSafetyLedgerInvalid
	}
	raw, err := json.Marshal(payloadForEvent(event))
	if err != nil || len(raw) > maxLedgerPayload {
		return nil, ErrSpokenSafetyLedgerInvalid
	}
	return raw, nil
}

func DecodeSpokenSafetyLedgerEvent(kind SpokenSafetyLedgerEventKind, raw []byte) (SpokenSafetyLedgerEvent, error) {
	event := SpokenSafetyLedgerEvent{Kind: kind}
	var target any
	switch kind {
	case LedgerSourcePlanned:
		event.Source = &SpokenSafetySourcePlanned{}
		target = event.Source
	case LedgerProposalCompleted:
		event.Proposal = &SpokenSafetyProposalCompleted{}
		target = event.Proposal
	case LedgerInferenceReserved:
		event.Reserve = &SpokenSafetyInferenceReserved{}
		target = event.Reserve
	case LedgerInferenceSettled:
		event.Settle = &SpokenSafetyInferenceSettled{}
		target = event.Settle
	case LedgerTerminal:
		event.Terminal = &SpokenSafetyTerminalResult{}
		target = event.Terminal
	default:
		return SpokenSafetyLedgerEvent{}, ErrSpokenSafetyLedgerInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return SpokenSafetyLedgerEvent{}, ErrSpokenSafetyLedgerInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return SpokenSafetyLedgerEvent{}, ErrSpokenSafetyLedgerInvalid
	}
	return event, nil
}

func SpokenSafetyLedgerEventInferenceID(event SpokenSafetyLedgerEvent) string {
	if event.Reserve != nil {
		return event.Reserve.EvaluationID
	}
	if event.Settle != nil {
		return event.Settle.EvaluationID
	}
	return ""
}

func payloadForEvent(event SpokenSafetyLedgerEvent) any {
	switch event.Kind {
	case LedgerSourcePlanned:
		return event.Source
	case LedgerProposalCompleted:
		return event.Proposal
	case LedgerInferenceReserved:
		return event.Reserve
	case LedgerInferenceSettled:
		return event.Settle
	case LedgerTerminal:
		return event.Terminal
	default:
		return nil
	}
}

func nilPayload(value any) bool {
	switch payload := value.(type) {
	case *SpokenSafetySourcePlanned:
		return payload == nil
	case *SpokenSafetyProposalCompleted:
		return payload == nil
	case *SpokenSafetyInferenceReserved:
		return payload == nil
	case *SpokenSafetyInferenceSettled:
		return payload == nil
	case *SpokenSafetyTerminalResult:
		return payload == nil
	default:
		return true
	}
}
