package recommend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// Evaluate scores one model output against only the facts in its supplied
// synthetic snapshot. A concept citing any other fact is rejected whole.
func Evaluate(snapshot Snapshot, raw []byte) Assessment {
	effectful := effectfulConcepts(raw)
	output, err := decodeModelOutput(raw, len(effectful) == 0)
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

func decodeModelOutput(raw []byte, strict bool) (modelOutput, error) {
	var output modelOutput
	if !strict {
		err := json.Unmarshal(raw, &output)
		return output, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return modelOutput{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return modelOutput{}, fmt.Errorf("trailing JSON value")
	}
	return output, nil
}

func effectfulConcepts(raw []byte) map[int]bool {
	var envelope struct {
		Concepts []map[string]json.RawMessage `json:"concepts"`
	}
	_ = json.Unmarshal(raw, &envelope)
	forbidden := map[string]bool{
		"channelId": true, "proposalId": true, "jobId": true, "status": true,
		"approve": true, "approved": true, "acquisitions": true,
	}
	result := make(map[int]bool)
	for i, concept := range envelope.Concepts {
		for field := range concept {
			if forbidden[field] {
				result[i] = true
				break
			}
		}
	}
	return result
}
