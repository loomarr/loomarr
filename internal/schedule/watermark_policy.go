package schedule

import (
	"fmt"
	"strings"
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
	// Opacity is 0.1–1; nil is 0.65.
	Opacity *float64 `json:"opacity,omitempty" minimum:"0.1" maximum:"1" doc:"Bug opacity, 0.1-1; omitted is 0.65"`
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

// Watermark defaults: the maintainer-approved look (2026-09-26, PR #1532).
const (
	WatermarkDefaultCorner  = "top-right"
	WatermarkDefaultOpacity = 0.65
	WatermarkDefaultSize    = 0.06
	WatermarkDefaultMargin  = 0.05
	watermarkMaxCallsign    = 12
)

// ResolvedWatermark is a channel's effective bug settings.
type ResolvedWatermark struct {
	Enabled               bool
	Corner                string
	Opacity, Size, Margin float64
	Image, Callsign       string
}

// ResolveWatermark applies the defaults to a channel's (possibly nil) policy.
func ResolveWatermark(p *WatermarkPolicy) ResolvedWatermark {
	r := ResolvedWatermark{Enabled: true, Corner: WatermarkDefaultCorner, Opacity: WatermarkDefaultOpacity,
		Size: WatermarkDefaultSize, Margin: WatermarkDefaultMargin}
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
	if p.Size != nil {
		r.Size = *p.Size
	}
	if p.Margin != nil {
		r.Margin = *p.Margin
	}
	r.Image, r.Callsign = strings.TrimSpace(p.Image), strings.TrimSpace(p.Callsign)
	return r
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
