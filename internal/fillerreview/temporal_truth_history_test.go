package fillerreview

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestNormalizeTemporalTruthLabelsUsesConservativePrecedence(t *testing.T) {
	tests := []struct {
		name   string
		labels fillereval.Labels
		unit   fillereval.UnitKind
		role   fillereval.TemporalRole
	}{
		{
			name: "deterministic invalid outranks suggested programme role",
			labels: temporalTruthValidLabels(fillereval.TruthInvalid, fillereval.RejectDeterministic,
				"programme_excerpt"), unit: fillereval.UnitUnusable,
		},
		{
			name: "ambiguity outranks suggested structural role",
			labels: temporalTruthValidLabels(fillereval.TruthAmbiguous, "",
				"compilation"), unit: fillereval.UnitUnclear,
		},
		{
			name: "eligible contradictory structural role remains structural",
			labels: temporalTruthValidLabels(fillereval.TruthEligible, "",
				"programme_excerpt"), unit: fillereval.UnitProgrammeExcerpt,
		},
		{
			name: "eligible filler role becomes standalone",
			labels: temporalTruthValidLabels(fillereval.TruthEligible, "",
				"commercial"), unit: fillereval.UnitStandalone, role: fillereval.TemporalRoleCommercial,
		},
		{
			name: "semantic invalid standalone role remains unclear",
			labels: temporalTruthValidLabels(fillereval.TruthInvalid, fillereval.RejectSemantic,
				"promo"), unit: fillereval.UnitUnclear,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment, err := normalizeTemporalTruthLabels("legacy", test.labels)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Unit != test.unit || assessment.Role != test.role {
				t.Fatalf("normalized assessment = %+v", assessment)
			}
		})
	}
}

func TestNormalizeTemporalTruthLabelsRejectsMalformedRecoveredRows(t *testing.T) {
	labels := temporalTruthValidLabels(fillereval.TruthEligible, "", "commercial")
	labels.Evidence = nil
	if _, err := normalizeTemporalTruthLabels("legacy", labels); err == nil || !strings.Contains(err.Error(), "at least one evidence") {
		t.Fatalf("malformed recovered labels error = %v", err)
	}
}

func temporalTruthValidLabels(truth fillereval.Truth, reject fillereval.RejectClass, role string) fillereval.Labels {
	labels := fillereval.Labels{
		Truth: truth, RejectClass: reject, ContentRole: role, Slices: []string{"legacy"},
		Evidence: []fillereval.Evidence{{ID: "frame-01", Kind: "frame", Claim: "legacy", Value: "observed", Provenance: "cases/opaque/frame-01.jpg"}},
	}
	if truth == fillereval.TruthAmbiguous {
		labels.ReviewQuestion = "Is this one complete temporal unit?"
	}
	return labels
}
