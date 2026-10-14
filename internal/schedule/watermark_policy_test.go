package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

// install is an install's watermark settings as the caller resolves them.
var install = WatermarkInstall{Opacity: 0.55, Look: "outline"}

// ON BY DEFAULT with the approved look; a stored value is always a real choice.
func TestResolveWatermark_DefaultsAndOverrides(t *testing.T) {
	// Opacity and look: the channel's override, else the install settings
	// (playout.watermark_opacity_pct, playout.watermark_look).
	if got := ResolveWatermark(nil, install); got != (ResolvedWatermark{Enabled: true, Corner: "top-right", Opacity: 0.55, Look: "outline", Color: "era", Holiday: true, Size: 0.06, Margin: 0.05}) {
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

// Holiday versions (#1659 Q11, Q13, Q20): on by default, off with false, never for a channel that
// ignores holidays or for a custom image, the channel's pick where there is a choice, and the same
// holiday the scheduler boosts.
func TestWatermarkHoliday(t *testing.T) {
	halloween := time.Date(2026, time.October, 20, 20, 0, 0, 0, time.UTC)
	july := time.Date(2026, time.July, 4, 20, 0, 0, 0, time.UTC)
	off, on := false, true
	for _, c := range []struct {
		name               string
		p                  *WatermarkPolicy
		seasonal           SeasonalPolicy
		at                 time.Time
		wantHoliday, motif string
	}{
		{"default on, default picture", nil, SeasonalPolicy{}, halloween, "halloween", "pumpkin"},
		{"explicitly on", &WatermarkPolicy{Holiday: &on}, SeasonalPolicy{Mode: SeasonalAuto}, halloween, "halloween", "pumpkin"},
		{"the channel's pick", &WatermarkPolicy{Motifs: map[string]string{"halloween": "ghost"}}, SeasonalPolicy{}, halloween, "halloween", "ghost"},
		{"turned off", &WatermarkPolicy{Holiday: &off}, SeasonalPolicy{}, halloween, "", ""},
		{"Ignore holidays", &WatermarkPolicy{Holiday: &on}, SeasonalPolicy{Mode: SeasonalOff}, halloween, "", ""},
		{"holiday the channel leaves out", nil, SeasonalPolicy{Holidays: []string{"christmas"}}, halloween, "", ""},
		{"own image: paused", &WatermarkPolicy{Image: "abc"}, SeasonalPolicy{}, halloween, "", ""},
		{"no holiday on", nil, SeasonalPolicy{}, july, "", ""},
		{"Christmas", nil, SeasonalPolicy{}, time.Date(2026, time.December, 10, 0, 0, 0, 0, time.UTC), "christmas", "holly"},
		{"New Year straddles the year", nil, SeasonalPolicy{}, time.Date(2027, time.January, 1, 12, 0, 0, 0, time.UTC), "newyear", "firework"},
	} {
		r := ResolveWatermark(c.p, install)
		h, m := WatermarkHoliday(c.p, r, c.seasonal, c.at, nil)
		if h != c.wantHoliday || m != c.motif {
			t.Errorf("%s: %q %q, want %q %q", c.name, h, m, c.wantHoliday, c.motif)
		}
		if c.wantHoliday != "" && h != ActiveHolidayAt(c.at, c.seasonal, nil) {
			t.Errorf("%s: the logo's holiday disagrees with the scheduler's", c.name)
		}
	}
}

func TestWatermarkPolicy_ValidateColorAndMotifs(t *testing.T) {
	for _, bad := range []WatermarkPolicy{
		{Color: "lineup"}, {Color: "#ff0000"}, {Motifs: map[string]string{"easter": "egg"}},
		{Motifs: map[string]string{"halloween": "holly"}},
	} {
		if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &bad}}).Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	for _, color := range WatermarkColors {
		ok := WatermarkPolicy{Color: color, Motifs: map[string]string{"halloween": "bat", "christmas": "snowflake", "valentines": "heart"}}
		if err := (ChannelPolicy{OperatorPolicy: OperatorPolicy{Watermark: &ok}}).Validate(); err != nil {
			t.Errorf("rejected %+v: %v", ok, err)
		}
	}
}

func TestWatermarkEraDecade(t *testing.T) {
	for _, c := range []struct {
		dates *DateScope
		want  int
	}{
		{nil, 0},
		{&DateScope{SeriesAiring: []Range{{From: 1980, To: 1989}}}, 1980},
		{&DateScope{MovieRelease: []Range{{From: 1975, To: 1979}}, SeriesAiring: []Range{{From: 1982, To: 1988}}}, 1980},
		{&DateScope{SeriesAiring: []Range{{From: 1990}}}, 0}, // open-ended: no era
	} {
		if got := WatermarkEraDecade(ScopePolicy{Dates: c.dates}); got != c.want {
			t.Errorf("%+v: %d, want %d", c.dates, got, c.want)
		}
	}
}
