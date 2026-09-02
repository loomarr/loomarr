package recommend

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// StructuralDiagnostics is deliberately content-free. It records only stable
// shape facts needed to distinguish prompt, schema, and output-ceiling failures;
// it has no field capable of retaining a prompt, response, value, or field name.
type StructuralDiagnostics struct {
	RootJSONValid       bool `json:"rootJsonValid"`
	RequiredFieldsValid bool `json:"requiredFieldsValid"`
	UnknownFieldCount   int  `json:"unknownFieldCount"`
	EffectfulFieldCount int  `json:"effectfulFieldCount"`
	Truncated           bool `json:"truncated"`
	Abstained           bool `json:"abstained"`
	ConceptCount        int  `json:"conceptCount"`
}

// DiagnoseOutput classifies a model response without returning or retaining any
// of its content. Categories intentionally overlap: an effectful field is also
// unknown to the inert output schema.
func DiagnoseOutput(raw []byte) StructuralDiagnostics {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return StructuralDiagnostics{Truncated: looksTruncatedJSON(raw)}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return StructuralDiagnostics{}
	}
	root, ok := value.(map[string]any)
	if !ok {
		return StructuralDiagnostics{}
	}
	diagnostic := StructuralDiagnostics{RootJSONValid: true}
	diagnostic.UnknownFieldCount = countUnknownFields(root, map[string]fieldShape{
		"concepts": shapeConcepts,
	})
	diagnostic.EffectfulFieldCount = countEffectfulFields(value)

	conceptValues, ok := root["concepts"].([]any)
	if !ok || len(conceptValues) > 4 {
		return diagnostic
	}
	diagnostic.ConceptCount = len(conceptValues)
	if len(conceptValues) == 0 {
		diagnostic.RequiredFieldsValid = true
		diagnostic.Abstained = true
		return diagnostic
	}
	for _, conceptValue := range conceptValues {
		concept, valid := conceptValue.(map[string]any)
		if !valid || !requiredString(concept["name"]) || !requiredIntent(concept["intent"]) || !requiredStringList(concept["evidenceIds"], true) {
			return diagnostic
		}
	}
	diagnostic.RequiredFieldsValid = true
	return diagnostic
}

type fieldShape uint8

const (
	shapeScalar fieldShape = iota
	shapeConcepts
	shapeIntent
)

func countUnknownFields(object map[string]any, allowed map[string]fieldShape) int {
	count := 0
	for field, value := range object {
		shape, known := allowed[field]
		if !known {
			count++
			continue
		}
		switch shape {
		case shapeConcepts:
			items, ok := value.([]any)
			if !ok {
				continue
			}
			for _, item := range items {
				if concept, valid := item.(map[string]any); valid {
					count += countUnknownFields(concept, map[string]fieldShape{
						"name": shapeScalar, "intent": shapeIntent, "evidenceIds": shapeScalar,
					})
				}
			}
		case shapeIntent:
			if intent, valid := value.(map[string]any); valid {
				count += countUnknownFields(intent, map[string]fieldShape{
					"description": shapeScalar, "era": shapeScalar, "tone": shapeScalar,
					"mustInclude": shapeScalar, "mustExclude": shapeScalar,
				})
			}
		}
	}
	return count
}

func countEffectfulFields(value any) int {
	forbidden := map[string]bool{
		"channelId": true, "proposalId": true, "jobId": true, "status": true,
		"approve": true, "approved": true, "acquisitions": true,
	}
	count := 0
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for field, nested := range typed {
				if forbidden[field] {
					count++
				}
				visit(nested)
			}
		case []any:
			for _, nested := range typed {
				visit(nested)
			}
		}
	}
	visit(value)
	return count
}

func requiredString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func requiredIntent(value any) bool {
	intent, ok := value.(map[string]any)
	return ok && requiredString(intent["description"])
}

func requiredStringList(value any, requireItem bool) bool {
	items, ok := value.([]any)
	if !ok || (requireItem && len(items) == 0) {
		return false
	}
	for _, item := range items {
		if !requiredString(item) {
			return false
		}
	}
	return true
}

func looksTruncatedJSON(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return false
	}
	depth := 0
	inString := false
	escaped := false
	for _, char := range trimmed {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return inString || escaped || depth > 0
}
