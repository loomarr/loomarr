package storagegovernor_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

func TestEstimateMediaUsesDeclaredBytesWithGovernorOwnedHeadroom(t *testing.T) {
	t.Parallel()
	budget, ok := storagegovernor.EstimateMedia(storagegovernor.MediaEstimate{DeclaredBytes: 100 << 20})
	if !ok {
		t.Fatal("declared media estimate was rejected")
	}
	if budget.WriteCeilingBytes != 132<<20 {
		t.Fatalf("write ceiling = %d, want 132 MiB", budget.WriteCeilingBytes)
	}
	if budget.ReservationBytes != 592<<20 {
		t.Fatalf("reservation = %d, want 592 MiB", budget.ReservationBytes)
	}
}

func TestEstimateMediaFallsBackToDurationAndQuality(t *testing.T) {
	t.Parallel()
	budget, ok := storagegovernor.EstimateMedia(storagegovernor.MediaEstimate{DurationMS: 60_000, Height: 720})
	if !ok {
		t.Fatal("duration media estimate was rejected")
	}
	// 60 seconds at the 12 Mbit/s 720p ceiling is 90 MB, plus the 32 MiB floor.
	wantWrite := int64(90_000_000 + 32<<20)
	if budget.WriteCeilingBytes != wantWrite {
		t.Fatalf("write ceiling = %d, want %d", budget.WriteCeilingBytes, wantWrite)
	}
	if budget.ReservationBytes <= budget.WriteCeilingBytes {
		t.Fatalf("reservation = %d, want derivative headroom above %d", budget.ReservationBytes, budget.WriteCeilingBytes)
	}
}

func TestEstimateMediaRefusesUnknownAndOverflowedInputs(t *testing.T) {
	t.Parallel()
	for name, estimate := range map[string]storagegovernor.MediaEstimate{
		"unknown":  {},
		"overflow": {DeclaredBytes: int64(^uint64(0) >> 1)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if budget, ok := storagegovernor.EstimateMedia(estimate); ok || budget != (storagegovernor.MediaBudget{}) {
				t.Fatalf("EstimateMedia(%+v) = (%+v, %v), want refusal", estimate, budget, ok)
			}
		})
	}
}
