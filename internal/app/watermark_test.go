package app

import (
	"context"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/channelbug"
	"github.com/loomarr/loomarr/internal/playout/watermark"
)

// testWatermarks is the watermark service over the install's default settings, its NVENC self-check
// verdict set to gatePassed (colour verified).
func testWatermarks(t *testing.T, ch channelstore.Channel, gatePassed bool) *channelWatermarks {
	return testWatermarksWith(t, visionSet(t, nil), ch, gatePassed)
}

// testWatermarksWith reads the install-wide opacity and look from set, a live settings service over
// the declared registry, the way the build wires it.
func testWatermarksWith(t *testing.T, set resolved, ch channelstore.Channel, gatePassed bool) *channelWatermarks {
	svc := channelbug.New(channelbug.Config{Dir: t.TempDir(), Channels: staticChannelReader{channel: ch},
		Log: slog.New(slog.DiscardHandler), Install: watermarkInstall(set)})
	svc.SetGateResult(encode.EncoderNVENC, gatePassed, false)
	return &channelWatermarks{svc}
}

// THE INSTALL SETTINGS (#1617): playout.watermark_look and playout.watermark_opacity_pct reach the
// service as the install-wide look and opacity, and a change applies to the next bug asked for.
func TestWatermarkInstall_ReadsTheSettingsLive(t *testing.T) {
	set := visionSet(t, map[string]string{"playout.watermark_opacity_pct": "55"})
	install := watermarkInstall(set)
	if got := install(); got.Opacity != 0.55 || got.Look != "text" {
		t.Fatalf("install = %+v, want 55%% Text", got)
	}
	set.svc.SetDB(map[string]string{"playout.watermark_look": "plate", "playout.watermark_opacity_pct": "70"})
	if got := install(); got.Opacity != 0.70 || got.Look != "plate" {
		t.Fatalf("after the change install = %+v, want 70%% Plate", got)
	}
}

// The channel page's options leave the service as the API's own types, every field carried over.
func TestChannelWatermarks_OptionsAreTheAPIShape(t *testing.T) {
	ch := channelstore.Channel{Channel: schedule.Channel{ID: "ch-1", Name: "Saturday Cartoons", Number: 7}}
	ch.Policy.Scope.Dates = schedule.EraDates(schedule.Range{From: 1980, To: 1989})
	ch.Policy.Seasonal = schedule.SeasonalPolicy{Holidays: []string{"halloween"}}
	ch.Policy.Watermark = &schedule.WatermarkPolicy{Motifs: map[string]string{"halloween": "ghost"}}
	oct13 := time.Date(2026, time.October, 13, 20, 0, 0, 0, time.UTC)

	got, err := testWatermarks(t, ch, true).Options(context.Background(), "ch-1", oct13)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoCallsign != "SC" || got.Era != "1980s" || !slices.Equal(got.EraPalette, []string{"red", "amber", "cyan"}) {
		t.Fatalf("name/era = %q %q %v", got.AutoCallsign, got.Era, got.EraPalette)
	}
	if len(got.Holidays) != 1 {
		t.Fatalf("holidays = %+v, want halloween only", got.Holidays)
	}
	hw := got.Holidays[0]
	if hw.ID != "halloween" || hw.Name != "Halloween" || hw.Start != "2026-10-01" || hw.End != "2026-10-31" || !hw.Active ||
		hw.Picture != "ghost" || len(hw.Pictures) != 3 || hw.Pictures[0] != (channelsapi.WatermarkPicture{ID: "pumpkin", Name: "Pumpkin"}) {
		t.Fatalf("halloween = %+v", hw)
	}
}

// ONE LIST, THREE COPIES: the renderer's styles, the policy's looks and the setting's options are
// declared in three packages that must not import each other; this pins them equal.
func TestWatermarkLooks_AgreeAcrossRendererPolicyAndSetting(t *testing.T) {
	var styles []string
	for _, s := range watermark.DesignStyles {
		styles = append(styles, string(s))
	}
	s, ok := settings.NewRegistry().Get("playout.watermark_look")
	if !ok {
		t.Fatal("playout.watermark_look not declared")
	}
	var options []string
	for _, o := range s.Enum {
		options = append(options, o.Value)
	}
	if !slices.Equal(styles, schedule.WatermarkLooks) || !slices.Equal(options, schedule.WatermarkLooks) {
		t.Errorf("renderer %v, policy %v, setting %v", styles, schedule.WatermarkLooks, options)
	}
	field, _ := reflect.TypeFor[schedule.WatermarkPolicy]().FieldByName("Look")
	if tag := field.Tag.Get("enum"); tag != strings.Join(schedule.WatermarkLooks, ",") {
		t.Errorf("policy enum tag %q, want %v", tag, schedule.WatermarkLooks)
	}
}
