package recommend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	FailureUnsupportedEvidence = "unsupported_evidence"
	FailureEffectAuthority     = "effect_authority"
	FailureDuplicateConcept    = "duplicate_concept"
	FailureInvalidSchema       = "invalid_schema"
)

type SignalKind string

const (
	SignalLibraryGenre SignalKind = "library_genre"
	SignalLibraryEra   SignalKind = "library_era"
	SignalPreference   SignalKind = "preference"
	SignalSeason       SignalKind = "season"
	SignalConstraint   SignalKind = "constraint"
)

type Signal struct {
	ID    string     `json:"id"`
	Kind  SignalKind `json:"kind"`
	Value string     `json:"value"`
}

type Snapshot struct {
	ID               string            `json:"id"`
	Signals          []Signal          `json:"signals"`
	ExistingConcepts []ExistingConcept `json:"existingConcepts,omitempty"`
}

type ExistingConcept struct {
	Name              string `json:"name"`
	IntentDescription string `json:"intentDescription"`
}

type DraftIntent struct {
	Description string   `json:"description"`
	Era         string   `json:"era,omitempty"`
	Tone        string   `json:"tone,omitempty"`
	MustInclude []string `json:"mustInclude,omitempty"`
	MustExclude []string `json:"mustExclude,omitempty"`
}

type ChannelConcept struct {
	Name        string      `json:"name"`
	Intent      DraftIntent `json:"intent"`
	EvidenceIDs []string    `json:"evidenceIds"`
}

type HardFailure struct {
	Code         string `json:"code"`
	ConceptIndex int    `json:"conceptIndex"`
}

type Assessment struct {
	Passed       bool             `json:"passed"`
	Concepts     []ChannelConcept `json:"concepts"`
	HardFailures []HardFailure    `json:"hardFailures,omitempty"`
}

type modelOutput struct {
	Concepts []ChannelConcept `json:"concepts"`
}

type rawModelOutput struct {
	Concepts json.RawMessage `json:"concepts"`
}

// Evaluate scores one model output against only the facts in its supplied
// synthetic snapshot. A concept citing any other fact is rejected whole.
func Evaluate(snapshot Snapshot, raw []byte) Assessment {
	output, effectful, invalid, err := decodeModelOutput(raw)
	if err != nil {
		return Assessment{HardFailures: []HardFailure{{Code: FailureInvalidSchema, ConceptIndex: -1}}}
	}
	supported := make(map[string]bool, len(snapshot.Signals))
	for _, signal := range snapshot.Signals {
		supported[signal.ID] = true
	}
	assessment := Assessment{}
	existingNames := make(map[string]bool, len(snapshot.ExistingConcepts))
	existingIntents := make(map[string]bool, len(snapshot.ExistingConcepts))
	for _, existing := range snapshot.ExistingConcepts {
		existingNames[normalizedConceptText(existing.Name)] = true
		existingIntents[normalizedConceptText(existing.IntentDescription)] = true
	}
	for i, concept := range output.Concepts {
		if effectful[i] {
			assessment.HardFailures = append(assessment.HardFailures, HardFailure{Code: FailureEffectAuthority, ConceptIndex: i})
			continue
		}
		if invalid[i] {
			assessment.HardFailures = append(assessment.HardFailures, HardFailure{Code: FailureInvalidSchema, ConceptIndex: i})
			continue
		}
		if existingNames[normalizedConceptText(concept.Name)] || existingIntents[normalizedConceptText(concept.Intent.Description)] {
			assessment.HardFailures = append(assessment.HardFailures, HardFailure{Code: FailureDuplicateConcept, ConceptIndex: i})
			continue
		}
		valid := true
		for _, evidenceID := range concept.EvidenceIDs {
			if !supported[evidenceID] {
				assessment.HardFailures = append(assessment.HardFailures, HardFailure{Code: FailureUnsupportedEvidence, ConceptIndex: i})
				valid = false
				break
			}
		}
		if valid {
			assessment.Concepts = append(assessment.Concepts, concept)
		}
	}
	assessment.Passed = len(assessment.HardFailures) == 0
	return assessment
}

func decodeModelOutput(raw []byte) (modelOutput, map[int]bool, map[int]bool, error) {
	var envelope rawModelOutput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return modelOutput{}, nil, nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return modelOutput{}, nil, nil, fmt.Errorf("trailing JSON value")
	}
	var rawConcepts []json.RawMessage
	if len(envelope.Concepts) == 0 || string(envelope.Concepts) == "null" || json.Unmarshal(envelope.Concepts, &rawConcepts) != nil {
		return modelOutput{}, nil, nil, fmt.Errorf("concepts must be an array")
	}
	if len(rawConcepts) > 4 {
		return modelOutput{}, nil, nil, fmt.Errorf("too many concepts")
	}
	forbidden := map[string]bool{
		"channelId": true, "proposalId": true, "jobId": true, "status": true,
		"approve": true, "approved": true, "acquisitions": true,
	}
	output := modelOutput{Concepts: make([]ChannelConcept, len(rawConcepts))}
	effectful := make(map[int]bool)
	invalid := make(map[int]bool)
	for i, rawConcept := range rawConcepts {
		var generic any
		if json.Unmarshal(rawConcept, &generic) != nil {
			invalid[i] = true
			continue
		}
		if containsForbiddenField(generic, forbidden) {
			effectful[i] = true
			_ = json.Unmarshal(rawConcept, &output.Concepts[i])
			continue
		}
		conceptDecoder := json.NewDecoder(bytes.NewReader(rawConcept))
		conceptDecoder.DisallowUnknownFields()
		if conceptDecoder.Decode(&output.Concepts[i]) != nil || !validConceptSchema(output.Concepts[i]) {
			invalid[i] = true
		}
	}
	return output, effectful, invalid, nil
}

func containsForbiddenField(value any, forbidden map[string]bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		for field, nested := range typed {
			if forbidden[field] || containsForbiddenField(nested, forbidden) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsForbiddenField(nested, forbidden) {
				return true
			}
		}
	}
	return false
}

func validConceptSchema(concept ChannelConcept) bool {
	if !boundedRequired(concept.Name, 128) || !boundedRequired(concept.Intent.Description, 512) ||
		len(concept.EvidenceIDs) == 0 || len(concept.EvidenceIDs) > 64 ||
		!boundedOptional(concept.Intent.Era, 128) || !boundedOptional(concept.Intent.Tone, 128) ||
		len(concept.Intent.MustInclude) > 32 || len(concept.Intent.MustExclude) > 32 {
		return false
	}
	seenEvidence := make(map[string]bool, len(concept.EvidenceIDs))
	for _, evidenceID := range concept.EvidenceIDs {
		if !boundedRequired(evidenceID, 256) || seenEvidence[evidenceID] {
			return false
		}
		seenEvidence[evidenceID] = true
	}
	for _, values := range [][]string{concept.Intent.MustInclude, concept.Intent.MustExclude} {
		for _, value := range values {
			if !boundedRequired(value, 256) {
				return false
			}
		}
	}
	return true
}

func boundedRequired(value string, max int) bool {
	length := len([]rune(strings.TrimSpace(value)))
	return length > 0 && length <= max
}

func boundedOptional(value string, max int) bool {
	return value == "" || boundedRequired(value, max)
}
