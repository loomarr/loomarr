package schedule

import (
	"encoding/json"
	"testing"
)

// install is an install's watermark settings as the caller resolves them.
var install = WatermarkInstall{Opacity: 0.55, Look: "outline"}

// ON BY DEFAULT with the approved look; a stored value is always a real choice.
func TestResolveWatermark_DefaultsAndOverrides(t *testing.T) {
	// Opacity and look: the channel's override, else the install settings
	// (playout.watermark_opacity_pct, playout.watermark_look).
	if got := ResolveWatermark(nil, install); got != (ResolvedWatermark{Enabled: true, Corner: "top-right", Opacity: 0.55, Look: "outline", Size: 0.06, Margin: 0.05}) {
		t.Errorf("nil policy: %+v", got)
	}
	if got := ResolveWatermark(&WatermarkPolicy{Corner: "top-left"}, install); got.Opacity != 0.55 || got.Look != "outline" {
		t.Errorf("omitted opacity and look: %v %q, want the install settings 0.55 outline", got.Opacity, got.Look)
	}
	var p WatermarkPolicy
	if err := json.Unmarshal([]byte(`{"enabled":false,"corner":"bottom-left","opacity":0.8,"look":"small-plate","size":0.045,"margin":0,"callsign":" RETRO "}`), &p); err != nil {
		t.Fatal(err)
	}
	got := ResolveWatermark(&p, install)
	if got.Enabled || got.Corner != "bottom-left" || got.Opacity != 0.8 || got.Look != "small-plate" || got.Size != 0.045 || got.Margin != 0 || got.Callsign != "RETRO" {
		t.Errorf("overrides: %+v", got)
	}
}

func TestWatermarkPolicy_Validate(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, bad := range []WatermarkPolicy{
		{Corner: "middle"}, {Opacity: f(0)}, {Opacity: f(1.2)}, {Size: f(0.5)}, {Margin: f(-0.1)},
		{Callsign: "THIRTEENCHARS"}, {Look: "fancy"},
	} {
		if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &bad}}).Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	for _, look := range WatermarkLooks {
		ok := WatermarkPolicy{Corner: "top-left", Opacity: f(1), Look: look, Size: f(0.02), Margin: f(0), Callsign: "KIDS"}
		if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &ok}}).Validate(); err != nil {
			t.Errorf("rejected %+v: %v", ok, err)
		}
	}
}
