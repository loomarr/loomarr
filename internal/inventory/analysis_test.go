package inventory

import (
	"reflect"
	"testing"
)

func TestKeyframeCodecRoundTripsAndStaysCompact(t *testing.T) {
	var frames []Keyframe
	for i := int64(0); i < 3600; i++ { // two hours at a 2 s GOP
		frames = append(frames, Keyframe{PTSMs: i * 2000, Offset: i*1_500_000 + (i%7)*13})
	}
	blob := EncodeKeyframes(frames)
	got, err := DecodeKeyframes(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, frames) {
		t.Fatal("round trip changed the index")
	}
	if per := float64(len(blob)) / float64(len(frames)); per > 8 {
		t.Fatalf("%.1f bytes per keyframe, want a compact (<=8, JSON is ~40) encoding", per)
	}
}

func TestKeyframeCodecKeepsMissingOffsets(t *testing.T) {
	frames := []Keyframe{{PTSMs: 0, Offset: -1}, {PTSMs: 2000, Offset: 900}, {PTSMs: 4000, Offset: -1}}
	got, err := DecodeKeyframes(EncodeKeyframes(frames))
	if err != nil || !reflect.DeepEqual(got, frames) {
		t.Fatalf("got %v err %v, want %v", got, err, frames)
	}
}

func TestDecodeKeyframesRejectsDamage(t *testing.T) {
	blob := EncodeKeyframes([]Keyframe{{PTSMs: 0, Offset: 1}, {PTSMs: 1000, Offset: 2}})
	for name, damaged := range map[string][]byte{
		"truncated": blob[:len(blob)-1], "trailing": append(append([]byte{}, blob...), 0x7f), "empty": nil,
	} {
		if _, err := DecodeKeyframes(damaged); err == nil {
			t.Errorf("%s blob decoded without error", name)
		}
	}
}

func TestAnalysisSeeksAndBreakWindows(t *testing.T) {
	a := Analysis{
		Keyframes: []Keyframe{{PTSMs: 0}, {PTSMs: 2000}, {PTSMs: 4000}},
		Breaks:    []Break{{AtMs: 1000, Confidence: 0.9}, {AtMs: 3000, Confidence: 0.3}, {AtMs: 3500, Confidence: 0.8}},
	}
	if k, ok := a.KeyframeAtOrBefore(3999); !ok || k.PTSMs != 2000 {
		t.Errorf("at-or-before 3999 = %v %v, want 2000", k, ok)
	}
	if k, ok := a.KeyframeAtOrAfter(2001); !ok || k.PTSMs != 4000 {
		t.Errorf("at-or-after 2001 = %v %v, want 4000", k, ok)
	}
	if _, ok := a.KeyframeAtOrAfter(4001); ok {
		t.Error("a keyframe after the last one was invented")
	}
	if got := a.BreaksBetween(2000, 4000, 0.5); len(got) != 1 || got[0].AtMs != 3500 {
		t.Errorf("breaks in window = %v, want only the confident one at 3500", got)
	}
}
