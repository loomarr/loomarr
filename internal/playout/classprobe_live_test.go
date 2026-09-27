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

// Premium is admitted only on its own measurement (#1520, #1512 G10): on a GPU host the probe
// measures the premium class at 2160p through the packager's own item builder, with a real speed and
// real CPU. A software host never airs a premium, so it measures none.
func TestLive_ClassProbeMeasuresThePremium(t *testing.T) {
	bin := ffmpegBin(t)
	enc := probeEncoder()
	cfg := ClassProbeConfig{
		FFmpeg: bin, ClipDir: t.TempDir(), Encoder: enc,
		CPUTonemap: TonemapperFor(bin)(), GPU: GPUFiltersFor(bin)(),
		Outputs: []Profile{Resolve(DefaultTier, enc, 0)},
		Classes: []StreamClass{ClassPremium4K},
	}
	res := ProbeClassCosts(t.Context(), cfg)
	c, ok := res.Costs[CostKey{Class: ClassPremium4K, Height: premiumHeight}]
	t.Logf("premium: %+v (measured %v); failures: %v", c, ok, res.Failures)
	if IsSoftwareEncoder(enc) {
		if ok {
			t.Fatalf("a software host measured a premium it never airs: %+v", c)
		}
		return
	}
	if !ok || c.Speed <= 0 || c.CPUCores <= 0 {
		t.Fatalf("premium cost = %+v (measured %v), want a real speed and CPU: %v", c, ok, res.Failures)
	}
	for k := range res.Costs {
		if k.Class != ClassPremium4K {
			t.Errorf("a premium-only run measured %s", k.Class)
		}
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
