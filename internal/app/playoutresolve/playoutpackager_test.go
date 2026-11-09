package playoutresolve

import (
	"testing"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

// Every format the packager encodes maps HDR with the live playout.tone_curve, the curve the
// capacity probe measured (ToneCurve), as the retired programme route did; not the default.
func TestPackagerSourceOutputCarriesTheToneCurve(t *testing.T) {
	s := PackagerSource{Res: &Resolver{
		encoder:   func() string { return string(encode.EncoderNVENC) },
		tier:      func() string { return "balanced" },
		toneCurve: func() string { return string(playout.ToneCurveBT2390) },
	}}
	for _, class := range []playout.FormatClass{playout.FormatBaseline, playout.FormatClass("unknown")} {
		if _, out := s.Output(t.Context(), "ch", class, 0); out.ToneCurve != playout.ToneCurveBT2390 {
			t.Errorf("%s: output tone curve %q, want the live setting %q", class, out.ToneCurve, playout.ToneCurveBT2390)
		}
	}
}
