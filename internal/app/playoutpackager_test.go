package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// The packager asks for the channel's bug only for a library programme (#1512 phase 1d, which the
// retired programme route drew): a break's commercial, a filler clip and flex never carry it, and a
// host whose GPU overlay has not passed its self-check draws none.
func TestPackagerSourceWatermarksProgrammesOnly(t *testing.T) {
	ch := store.Channel{Channel: schedule.Channel{Name: "Retro Cartoons", Number: 7}}
	programme := playout.Airing{Kind: schedule.SlotProgram, LibraryItemID: "title"}
	for _, tc := range []struct {
		name       string
		airing     playout.Airing
		gatePassed bool
		want       bool
	}{
		{"programme", programme, true, true},
		{"programme, overlay unverified", programme, false, false},
		{"break commercial", playout.Airing{Kind: schedule.SlotFiller, Source: "/filler/clip.mp4"}, true, false},
		{"filler clip in a programme slot", playout.Airing{Kind: schedule.SlotProgram, Source: "/filler/clip.mp4"}, true, false},
		{"flex", playout.Airing{Kind: schedule.SlotFlex}, true, false},
	} {
		s := packagerSource{watermark: testWatermarks(t, ch, tc.gatePassed).For}
		var wm *playout.Watermark
		if resolve := s.watermarkFor("ch", tc.airing); resolve != nil {
			wm = resolve(t.Context(), playout.EncoderNVENC, 1920, 1080)
		}
		if (wm != nil) != tc.want {
			t.Errorf("%s: bug = %+v, want one: %v", tc.name, wm, tc.want)
		}
	}
}

// Every format the packager encodes maps HDR with the live playout.tone_curve, the curve the
// capacity probe measured (ToneCurve), as the retired programme route did; not the default.
func TestPackagerSourceOutputCarriesTheToneCurve(t *testing.T) {
	s := packagerSource{res: &playoutResolver{
		encoder:   func() string { return string(playout.EncoderNVENC) },
		tier:      func() string { return "balanced" },
		toneCurve: func() string { return string(playout.ToneCurveBT2390) },
	}}
	for _, class := range []playout.FormatClass{playout.FormatBaseline, playout.FormatClass("unknown")} {
		if _, out := s.Output(t.Context(), "ch", class, 0); out.ToneCurve != playout.ToneCurveBT2390 {
			t.Errorf("%s: output tone curve %q, want the live setting %q", class, out.ToneCurve, playout.ToneCurveBT2390)
		}
	}
}
