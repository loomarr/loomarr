package filler

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// The silence guard, against the REAL audio that caused a false positive (§10 V40).
//
// ⚠ **This is the regression from a live run, not a hypothetical.** "USA Network Commercial Breaks
// (10-6-1994)" — a 978-second recorded ad break — was detected as ARABIC and tombstoned. The cause
// was not the model: `LanguageSpan` handed it the first ten seconds, which on a long recording is
// leader (tape run-up, dead air) measuring **-70 LUFS**. Asked what language silence is in, a model
// does not decline — it guesses.
//
// ⚠ And the guess is ARBITRARY, which is what makes it dangerous. Re-asked about the same leader
// span later it answered `en`; on the run that deleted the clip it answered `ar`. Not reliably
// wrong — reliably *unpredictable*, so no amount of prompt tuning makes acting on it safe.
//
// Two fixes, and this covers the load-bearing one:
//
//   - `LanguageSpan` now samples a long recording from its MIDDLE. That fixes WHERE we look.
//   - `spanIsSilent` refuses to ask at all below the floor. That holds WHEREVER we land, including
//     on a clip that is genuinely silent throughout.
//
// The fixtures are the real spans, downsampled to 8kHz/3s to keep them small; both preserve their
// integrated loudness (-70.0 and -25.3 LUFS).
func TestSpanIsSilent_TheLeaderThatDeletedAClip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, tc := range []struct {
		file, what string
		want       bool
	}{
		{"span_leader_silence.wav", "the -70 LUFS leader a model answered 'ar' about", true},
		{"span_real_speech.wav", "the -25 LUFS middle of the same recording", false},
	} {
		path := filepath.Join("..", "testkit", "fixtures", "whisper", tc.file)
		got, err := spanIsSilent(context.Background(), "ffmpeg", path)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		if got != tc.want {
			t.Errorf("%s: silent=%v, want %v", tc.what, got, tc.want)
		}
	}
}

// ⚠ The floor must not mistake a QUIET clip for a silent one. The quietest real clip measured in
// this catalog is -32.6 LUFS (§10 V40's loudness work) and it must still be judged — rejecting a
// quiet advert as "no speech" would be the same class of bug in the other direction.
func TestSilenceFloor_LeavesRoomForTheQuietestRealClip(t *testing.T) {
	const quietestMeasured = -32.6
	if silenceFloorLUFS >= quietestMeasured {
		t.Errorf("floor %.1f LUFS is at or above the quietest real clip (%.1f) — a quiet advert "+
			"would be treated as silent and never judged", silenceFloorLUFS, quietestMeasured)
	}
}

// The span rule itself: a long recording is sampled from the middle, a normal advert is not.
func TestLanguageSpan_LongRecordingsAreSampledFromTheMiddle(t *testing.T) {
	// The real clip: 978.767s.
	start, end := LanguageSpan(978_767)
	if start < 400_000 {
		t.Errorf("span starts at %dms — still in the leader that caused the false positive", start)
	}
	if end-start != LanguageSampleMs {
		t.Errorf("span is %dms, want %dms", end-start, LanguageSampleMs)
	}

	// ⚠ A normal advert is UNCHANGED. The middle of a 30-second spot is no better than 1s in, and
	// changing it would invalidate the behaviour already verified against real clips.
	if s, e := LanguageSpan(30_000); s != 1_000 || e != 11_000 {
		t.Errorf("30s advert span = [%d,%d), want [1000,11000) unchanged", s, e)
	}
}
