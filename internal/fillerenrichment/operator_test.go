package fillerenrichment_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

func TestOperator_IsAValidCompletedObservationOnEveryAxis(t *testing.T) {
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.FixedZone("EST", -5*3600))
	values := map[fillerenrichment.Axis]fillerenrichment.Value{
		fillerenrichment.AxisEra:      {Year: 1987},
		fillerenrichment.AxisAudience: {Text: "family"},
	}
	for _, axis := range fillerenrichment.TaxonomyAxes() {
		values[axis] = fillerenrichment.Value{} // a cleared axis is still an observation
	}
	for axis, value := range values {
		state := fillerenrichment.Operator("clip", axis, value, "operator.clip_edit", fillerenrichment.OperatorTaxonomyVersion, at)
		if err := state.Validate(); err != nil {
			t.Errorf("Operator(%s) is invalid: %v", axis, err)
		}
		ev := state.Evidence
		if ev.Kind != fillerenrichment.EvidenceOperator || ev.Confidence != 100 || state.Status != fillerenrichment.StatusComplete ||
			ev.Reference != "operator.clip_edit" || ev.Producer != "operator" || ev.ProducerVersion != "1" ||
			ev.TaxonomyVersion != fillerenrichment.OperatorTaxonomyVersion || !ev.ObservedAt.Equal(at) || ev.ObservedAt.Location() != time.UTC {
			t.Errorf("Operator(%s) = %+v, want complete full-confidence operator evidence observed in UTC", axis, state)
		}
	}
}
