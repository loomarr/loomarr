package schedule

import (
	"encoding/json"
	"testing"
)

// ON BY DEFAULT with the approved look; a stored value is always a real choice.
func TestResolveWatermark_DefaultsAndOverrides(t *testing.T) {
	// Opacity: the channel's override, else the install setting (playout.watermark_opacity_pct).
	if got := ResolveWatermark(nil, 0.55); got != (ResolvedWatermark{Enabled: true, Corner: "top-right", Opacity: 0.55, Size: 0.06, Margin: 0.05}) {
		t.Errorf("nil policy: %+v", got)
	}
	if got := ResolveWatermark(&WatermarkPolicy{Corner: "top-left"}, 0.55); got.Opacity != 0.55 {
		t.Errorf("omitted opacity: %v, want the install setting 0.55", got.Opacity)
	}
	var p WatermarkPolicy
	if err := json.Unmarshal([]byte(`{"enabled":false,"corner":"bottom-left","opacity":0.8,"size":0.045,"margin":0,"callsign":" RETRO "}`), &p); err != nil {
		t.Fatal(err)
	}
	got := ResolveWatermark(&p, 0.55)
	if got.Enabled || got.Corner != "bottom-left" || got.Opacity != 0.8 || got.Size != 0.045 || got.Margin != 0 || got.Callsign != "RETRO" {
		t.Errorf("overrides: %+v", got)
	}
}

func TestWatermarkPolicy_Validate(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, bad := range []WatermarkPolicy{
		{Corner: "middle"}, {Opacity: f(0)}, {Opacity: f(1.2)}, {Size: f(0.5)}, {Margin: f(-0.1)},
		{Callsign: "THIRTEENCHARS"},
	} {
		if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &bad}}).Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	ok := WatermarkPolicy{Corner: "top-left", Opacity: f(1), Size: f(0.02), Margin: f(0), Callsign: "KIDS"}
	if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &ok}}).Validate(); err != nil {
		t.Errorf("rejected %+v: %v", ok, err)
	}
}
