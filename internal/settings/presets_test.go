package settings

import (
	"slices"
	"testing"
)

// A preset is a value the setting itself accepts, so offering it can never produce an invalid
// save. One that does not parse (or fails Validate) is a declaration bug caught at boot.
func TestRegistry_RejectsAPresetThatDoesNotParse(t *testing.T) {
	declaration := Setting{
		Key: "test.every", EnvVar: "TEST_EVERY", Group: GroupAdvanced, Owner: OwnerAdvanced,
		Kind: KindDuration, Default: "6h", Doc: "test",
		Presets: []Preset{preset("6h", "Every 6 hours"), preset("daily", "Daily")},
	}
	defer func() {
		if recover() == nil {
			t.Fatal("newRegistry accepted a preset that is not a duration")
		}
	}()
	newRegistry([]Setting{declaration})
}

// Presets are published in the same canonical form a resolved value travels in, so the UI
// matches the current value against them by string: "24h" is declared, "24h0m0s" is served.
func TestRegistry_PresetsAreCanonical(t *testing.T) {
	r := newRegistry([]Setting{{
		Key: "test.every", EnvVar: "TEST_EVERY", Group: GroupAdvanced, Owner: OwnerAdvanced,
		Kind: KindDuration, Default: "6h", Doc: "test",
		Presets: []Preset{preset("0s", "Never"), preset("24h", "Daily")},
	}})
	s, _ := r.Get("test.every")
	want := []Preset{preset("0s", "Never"), preset("24h0m0s", "Daily")}
	if !slices.Equal(s.Presets, want) {
		t.Errorf("presets = %v, want %v", s.Presets, want)
	}
}

// The household download schedule offers its common choices from the registry.
func TestRegistry_FillerFetchEveryOffersTheHouseholdSchedule(t *testing.T) {
	s, _ := NewRegistry().Get(FillerFetchEvery.Key())
	var labels []string
	for _, p := range s.Presets {
		labels = append(labels, p.Label)
	}
	want := []string{"Never", "Every 6 hours", "Every 12 hours", "Daily", "Weekly"}
	if !slices.Equal(labels, want) {
		t.Errorf("filler.fetch.every presets = %v, want %v", labels, want)
	}
}
