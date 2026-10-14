package mediatools

import "testing"

func TestQualityFromDetectorOutput_NormalisesAndClosesSpans(t *testing.T) {
	stderr := `[blackdetect] black_start:1 black_end:4 black_duration:3
[blackdetect] black_start:3.5 black_end:6 black_duration:2.5
[silencedetect] silence_start: 8
[freezedetect] freeze_start: 9`
	got := qualityFromDetectorOutput(stderr, 10_000)
	if len(got.Black) != 1 || got.Black[0] != (Interval{StartMs: 1000, EndMs: 6000}) {
		t.Errorf("black = %+v, want one unioned interval", got.Black)
	}
	if len(got.Silence) != 1 || got.Silence[0] != (Interval{StartMs: 8000, EndMs: 10000}) {
		t.Errorf("silence = %+v, want EOF-closed interval", got.Silence)
	}
	if len(got.Freeze) != 1 || got.Freeze[0] != (Interval{StartMs: 9000, EndMs: 10000}) {
		t.Errorf("freeze = %+v, want EOF-closed interval", got.Freeze)
	}
}
