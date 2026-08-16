package playout_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/playout"
)

func TestEffectiveCapacity_AutomaticUsesMeasurement(t *testing.T) {
	if got := playout.EffectiveCapacity(6, 0, 0); got != 6 {
		t.Fatalf("EffectiveCapacity(6 measured, automatic cap) = %d, want 6", got)
	}
}

func TestEffectiveCapacity_ConfiguredCapCanOnlyLowerMeasurement(t *testing.T) {
	for _, tc := range []struct {
		name     string
		measured int
		cap      int
		want     int
	}{
		{name: "lower cap", measured: 6, cap: 3, want: 3},
		{name: "higher cap", measured: 3, cap: 9, want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := playout.EffectiveCapacity(tc.measured, tc.cap, 0); got != tc.want {
				t.Fatalf("EffectiveCapacity(%d measured, %d cap) = %d, want %d",
					tc.measured, tc.cap, got, tc.want)
			}
		})
	}
}

func TestEffectiveCapacity_UnknownMeasurementAllowsOneSafeTranscode(t *testing.T) {
	for _, measured := range []int{0, -1} {
		if got := playout.EffectiveCapacity(measured, 0, 0); got != 1 {
			t.Errorf("EffectiveCapacity(%d measured, automatic cap) = %d, want 1", measured, got)
		}
	}
}

func TestEffectiveCapacity_ShadesResidentVRAMWithoutDisablingPlayout(t *testing.T) {
	for _, tc := range []struct {
		name        string
		measured    int
		cap         int
		residentGiB float64
		want        int
	}{
		{name: "shade after cap", measured: 8, cap: 6, residentGiB: 8, want: 4},
		{name: "floor", measured: 2, cap: 0, residentGiB: 16, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := playout.EffectiveCapacity(tc.measured, tc.cap, tc.residentGiB); got != tc.want {
				t.Fatalf("EffectiveCapacity(%d measured, %d cap, %.1f GiB) = %d, want %d",
					tc.measured, tc.cap, tc.residentGiB, got, tc.want)
			}
		})
	}
}
