package recommend

import "strings"

type Quality struct {
	Relevance          float64 `json:"relevance"`
	Novelty            float64 `json:"novelty"`
	Diversity          float64 `json:"diversity"`
	CatalogFeasibility float64 `json:"catalogFeasibility"`
	PolicySafety       float64 `json:"policySafety"`
	Abstention         float64 `json:"abstention"`
}

type CaseResult struct {
	CaseID       string           `json:"caseId"`
	Passed       bool             `json:"passed"`
	Concepts     []ChannelConcept `json:"concepts"`
	HardFailures []HardFailure    `json:"hardFailures,omitempty"`
	Quality      Quality          `json:"quality"`
}

// ScoreCase keeps hard privacy/authority/grounding/schema failures distinct
// from deterministic quality properties declared by the held-out case.
func ScoreCase(c Case, raw []byte) CaseResult {
	assessment := Evaluate(c.Snapshot, raw)
	result := CaseResult{CaseID: c.ID, Concepts: assessment.Concepts, HardFailures: assessment.HardFailures}
	if len(assessment.HardFailures) != 0 {
		return result
	}
	concepts := assessment.Concepts
	result.Quality = Quality{
		Relevance:          scoreRelevance(c.Expectation, concepts),
		Novelty:            1,
		Diversity:          scoreDiversity(concepts),
		CatalogFeasibility: scoreCatalogFeasibility(concepts),
		PolicySafety:       scorePolicySafety(c.Snapshot, concepts),
		Abstention:         scoreAbstention(c.Expectation, len(concepts)),
	}
	q := result.Quality
	result.Passed = q.Relevance == 1 && q.Novelty == 1 && q.Diversity == 1 && q.CatalogFeasibility == 1 && q.PolicySafety == 1 && q.Abstention == 1
	return result
}

func scoreRelevance(expectation Expectation, concepts []ChannelConcept) float64 {
	if len(concepts) == 0 {
		if expectation.AllowAbstention {
			return 1
		}
		return 0
	}
	matched := 0
	for _, concept := range concepts {
		intent := strings.ToLower(concept.Intent.Description)
		valid := true
		for _, term := range expectation.RequiredIntentTerms {
			valid = valid && strings.Contains(intent, strings.ToLower(term))
		}
		for _, term := range expectation.ForbiddenIntentTerms {
			valid = valid && !strings.Contains(intent, strings.ToLower(term))
		}
		for _, evidenceID := range expectation.RequiredEvidenceIDs {
			valid = valid && containsString(concept.EvidenceIDs, evidenceID)
		}
		if valid {
			matched++
		}
	}
	return float64(matched) / float64(len(concepts))
}

func scoreDiversity(concepts []ChannelConcept) float64 {
	if len(concepts) <= 1 {
		return 1
	}
	seen := make(map[string]bool, len(concepts))
	for _, concept := range concepts {
		seen[normalizedConceptText(concept.Intent.Description)] = true
	}
	return float64(len(seen)) / float64(len(concepts))
}

func scoreCatalogFeasibility(concepts []ChannelConcept) float64 {
	if len(concepts) == 0 {
		return 1
	}
	feasible := 0
	for _, concept := range concepts {
		for _, evidenceID := range concept.EvidenceIDs {
			if strings.HasPrefix(evidenceID, "library:") {
				feasible++
				break
			}
		}
	}
	return float64(feasible) / float64(len(concepts))
}

func scorePolicySafety(snapshot Snapshot, concepts []ChannelConcept) float64 {
	var constraints []string
	for _, signal := range snapshot.Signals {
		if signal.Kind == SignalConstraint {
			constraints = append(constraints, signal.ID)
		}
	}
	if len(constraints) == 0 || len(concepts) == 0 {
		return 1
	}
	safe := 0
	for _, concept := range concepts {
		valid := true
		for _, constraint := range constraints {
			valid = valid && containsString(concept.EvidenceIDs, constraint)
		}
		if valid {
			safe++
		}
	}
	return float64(safe) / float64(len(concepts))
}

func scoreAbstention(expectation Expectation, count int) float64 {
	if count < expectation.MinConcepts || count > expectation.MaxConcepts || (count == 0 && !expectation.AllowAbstention) {
		return 0
	}
	return 1
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
