//go:build ffmpeg

package playout

import (
	"os"
	"testing"
)

// probeEncoder is the encoder the live class probe runs on: software by default (every CI host),
// LOOMARR_PROBE_ENCODER=h264_nvenc or h264_vaapi on a GPU host.
func probeEncoder() Encoder {
	if e := os.Getenv("LOOMARR_PROBE_ENCODER"); e != "" {
		return Encoder(e)
	}
	return EncoderSoftware
}

// The probe must MEASURE: a class it reports has a real speed and real CPU, and the HDR clip ran
// through a tone-mapper. A vacuous probe (an encode that never ran) cannot fake either number.
func TestLive_ClassProbeMeasuresEveryClassAndSelfChecksTonemap(t *testing.T) {
	bin := ffmpegBin(t)
	enc := probeEncoder()
	cfg := ClassProbeConfig{
		FFmpeg: bin, ClipDir: t.TempDir(), Encoder: enc,
		CPUTonemap: TonemapperFor(bin)(),
		GPU:        GPUFiltersFor(bin)(),
		Outputs:    []Profile{Resolve(DefaultTier, enc, 0)},
	}
	res := ProbeClassCosts(t.Context(), cfg)
	for key, cost := range res.Costs {
		t.Logf("%s @%dp: speed %.2fx, %.3f cores per stream at 1x", key.Class, key.Height, cost.Speed, cost.CPUCores)
	}
	t.Logf("tone-map self-check: %+v; failures: %v", res.Tonemap, res.Failures)

	top := cfg.Outputs[0].Height
	for _, class := range TranscodeClasses {
		c, ok := res.Costs[HDRKey(class, top, ToneCurveHable)]
		if !ok {
			t.Errorf("%s was not measured: %v", class, res.Failures)
			continue
		}
		if c.Speed <= 0 || c.CPUCores <= 0 {
			t.Errorf("%s measured speed %.2f and %.3f cores: a real encode has both", class, c.Speed, c.CPUCores)
		}
	}
	if !res.Tonemap.Ran || !res.Tonemap.OK || res.Tonemap.Stage == "" {
		t.Errorf("tone-map self-check = %+v, want a tone-mapper that ran", res.Tonemap)
	}
}

func TestLive_SessionLimitProbe(t *testing.T) {
	bin := ffmpegBin(t)
	enc := probeEncoder()
	if IsSoftwareEncoder(enc) {
		n, err := ProbeSessionLimit(t.Context(), SessionProbeConfig{FFmpeg: bin, Encoder: enc, Bound: SessionProbeBound, InUse: EncoderSessionsInUse(enc)})
		if n != 0 || err != nil {
			t.Fatalf("software host session limit = %d, %v; want 0 (no limit), no probe", n, err)
		}
		return
	}
	n, err := ProbeSessionLimit(t.Context(), SessionProbeConfig{FFmpeg: bin, Encoder: enc, Bound: SessionProbeBound, InUse: EncoderSessionsInUse(enc)})
	if err != nil || n < 1 {
		t.Fatalf("session probe on %s: %d, %v", enc, n, err)
	}
	t.Logf("%s opened %d of %d concurrent sessions", enc, n, SessionProbeBound)
}
