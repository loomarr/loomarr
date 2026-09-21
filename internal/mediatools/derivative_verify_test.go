package mediatools

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFastStartReadsMeasuredTopLevelAtomOrder(t *testing.T) {
	box := func(kind string) []byte {
		raw := make([]byte, 8)
		binary.BigEndian.PutUint32(raw[:4], uint32(len(raw)))
		copy(raw[4:], kind)
		return raw
	}
	write := func(t *testing.T, order ...string) string {
		t.Helper()
		var raw []byte
		for _, kind := range order {
			raw = append(raw, box(kind)...)
		}
		path := filepath.Join(t.TempDir(), "candidate.mp4")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if fast, err := verifyFastStart(write(t, "ftyp", "free", "moov", "mdat")); err != nil || !fast {
		t.Fatalf("fast-start MP4 = %v, %v", fast, err)
	}
	if fast, err := verifyFastStart(write(t, "ftyp", "mdat", "moov")); err != nil || fast {
		t.Fatalf("tail-moov MP4 = %v, %v", fast, err)
	}
	malformed := write(t, "ftyp", "moov", "mdat")
	if err := os.WriteFile(malformed, []byte{0, 0, 0, 4, 'm', 'o', 'o', 'v'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyFastStart(malformed); err == nil {
		t.Fatal("malformed MP4 box was accepted")
	}
}

func TestValidateDerivativeQCEnforcesMeasuredRecipeProperties(t *testing.T) {
	base := DerivativeQC{
		Version: DerivativeQCVersion, FastStart: true, CompleteDecode: true, Seekable: true,
		FirstVideoKeyframeMs: 0, MaxVideoKeyframeGapMs: 2_000, TerminalKeyframeGapMs: 1_900,
		Loudness: ConditioningLoudness{IntegratedLUFS: -23, Available: true,
			TruePeak: ConditioningTruePeak{State: TruePeakFinite, DBTP: -2}},
	}
	if err := ValidateDerivativeQC(base, 30_000, 2, true, -23); err != nil {
		t.Fatal(err)
	}
	preservedVFR := base
	preservedVFR.MaxVideoKeyframeGapMs = 1_368
	preservedVFR.TerminalKeyframeGapMs = 631
	if err := ValidateDerivativeQC(preservedVFR, 3_000, 1, true, -23); err != nil {
		t.Fatalf("preserved VFR keyframe cadence was rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*DerivativeQC)
		want   string
	}{
		{name: "not fast start", mutate: func(q *DerivativeQC) { q.FastStart = false }, want: "incomplete"},
		{name: "not completely decoded", mutate: func(q *DerivativeQC) { q.CompleteDecode = false }, want: "incomplete"},
		{name: "not seekable", mutate: func(q *DerivativeQC) { q.Seekable = false }, want: "incomplete"},
		{name: "late first keyframe", mutate: func(q *DerivativeQC) { q.FirstVideoKeyframeMs = 300 }, want: "keyframe"},
		{name: "unbounded GOP", mutate: func(q *DerivativeQC) { q.MaxVideoKeyframeGapMs = 2_501 }, want: "keyframe"},
		{name: "loudness target miss", mutate: func(q *DerivativeQC) { q.Loudness.IntegratedLUFS = -18 }, want: "missed target"},
		{name: "normalized true peak", mutate: func(q *DerivativeQC) { q.Loudness.TruePeak.DBTP = -0.1 }, want: "true peak"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := base
			test.mutate(&got)
			if err := ValidateDerivativeQC(got, 30_000, 2, true, -23); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyDerivativeKeyframesAllowsBoundedContainerRounding(t *testing.T) {
	probe := derivativeProbeExecutable(t, `{"streams":[{"start_time":"0.800801"}],"frames":[{"best_effort_timestamp_time":"0.800801"},{"best_effort_timestamp_time":"1.801802"},{"best_effort_timestamp_time":"3.169837"}]}`)
	first, maximum, terminal, err := verifyDerivativeKeyframes(
		context.Background(), probe, "candidate.mp4", 3_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != 0 || maximum != 1_368 || terminal != 631 {
		t.Fatalf("keyframe measurements = %d, %d, %d", first, maximum, terminal)
	}
}

func TestVerifyDerivativeKeyframesRejectsKeyframeBeyondTerminalSlack(t *testing.T) {
	probe := derivativeProbeExecutable(t, `{"streams":[{"start_time":"0.800000"}],"frames":[{"best_effort_timestamp_time":"0.800000"},{"best_effort_timestamp_time":"4.000000"}]}`)
	_, _, _, err := verifyDerivativeKeyframes(
		context.Background(), probe, "candidate.mp4", 2_600,
	)
	if err == nil || !strings.Contains(err.Error(), "beyond output duration") {
		t.Fatalf("verification error = %v, want beyond output duration", err)
	}
}

func derivativeProbeExecutable(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+document+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
