package app

import (
	"slices"
	"testing"

	setupapi "github.com/loomarr/loomarr/internal/api/setup"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

func TestToAPIEntry_ExposesOwnerAndApplyTiming(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		apply settings.ApplyTiming
		want  string
	}{
		{name: "default is live", apply: settings.ApplyLive, want: "live"},
		{name: "generation scoped", apply: settings.ApplyRestart, want: "restart"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := toAPIEntry(settings.Entry{Setting: settings.Setting{
				Key: "test.key", Owner: settings.OwnerFillerIncoming, Kind: settings.KindString, Apply: tc.apply,
			}})
			if entry.Owner != "filler.incoming" {
				t.Errorf("owner = %q, want filler.incoming", entry.Owner)
			}
			if entry.Apply != tc.want {
				t.Errorf("apply = %q, want %q", entry.Apply, tc.want)
			}
		})
	}
}

func TestToAPIEntry_CarriesLanguageCodesForClientLocalization(t *testing.T) {
	t.Parallel()
	entry := toAPIEntry(settings.Entry{Setting: settings.Setting{
		Key: "filler.language", Owner: settings.OwnerFillerPlayback, Kind: settings.KindEnum,
		Enum: []settings.EnumOption{{Value: "en", Label: "en"}, {Value: "pl", Label: "pl"}},
	}})
	if len(entry.Enum) != 2 || entry.Enum[1] != "pl" || entry.EnumOptions[1].Label != "pl" {
		t.Fatalf("language options = %#v / %#v, want codes without English labels", entry.Enum, entry.EnumOptions)
	}
}

func TestToAPIEntry_CarriesRegistryPresets(t *testing.T) {
	t.Parallel()
	entry := toAPIEntry(settings.Entry{Setting: settings.Setting{
		Key: "filler.fetch.every", Owner: settings.OwnerFillerDownloads, Kind: settings.KindDuration,
		Presets: []settings.Preset{{Value: "0s", Label: "Never"}, {Value: "6h0m0s", Label: "Every 6 hours"}},
	}})
	want := []setupapi.SettingPreset{{Value: "0s", Label: "Never"}, {Value: "6h0m0s", Label: "Every 6 hours"}}
	if !slices.Equal(entry.Presets, want) {
		t.Fatalf("presets = %#v, want %#v", entry.Presets, want)
	}
}
