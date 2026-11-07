package enrichment_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

func TestOperator_IsAValidCompletedObservationOnEveryAxis(t *testing.T) {
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.FixedZone("EST", -5*3600))
	values := map[enrichment.Axis]enrichment.Value{
		enrichment.AxisEra:      {Year: 1987},
		enrichment.AxisAudience: {Text: "family"},
	}
	for _, axis := range enrichment.TaxonomyAxes() {
		values[axis] = enrichment.Value{} // a cleared axis is still an observation
	}
	for axis, value := range values {
		state := enrichment.Operator("clip", axis, value, "operator.clip_edit", enrichment.OperatorTaxonomyVersion, at)
		if err := state.Validate(); err != nil {
			t.Errorf("Operator(%s) is invalid: %v", axis, err)
		}
		ev := state.Evidence
		if ev.Kind != enrichment.EvidenceOperator || ev.Confidence != 100 || state.Status != enrichment.StatusComplete ||
			ev.Reference != "operator.clip_edit" || ev.Producer != "operator" || ev.ProducerVersion != "1" ||
			ev.TaxonomyVersion != enrichment.OperatorTaxonomyVersion || !ev.ObservedAt.Equal(at) || ev.ObservedAt.Location() != time.UTC {
			t.Errorf("Operator(%s) = %+v, want complete full-confidence operator evidence observed in UTC", axis, state)
		}
	}
}
