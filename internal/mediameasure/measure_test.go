package mediameasure

import (
	"context"
	"math"
	"testing"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestKeyframes_IndexesEverySyncPacketWithPositions(t *testing.T) {
	frames, err := DefaultTools("", "").Keyframes(context.Background(), fadeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 7 {
		t.Fatalf("keyframes = %d %v, want one per second of the 7 s fixture", len(frames), frames)
	}
	for i, frame := range frames {
		if want := int64(i) * 1000; frame.PTSMs < want-40 || frame.PTSMs > want+40 {
			t.Errorf("keyframe %d pts = %d ms, want ~%d", i, frame.PTSMs, want)
		}
		if frame.Offset <= 0 {
			t.Errorf("keyframe %d has no byte offset (%d)", i, frame.Offset)
		}
		if i > 0 && (frame.PTSMs <= frames[i-1].PTSMs || frame.Offset <= frames[i-1].Offset) {
			t.Errorf("keyframe %d is not after keyframe %d: %v", i, i-1, frames)
		}
	}
}

func TestDecodePass_MeasuresSpansAndLoudness(t *testing.T) {
	quality, loudness, err := DefaultTools("", "").Decode(context.Background(), fadeFixture(t), 7000, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(quality.Black) != 1 || quality.Black[0].StartMs < 2900 || quality.Black[0].StartMs > 3100 {
		t.Fatalf("black spans = %v, want one starting ~3000 ms", quality.Black)
	}
	if len(quality.Silence) != 1 || quality.Silence[0].StartMs < 2900 || quality.Silence[0].StartMs > 3150 {
		t.Fatalf("silence spans = %v, want one starting ~3000 ms", quality.Silence)
	}
	if !loudness.Available || loudness.IntegratedLUFS > -10 || loudness.IntegratedLUFS < -60 ||
		math.IsNaN(loudness.IntegratedLUFS) {
		t.Fatalf("loudness = %+v, want a finite integrated LUFS for a tone", loudness)
	}
	if loudness.TruePeak.State != mediatools.TruePeakFinite {
		t.Fatalf("true peak = %+v, want finite", loudness.TruePeak)
	}
}

func TestBreakCandidates_NeedBlackAndSilenceTogether(t *testing.T) {
	kf := []inventory.Keyframe{{PTSMs: 0}, {PTSMs: 1000}, {PTSMs: 3000}, {PTSMs: 4000}}
	black := []mediatools.Interval{{StartMs: 3000, EndMs: 4000}, {StartMs: 6000, EndMs: 6500}}
	silence := []mediatools.Interval{{StartMs: 3050, EndMs: 3900}, {StartMs: 8000, EndMs: 9000}}
	got := BreakCandidates(black, silence, kf, 10_000)
	if len(got) != 1 {
		t.Fatalf("breaks = %+v, want exactly the one black+silence coincidence", got)
	}
	b := got[0]
	if b.AtMs != 3475 || b.OverlapMs != 850 {
		t.Errorf("break = %+v, want midpoint 3475 ms over an 850 ms overlap", b)
	}
	if b.KeyframeMs != 4000 {
		t.Errorf("keyframe = %d, want the first keyframe at or after the break (4000)", b.KeyframeMs)
	}
	if b.Confidence <= 0.5 || b.Confidence > 1 {
		t.Errorf("confidence = %v, want high for an 850 ms coincidence", b.Confidence)
	}
}

func TestBreakCandidates_ShortCoincidenceIsLowConfidenceAndEdgesAreDropped(t *testing.T) {
	black := []mediatools.Interval{{StartMs: 0, EndMs: 800}, {StartMs: 5000, EndMs: 5150}, {StartMs: 9700, EndMs: 10_000}}
	silence := []mediatools.Interval{{StartMs: 0, EndMs: 900}, {StartMs: 5000, EndMs: 5150}, {StartMs: 9700, EndMs: 10_000}}
	got := BreakCandidates(black, silence, nil, 10_000)
	if len(got) != 1 || got[0].AtMs != 5075 {
		t.Fatalf("breaks = %+v, want only the mid-programme 5075 ms candidate (opening and closing fades are not breaks)", got)
	}
	if got[0].Confidence >= 0.5 {
		t.Errorf("confidence = %v for a 150 ms coincidence, want low", got[0].Confidence)
	}
	if got[0].KeyframeMs != 0 {
		t.Errorf("keyframe = %d with no index, want 0", got[0].KeyframeMs)
	}
}
