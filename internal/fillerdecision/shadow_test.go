package fillerdecision

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
)

type captureRecordWriter struct{ records []Record }

func (w *captureRecordWriter) Record(_ context.Context, record Record) error {
	w.records = append(w.records, record)
	return nil
}

func TestShadowObserveBuildsOneStableDurableEvaluation(t *testing.T) {
	writer := &captureRecordWriter{}
	shadow, err := NewShadow(writer, filleradmission.Policy{
		Version:             "production-shadow-v1",
		TaxonomyVersion:     "production-shadow-taxonomy-v1",
		AllowedContentRoles: []string{filleradmission.RoleCommercial},
	}, "production-shadow-evidence-v1")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 26, 20, 0, 0, 123, time.UTC)
	observation := ShadowObservation{
		ClipHash: "clip-1", ObservedAt: at,
		Evidence: []filleradmission.Evidence{{
			ID: "media-usability:probe", Claim: filleradmission.ClaimMediaUsability,
			Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder,
			Source: "pipeline:probe",
		}},
	}

	if err := shadow.Observe(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	if err := shadow.Observe(t.Context(), observation); err != nil {
		t.Fatal(err)
	}
	if len(writer.records) != 2 {
		t.Fatalf("records = %d, want 2 calls", len(writer.records))
	}
	first, second := writer.records[0], writer.records[1]
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same observation produced different records:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if first.ID == "" || first.EvidenceHash == "" || first.CreatedAt != at {
		t.Fatalf("record identity = %+v", first)
	}
	if first.ApplicationMode != ApplicationModeShadow {
		t.Fatalf("record application mode = %q, want %q", first.ApplicationMode, ApplicationModeShadow)
	}
	if first.SchemaVersion != filleradmission.SchemaVersion ||
		first.EvidenceVersion != "production-shadow-evidence-v1" ||
		first.PolicyVersion != "production-shadow-v1" ||
		first.TaxonomyVersion != "production-shadow-taxonomy-v1" {
		t.Fatalf("record lineage = %+v", first)
	}
	if first.Result.Decision == nil || first.Result.Decision.Verdict != filleradmission.VerdictReview {
		t.Fatalf("result = %+v, want missing-rights review", first.Result)
	}
	if !reflect.DeepEqual(first.Result.Decision.ReasonCodes, []filleradmission.ReasonCode{filleradmission.ReasonMissingSourceLicense}) {
		t.Fatalf("reasons = %v", first.Result.Decision.ReasonCodes)
	}
}

func TestShadowObserveRejectsAnUnidentifiableObservation(t *testing.T) {
	writer := &captureRecordWriter{}
	shadow, err := NewShadow(writer, filleradmission.Policy{
		Version: "p1", TaxonomyVersion: "t1",
		AllowedContentRoles: []string{filleradmission.RoleCommercial},
	}, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if err := shadow.Observe(t.Context(), ShadowObservation{}); err == nil {
		t.Fatal("Observe() error = nil, want invalid observation")
	}
	if len(writer.records) != 0 {
		t.Fatalf("invalid observation wrote %d records", len(writer.records))
	}
}
