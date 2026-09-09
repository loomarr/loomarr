package playoutcert

import (
	"math"
	"testing"
	"time"
)

func programmeTruthFixture() ([]ExpectedProgramme, ProgrammeMediaClock) {
	origin := time.Unix(1000, 0).UTC()
	programmes := make([]ExpectedProgramme, 3)
	for index := range programmes {
		programmes[index] = ExpectedProgramme{StartsAt: origin.Add(time.Duration(index) * 2 * time.Second), EndsAt: origin.Add(time.Duration(index+1) * 2 * time.Second), Luma: SignalRange{0, 25}, ZeroCrossingRate: SignalRange{0.012, 0.026}, RMSDB: SignalRange{-80, -1}}
		if index%2 == 1 {
			programmes[index].Luma = SignalRange{225, 255}
			programmes[index].ZeroCrossingRate = SignalRange{0.027, 0.050}
		}
	}
	return programmes, ProgrammeMediaClock{Origin: origin, Generation: "one"}
}

func TestProgrammeAudioPrerollCannotQualifyContentOrHideLaterMismatch(t *testing.T) {
	programmes, clock := programmeTruthFixture()
	check := newProgrammeSignalCheck(programmes, clock.Origin.Add(2100*time.Millisecond))
	// Captured AAC startup measurements: the following frames carry the correct
	// 880 Hz signal, while codec priming itself is outside that content signature.
	priming := DecodedAudioSignal{PTSUS: 2_300_000, Samples: 1024, ZeroCrossingRate: 0.053711, RMSDB: -37.044586}
	if err := check.checkAudioSignal(clock, priming, true); err != nil {
		t.Fatal(err)
	}
	if check.audioSamples != 0 || check.audio[1] != 0 || !check.lastMatchedAudio.IsZero() || check.transition >= 0 {
		t.Fatal("codec priming counted as programme evidence")
	}
	content := priming
	content.PTSUS += 21_333
	content.ZeroCrossingRate = 0.036133
	content.RMSDB = -22.412026
	if err := check.audioSignal(clock, content); err != nil || check.audio[1] != 1024 {
		t.Fatalf("first content frame: samples=%d err=%v", check.audio[1], err)
	}
	content.PTSUS += 21_333
	content.ZeroCrossingRate = priming.ZeroCrossingRate
	if err := check.audioSignal(clock, content); err == nil || err.Error() != "programme_audio_mismatch" {
		t.Fatalf("later corrupt audio was not rejected: %v", err)
	}
	if err := check.audioSignal(clock, priming); err == nil || err.Error() != "audio_time_regressed" {
		t.Fatalf("priming did not establish strict timestamp ordering: %v", err)
	}
}

func TestProgrammeAudioPrerollRequiresOneValidAACFrame(t *testing.T) {
	for name, signal := range map[string]DecodedAudioSignal{
		"short":          {PTSUS: 300_000, Samples: 1023, ZeroCrossingRate: 0.05, RMSDB: -30},
		"long":           {PTSUS: 300_000, Samples: 1025, ZeroCrossingRate: 0.05, RMSDB: -30},
		"invalid signal": {PTSUS: 300_000, Samples: 1024, ZeroCrossingRate: math.NaN(), RMSDB: -30},
	} {
		t.Run(name, func(t *testing.T) {
			programmes, clock := programmeTruthFixture()
			check := newProgrammeSignalCheck(programmes, clock.Origin)
			if err := check.checkAudioSignal(clock, signal, true); err == nil || err.Error() != "invalid_audio_signal" {
				t.Fatalf("invalid priming admitted: %v", err)
			}
		})
	}
}

