package filler_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
)

type captureShadowObserver struct {
	observations []fillerdecision.ShadowObservation
	err          error
}

func (o *captureShadowObserver) Observe(_ context.Context, observation fillerdecision.ShadowObservation) error {
	o.observations = append(o.observations, observation)
	return o.err
}

func TestAdmissionStageCapturesOnlyProvenProductionFacts(t *testing.T) {
	observer := &captureShadowObserver{}
	stage := filler.NewAdmissionStage(observer)
	at := time.Date(2026, 8, 26, 20, 30, 0, 0, time.UTC)
	clip := filler.StoreClip{Clip: filler.Clip{
		Hash: "clip-1", Name: "WXYZ station ident 1994.mov", Kind: filler.Commercial,
	}, UpdatedAt: at}

	result, err := stage.Run(t.Context(), clip)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != filler.VerdictContinue {
		t.Fatalf("verdict = %v", result.Verdict)
	}
	if len(observer.observations) != 1 {
		t.Fatalf("observations = %d", len(observer.observations))
	}
	got := observer.observations[0]
	if got.ClipHash != clip.Hash || got.ObservedAt != at {
		t.Fatalf("observation identity = %+v", got)
	}
	want := []filleradmission.Evidence{
		{ID: "media-usability:probe", Claim: filleradmission.ClaimMediaUsability, Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder, Source: "pipeline:probe"},
		{ID: "recording-date:filename", Claim: filleradmission.ClaimRecordingDate, Value: "1994", Kind: filleradmission.KindFilename, Source: "clip:original-name", Location: "filename"},
		{ID: "content-role:filename", Claim: filleradmission.ClaimContentRole, Value: filleradmission.RoleStationID, Kind: filleradmission.KindFilename, Source: "clip:original-name", Location: "filename"},
	}
	if !reflect.DeepEqual(got.Evidence, want) {
		t.Fatalf("evidence = %+v, want %+v", got.Evidence, want)
	}
}

func TestAdmissionStageDoesNotTreatAnUnmarkedFilenameAsACommercialClaim(t *testing.T) {
	observer := &captureShadowObserver{}
	stage := filler.NewAdmissionStage(observer)
	_, err := stage.Run(t.Context(), filler.StoreClip{Clip: filler.Clip{Hash: "clip-1", Name: "mystery.mov"}, UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range observer.observations[0].Evidence {
		if fact.Claim == filleradmission.ClaimContentRole {
			t.Fatalf("default KindFromName leaked into evidence: %+v", fact)
		}
	}
}

func TestAdmissionStageRequiresAnExplicitFilenameRoleToken(t *testing.T) {
	observer := &captureShadowObserver{}
	stage := filler.NewAdmissionStage(observer)
	for _, name := range []string{"Confidential footage.mov", "Commercialization report.mov", "Station tour.mov"} {
		observer.observations = nil
		_, err := stage.Run(t.Context(), filler.StoreClip{
			Clip: filler.Clip{Hash: "clip-1", Name: name}, UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range observer.observations[0].Evidence {
			if fact.Claim == filleradmission.ClaimContentRole {
				t.Fatalf("%q produced content-role evidence %+v", name, fact)
			}
		}
	}
}
