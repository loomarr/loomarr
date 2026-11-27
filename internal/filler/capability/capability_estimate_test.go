package capability

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// A high-confidence decade estimate from the frames dates a clip that has no era, as an estimate
// below every literal source: item metadata replaces it (#3838 decision 7).
func TestFillerEnrichmentVisionDecadeEstimateDatesAnUndatedClipAndYieldsToLiteralEvidence(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_200, 0).UTC()
	clip := clipstore.Clip{Clip: clips.Clip{
		Hash: "undated", Path: "undated.mp4", Name: "Undated advert", Role: clips.Commercial,
		Placement: clips.PlacementBreakBody,
	}, UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	observer := &recordingVisionObserver{observation: filler.VisionObservation{
		VisibleText: "SNACKS", EstimatedDecade: 1990,
	}}
	runner := enrichment.NewCapabilityRunner(
		enrichment.CapabilityVision,
		storeadapter.NewEnrichment(st),
		func() enrichment.CapabilitySelection {
			return enrichment.CapabilitySelection{Available: true, Producer: "vision:test", ProducerVersion: "vision-v1:model"}
		},
		NewVisionExecutor(st, observer).Run,
		func() int { return 1 }, func() time.Time { return at.Add(time.Minute) },
	)
	if result, err := runner.Run(t.Context()); err != nil || result.Failed != 0 {
		t.Fatalf("vision pass = %+v, err %v", result, err)
	}
	got, err := st.GetClip(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Era != 1990 {
		t.Fatalf("era = %d, want the estimated decade 1990", got.Era)
	}
	states, err := st.ListFillerEnrichment(t.Context(), clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var era enrichment.State
	for _, s := range states {
		if s.Axis == enrichment.AxisEra {
			era = s
		}
	}
	if era.Evidence.Kind != enrichment.EvidenceInference || era.Value.Year != 1990 {
		t.Fatalf("era evidence = %+v, want an inference of 1990", era)
	}

	literal := enrichment.State{ClipHash: clip.Hash, Axis: enrichment.AxisEra, Status: enrichment.StatusComplete,
		Value: enrichment.Value{Year: 1994},
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "listing.upload_date",
			Confidence: 100, Producer: "deterministic", ProducerVersion: "1", ObservedAt: at.Add(2 * time.Minute)}}
	if _, changed, err := st.ApplyFillerEnrichment(t.Context(), literal, at.Add(2*time.Minute)); err != nil || !changed {
		t.Fatalf("literal evidence = changed %v, err %v, want it to replace the estimate", changed, err)
	}
	if got, _ := st.GetClip(t.Context(), clip.Hash); got.Era != 1994 {
		t.Fatalf("era = %d after literal evidence, want 1994", got.Era)
	}
}