func feedProgrammeSignals(t *testing.T, check *programmeSignalCheck, clock ProgrammeMediaClock, index int, audio bool) {
	t.Helper()
	luma, zero := 16.0, 0.02
	if index%2 == 1 {
		luma, zero = 235, 0.04
	}
	for frame := range 2 {
		pts := int64(index)*2_000_000 + 300_000 + int64(frame)*40_000
		if err := check.videoSignal(clock, DecodedVideoSignal{PTSUS: pts, Luma: luma}); err != nil {
			t.Fatal(err)
		}
		if audio {
			if err := check.audioSignal(clock, DecodedAudioSignal{PTSUS: pts, Samples: 1024, ZeroCrossingRate: zero, RMSDB: -20}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestProgrammeSignalCheckRequiresBothSignalsForPredeclaredSuccession(t *testing.T) {
	programmes, clock := programmeTruthFixture()
	check := newProgrammeSignalCheck(programmes, clock.Origin.Add(100*time.Millisecond))
	feedProgrammeSignals(t, check, clock, 0, true)
	feedProgrammeSignals(t, check, clock, 1, false)
	if check.transition >= 0 {
		t.Fatal("video-only transition qualified")
	}
	for _, pts := range []int64{2_300_000, 2_340_000} {
		if err := check.audioSignal(clock, DecodedAudioSignal{PTSUS: pts, Samples: 1024, ZeroCrossingRate: 0.04, RMSDB: -20}); err != nil {
			t.Fatal(err)
		}
	}
	if check.transition != 1 || check.videoFrames != 4 || check.audioSamples != 4096 {
		t.Fatalf("qualified state=%+v", check)
	}
}

func TestProgrammeSignalCheckCannotChooseSequenceFromObservedProgrammes(t *testing.T) {
	programmes, clock := programmeTruthFixture()
	check := newProgrammeSignalCheck(programmes, clock.Origin.Add(100*time.Millisecond))
	feedProgrammeSignals(t, check, clock, 1, true)
	feedProgrammeSignals(t, check, clock, 2, true)
	if check.transition >= 0 || check.initial != 0 {
		t.Fatal("skipped expected first programme by fitting observed succession")
	}
}

func TestProgrammeSignalCheckExcludesBufferedPreArmMedia(t *testing.T) {
	programmes, clock := programmeTruthFixture()
	check := newProgrammeSignalCheck(programmes, clock.Origin.Add(2100*time.Millisecond))
	feedProgrammeSignals(t, check, clock, 0, true)
	feedProgrammeSignals(t, check, clock, 1, true)
	if check.transition >= 0 || check.video[0] != 0 || check.audio[0] != 0 {
		t.Fatal("buffered pre-arm media qualified transition")
	}
	feedProgrammeSignals(t, check, clock, 2, true)
	if check.transition != 2 {
		t.Fatal("post-arm expected succession did not qualify")
	}
}

func TestProgrammeSignalCheckRejectsWrongMediaAndInvalidCoordinates(t *testing.T) {
	for _, name := range []string{"wrong video", "wrong audio", "silence", "regression", "nan", "overflow", "missing clock"} {
		t.Run(name, func(t *testing.T) {
			programmes, clock := programmeTruthFixture()
			check := newProgrammeSignalCheck(programmes, clock.Origin)
			var err error
			switch name {
			case "wrong video":
				err = check.videoSignal(clock, DecodedVideoSignal{PTSUS: 300_000, Luma: 235})
			case "wrong audio":
				err = check.audioSignal(clock, DecodedAudioSignal{PTSUS: 300_000, Samples: 1024, ZeroCrossingRate: 0.04, RMSDB: -20})
			case "silence":
				err = check.audioSignal(clock, DecodedAudioSignal{PTSUS: 300_000, Samples: 1024, ZeroCrossingRate: 0.02, Silence: true})
			case "regression":
				feedProgrammeSignals(t, check, clock, 0, true)
				err = check.videoSignal(clock, DecodedVideoSignal{PTSUS: 320_000, Luma: 16})
			case "nan":
				err = check.videoSignal(clock, DecodedVideoSignal{PTSUS: 300_000, Luma: math.NaN()})
			case "overflow":
				err = check.videoSignal(clock, DecodedVideoSignal{PTSUS: math.MaxInt64, Luma: 16})
			case "missing clock":
				err = check.videoSignal(ProgrammeMediaClock{}, DecodedVideoSignal{PTSUS: 300_000, Luma: 16})
			}
			if err == nil {
				t.Fatal("invalid observation accepted")
			}
		})
	}
}

func TestProgrammeTruthRequiresCompleteUnambiguousContiguousSequence(t *testing.T) {
	for _, name := range []string{"valid", "gap", "overlap", "ambiguous", "invalid range", "incomplete", "empty"} {
		t.Run(name, func(t *testing.T) {
			programmes, clock := programmeTruthFixture()
			switch name {
			case "gap":
				programmes[1].StartsAt = programmes[1].StartsAt.Add(time.Second)
			case "overlap":
				programmes[1].StartsAt = programmes[1].StartsAt.Add(-time.Second)
			case "ambiguous":
				programmes[1].Luma = programmes[0].Luma
				programmes[1].ZeroCrossingRate = programmes[0].ZeroCrossingRate
			case "invalid range":
				programmes[0].Luma.Min = math.NaN()
			case "incomplete":
				programmes = programmes[:2]
			case "empty":
				programmes = nil
			}
			err := validateProgrammeSequence(programmes, clock.Origin, clock.Origin.Add(5*time.Second))
			if (err == nil) != (name == "valid") {
				t.Fatalf("sequence validation=%v", err)
			}
		})
	}
}

func TestProgrammeSignalCheckEpochResetPreservesArmAndExpectation(t *testing.T) {
	programmes, clock := programmeTruthFixture()
	check := newProgrammeSignalCheck(programmes, clock.Origin.Add(100*time.Millisecond))
	feedProgrammeSignals(t, check, clock, 0, true)
	check.beginEpoch()
	// New-epoch decoder preroll is outside the guarded programme interior.
	if err := check.audioSignal(clock, DecodedAudioSignal{PTSUS: 0, Samples: 1024, ZeroCrossingRate: 0.04, RMSDB: -60}); err != nil {
		t.Fatal(err)
	}
	if check.transition >= 0 || check.initial != 0 || check.audioSamples != 2048 {
		t.Fatal("epoch reset changed qualified truth")
	}
	feedProgrammeSignals(t, check, clock, 1, true)
	if check.transition != 1 {
		t.Fatal("expected cross-epoch succession did not qualify")
	}
}
