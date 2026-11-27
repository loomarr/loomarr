package filler_test

import (
	"context"
	"testing"
)

// A decade the model estimates from picture and sound is kept only at high confidence and only
// when no year was read off the screen, and it is stored as the decade's first year (#3838
// decision 7). The estimate is for the rung to hand on as inference-ranked evidence, which any
// literal evidence replaces.
func TestVisionStage_KeepsAHighConfidenceDecadeEstimate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		want   int
	}{
		{"high confidence, a decade", `{"visibleText":"","eraDecade":1990,"eraConfidence":"high"}`, 1990},
		{"high confidence, a year snaps to its decade", `{"visibleText":"","eraDecade":1993,"eraConfidence":"high"}`, 1990},
		{"low confidence", `{"visibleText":"","eraDecade":1990,"eraConfidence":"low"}`, 0},
		{"no confidence stated", `{"visibleText":"","eraDecade":1990}`, 0},
		{"outside the supported years", `{"visibleText":"","eraDecade":1800,"eraConfidence":"high"}`, 0},
		{"a literal year wins, so there is no estimate", `{"visibleText":"COPYRIGHT 1987","era":1987,"eraDecade":1990,"eraConfidence":"high"}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newVisionStage(newFakeVisionStore(), &scriptedVision{frames: oneFrame}, &scriptedProvider{answer: tc.answer})
			_, observation, err := s.Observe(context.Background(), wordless(visionClip("silent.mp4")))
			if err != nil {
				t.Fatal(err)
			}
			if observation.EstimatedDecade != tc.want {
				t.Errorf("estimated decade = %d, want %d", observation.EstimatedDecade, tc.want)
			}
		})
	}
}
