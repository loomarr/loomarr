package schedule

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// WatermarkPolicy is one channel's bug settings. Every field is a pointer or empty string so that
// nil/"" means "the approved default" and a stored value is always a real choice (no 0-means-inherit
// trap: a margin of 0 is a real, if unusual, choice).
type WatermarkPolicy struct {
	// Enabled turns the bug off with false. nil is on (the default).
	Enabled *bool `json:"enabled,omitempty" doc:"false turns the bug off; omitted is on (the default)"`
	// Corner is where the bug sits on the active picture: top-right (default), top-left,
	// bottom-right, bottom-left.
	Corner string `json:"corner,omitempty" enum:"top-right,top-left,bottom-right,bottom-left" doc:"Corner of the active picture; omitted is top-right"`
	// Opacity is 0.1–1; nil is the install setting (playout.watermark_opacity_pct, default 40%).
	Opacity *float64 `json:"opacity,omitempty" minimum:"0.1" maximum:"1" doc:"Bug opacity, 0.1-1; omitted is the install setting playout.watermark_opacity_pct (default 40%)"`
	// Look is the generated callsign bug's style (WatermarkLooks); "" is the install setting
	// (playout.watermark_look, default text). A custom image keeps its own shape.
	Look string `json:"look,omitempty" enum:"plate,text,outline,small-plate,tint,twotone,badge,lockup,glass,stripe" doc:"Style of the generated callsign bug: plate, text (the Plate's letters, no plate), outline (hollow letters), small-plate (75% size, no shadow), tint (colour plate), twotone (number + name), badge (number badge), lockup (number | name, white), glass (frosted plate) or stripe (era stripe); omitted is the install setting playout.watermark_look (default text)"`
	// Color is the coloured looks' colour (tint, twotone, badge, stripe): era (the colours of the
	// channel's decade, amber when it has none; the default) or one design-token colour. The other
	// looks are white whatever this says.
	Color string `json:"color,omitempty" enum:"era,amber,cyan,green,red,pink,yellow" doc:"Colour of the coloured looks: era (colours from the channel's decade, amber without one) or a fixed colour; omitted is era"`
	// Holiday turns the holiday version (a small picture beside the logo while a holiday is on) off
	// with false. nil is on, unless the channel's seasonal mode is off (Ignore holidays), which
	// never shows one. A custom image never gets one.
	Holiday *bool `json:"holiday,omitempty" doc:"false turns the holiday version off; omitted is on unless the channel ignores holidays (seasonal mode off)"`
	// Motifs picks the holiday picture where there is a choice (WatermarkMotifs): halloween
	// (pumpkin, bat, ghost) and christmas (holly, snowflake). An omitted holiday is its first.
	Motifs map[string]string `json:"motifs,omitempty" doc:"Holiday picture per holiday, where there is a choice: halloween is pumpkin, bat or ghost; christmas is holly or snowflake; an omitted holiday is the first"`
	// Size is the bug's height as a share of the frame height (a square bug's; wider marks keep the
	// same area), 0.02–0.15; nil is 0.06.
	Size *float64 `json:"size,omitempty" minimum:"0.02" maximum:"0.15" doc:"Bug height as a share of the frame height, 0.02-0.15; omitted is 0.06"`
	// Margin is the inset from the active picture's edges as a share of the frame width
	// (horizontal) and height (vertical), 0–0.2; nil is 0.05 (EBU R95 graphics-safe).
	Margin *float64 `json:"margin,omitempty" minimum:"0" maximum:"0.2" doc:"Inset from the picture edges as a share of the frame width and height, 0-0.2; omitted is 0.05"`
	// Image is a custom upload's image hash (role "watermark"). It wins over the automatic image.
	Image string `json:"image,omitempty" doc:"Custom bug image hash (upload with role watermark); omitted is automatic"`
	// Callsign is the text of the generated Plate bug, used when there is no custom image and no
	// network logo. Omitted derives it from the channel name.
	Callsign string `json:"callsign,omitempty" maxLength:"12" doc:"Text of the generated Plate bug; omitted derives it from the channel name"`
}

// Watermark defaults: the maintainer-approved look (2026-09-26, PR #1532). The default opacity and
// look are the install settings' (#1617), passed to ResolveWatermark.
const (
	WatermarkDefaultCorner = "top-right"
	WatermarkDefaultSize   = 0.06
	WatermarkDefaultMargin = 0.05
	watermarkMaxCallsign   = 12
)

// WatermarkLooks is every generated-bug style, in the order the API lists them: the renderer's
// watermark.Styles and the playout.watermark_look options, pinned equal by an internal/app test.
var WatermarkLooks = []string{"plate", "text", "outline", "small-plate", "tint", "twotone", "badge", "lockup", "glass", "stripe"}

// WatermarkDefaultColor is "Match the era" (#1659 Q8).
const WatermarkDefaultColor = "era"

// WatermarkColors is every policy colour: era, then the renderer's watermark.Colours (pinned equal
// by an internal/app test).
var WatermarkColors = []string{"era", "amber", "cyan", "green", "red", "pink", "yellow"}

// WatermarkMotifs is the pictures of each holiday with a holiday version, the default first (#1659
// Q13): a choice only for Halloween and Christmas. A calendar holiday missing here airs the plain
// logo (#2030). Its ids are the renderer's watermark.Motifs.
var WatermarkMotifs = map[string][]string{
	"halloween":    {"pumpkin", "bat", "ghost"},
	"thanksgiving": {"leaf"},
	"christmas":    {"holly", "snowflake"},
	"newyear":      {"firework"},
	"valentines":   {"heart"},
}

// WatermarkInstall is the install-wide watermark settings, resolved by the caller:
// playout.watermark_opacity_pct as a fraction, and playout.watermark_look.
type WatermarkInstall struct {
	Opacity float64
	Look    string
}

