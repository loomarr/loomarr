package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
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
		s := playoutresolve.PackagerSource{Watermark: testWatermarks(t, ch, tc.gatePassed).For}
		var wm *playout.Watermark
		if resolve := s.WatermarkFor("ch", tc.airing); resolve != nil {
			wm = resolve(t.Context(), encode.EncoderNVENC, 1920, 1080)
		}
		if (wm != nil) != tc.want {
			t.Errorf("%s: bug = %+v, want one: %v", tc.name, wm, tc.want)
		}
	}
}