// ResolvedWatermark is a channel's effective bug settings.
type ResolvedWatermark struct {
	Enabled         bool
	Corner          string
	Opacity         float64
	Look            string
	Color           string
	Holiday         bool
	Size, Margin    float64
	Image, Callsign string
}

// WatermarkHoliday is the holiday version a channel's bug airs with at an airing's start: the
// calendar holiday and its picture, or "" for none. It is "" when the version is off, when the
// channel ignores holidays or limits them to others (ActiveHolidayAt), and for a custom image
// (#1659 Q20: the household's own artwork airs unchanged). cal is the calendar the schedule's
// holiday window reads.
func WatermarkHoliday(p *WatermarkPolicy, r ResolvedWatermark, seasonal SeasonalPolicy, at time.Time, cal Calendar) (holiday, motif string) {
	if !r.Holiday || r.Image != "" {
		return "", ""
	}
	holiday = ActiveHolidayAt(at, seasonal, cal)
	if motif = WatermarkMotif(p, holiday); motif == "" {
		return "", ""
	}
	return holiday, motif
}

// WatermarkMotif is the picture a channel's logo shows for the holiday: its choice when it is one of
// the holiday's pictures, else the default (the first); "" for an unknown holiday.
func WatermarkMotif(p *WatermarkPolicy, holiday string) string {
	choices := WatermarkMotifs[holiday]
	if len(choices) == 0 {
		return ""
	}
	if p != nil && slices.Contains(choices, p.Motifs[holiday]) {
		return p.Motifs[holiday]
	}
	return choices[0]
}

// WatermarkMotifNames is each holiday picture's plain name on the channel page.
var WatermarkMotifNames = map[string]string{
	"pumpkin": "Pumpkin", "bat": "Bat", "ghost": "Ghost", "leaf": "Leaf", "holly": "Holly",
	"snowflake": "Snowflake", "firework": "Firework", "heart": "Heart",
}

// ResolveWatermark applies the defaults to a channel's (possibly nil) policy. The channel's own
// opacity and look win over the install settings. Callers pass those rather than this package
// reading settings, like ResolveAudioLanguage.
func ResolveWatermark(p *WatermarkPolicy, install WatermarkInstall) ResolvedWatermark {
	r := ResolvedWatermark{Enabled: true, Corner: WatermarkDefaultCorner, Opacity: install.Opacity, Look: install.Look,
		Color: WatermarkDefaultColor, Holiday: true, Size: WatermarkDefaultSize, Margin: WatermarkDefaultMargin}
	if p == nil {
		return r
	}
	if p.Enabled != nil {
		r.Enabled = *p.Enabled
	}
	if p.Corner != "" {
		r.Corner = p.Corner
	}
	if p.Opacity != nil {
		r.Opacity = *p.Opacity
	}
	if p.Look != "" {
		r.Look = p.Look
	}
	if p.Color != "" {
		r.Color = p.Color
	}
	if p.Holiday != nil {
		r.Holiday = *p.Holiday
	}
	if p.Size != nil {
		r.Size = *p.Size
	}
	if p.Margin != nil {
		r.Margin = *p.Margin
	}
	r.Image, r.Callsign = strings.TrimSpace(p.Image), strings.TrimSpace(p.Callsign)
	return r
}

// WatermarkEraDecade is the decade a channel's era sits in (1980 for the 1980s), the key
// of "Match the era" (watermark.EraPalette), or 0 when the channel has no era. The era is the one the
// Programming tab shows (the date scope's era range: the same single range on all three axes), so what airs and
// what the page says cannot disagree; any other date scope, an open-ended range or one spanning
// decades has none (amber).
func WatermarkEraDecade(scope ScopePolicy) int {
	r, ok := scope.Dates.Era()
	if !ok || r.From <= 0 || r.To < r.From || r.From/10 != r.To/10 {
		return 0
	}
	return r.From / 10 * 10
}

func (p *WatermarkPolicy) validate() error {
	if p == nil {
		return nil
	}
	switch p.Corner {
	case "", "top-right", "top-left", "bottom-right", "bottom-left":
	default:
		return fmt.Errorf("watermark.corner %q is not a corner", p.Corner)
	}
	if p.Look != "" && !slices.Contains(WatermarkLooks, p.Look) {
		return fmt.Errorf("watermark.look %q is not one of %v", p.Look, WatermarkLooks)
	}
	if p.Color != "" && !slices.Contains(WatermarkColors, p.Color) {
		return fmt.Errorf("watermark.color %q is not one of %v", p.Color, WatermarkColors)
	}
	for holiday, motif := range p.Motifs {
		choices, ok := WatermarkMotifs[holiday]
		if !ok {
			return fmt.Errorf("watermark.motifs: %q is not a calendar holiday", holiday)
		}
		if !slices.Contains(choices, motif) {
			return fmt.Errorf("watermark.motifs.%s %q is not one of %v", holiday, motif, choices)
		}
	}
	for _, f := range []struct {
		name     string
		v        *float64
		min, max float64
	}{{"opacity", p.Opacity, 0.1, 1}, {"size", p.Size, 0.02, 0.15}, {"margin", p.Margin, 0, 0.2}} {
		if f.v != nil && (*f.v < f.min || *f.v > f.max || *f.v != *f.v) {
			return fmt.Errorf("watermark.%s %v out of range (%v–%v)", f.name, *f.v, f.min, f.max)
		}
	}
	if len([]rune(strings.TrimSpace(p.Callsign))) > watermarkMaxCallsign {
		return fmt.Errorf("watermark.callsign longer than %d characters", watermarkMaxCallsign)
	}
	return nil
}
