package mediatools_test

import (
	"math"
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
)

func validPersistedConditioningMeasurement() mediatools.ConditioningMeasurement {
	available := func(ms int64) mediatools.OptionalMilliseconds {
		return mediatools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	return mediatools.ConditioningMeasurement{
		ContainerDurationMs: 30_000,
		Streams: []mediatools.ConditioningStream{
			{Kind: mediatools.StreamVideo, Index: 0, Start: available(0), Duration: available(30_000),
				Cadence: &mediatools.Rational{Numerator: 30_000, Denominator: 1001}},
			{Kind: mediatools.StreamAudio, Index: 1, Start: available(120), Duration: available(29_880)},
		},
		AVSkew: mediatools.ConditioningSkew{Start: available(120), End: available(0)},
		Loudness: mediatools.ConditioningLoudness{
			IntegratedLUFS: -23.1, Available: true,
			TruePeak: mediatools.ConditioningTruePeak{State: mediatools.TruePeakFinite, DBTP: -2.1},
		},
		Quality: mediatools.MediaQuality{
			EvidenceVersion: mediatools.MediaQualityEvidenceV1,
			Provenance:      mediatools.MediaQualityProvenanceFFmpegDetectors,
			DurationMs:      30_000,
			Black:           []mediatools.Interval{{StartMs: 1_000, EndMs: 2_000}, {StartMs: 4_000, EndMs: 5_000}},
		},
		Cuts: []mediatools.ConditioningCutMeasurement{{Intended: mediatools.Interval{StartMs: 1_000, EndMs: 31_000}, Streams: []mediatools.ConditioningCutStream{
			{Kind: mediatools.StreamVideo, Index: 0, StartError: available(3), EndError: available(-4)},
			{Kind: mediatools.StreamAudio, Index: 1, StartError: available(5), EndError: available(-2)},
		}}},
	}
}

func TestValidateConditioningEvidenceRejectsNonCanonicalPersistedFacts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*mediatools.ConditioningMeasurement)
	}{
		{name: "stream index reused across kinds", mutate: func(m *mediatools.ConditioningMeasurement) { m.Streams[1].Index = 0; m.Cuts[0].Streams[1].Index = 0 }},
		{name: "second audio stream", mutate: func(m *mediatools.ConditioningMeasurement) { m.Streams = append(m.Streams, m.Streams[1]) }},
		{name: "negative stream ends before artifact", mutate: func(m *mediatools.ConditioningMeasurement) {
			m.Streams[0].Start.Milliseconds = -30_001
			m.AVSkew.Start.Milliseconds = 30_121
			m.AVSkew.End.Milliseconds = 30_001
		}},
		{name: "stream end arithmetic overflows", mutate: func(m *mediatools.ConditioningMeasurement) { m.Streams[0].Start.Milliseconds = int64(^uint64(0) >> 1) }},
		{name: "quality duration differs", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.DurationMs-- }},
		{name: "quality evidence version missing", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.EvidenceVersion = 0 }},
		{name: "quality provenance unknown", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.Provenance = "invented" }},
		{name: "detector intervals unsorted", mutate: func(m *mediatools.ConditioningMeasurement) {
			m.Quality.Black[0], m.Quality.Black[1] = m.Quality.Black[1], m.Quality.Black[0]
		}},
		{name: "detector intervals overlap", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.Black[1].StartMs = 1_500 }},
		{name: "detector intervals touch instead of being normalized", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.Black[1].StartMs = 2_000 }},
		{name: "detector interval exceeds duration", mutate: func(m *mediatools.ConditioningMeasurement) { m.Quality.Black[1].EndMs = 30_001 }},
		{name: "cut identity differs", mutate: func(m *mediatools.ConditioningMeasurement) { m.Cuts[0].Streams[1].Index = 7 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			measurement := validPersistedConditioningMeasurement()
			tc.mutate(&measurement)
			if err := mediatools.ValidateConditioningEvidence(measurement, mediatools.ConditioningBeforeRewrite); err == nil {
				t.Fatal("invalid persisted conditioning evidence was accepted")
			}
		})
	}
}

func TestValidateConditioningCutEvidenceFailsClosedWithoutMeasuredEdges(t *testing.T) {
	m := validPersistedConditioningMeasurement()
	intended := mediatools.Interval{StartMs: 1_000, EndMs: 31_000}
	if err := mediatools.ValidateConditioningCutEvidence(m, intended); err != nil {
		t.Fatal(err)
	}
	m.Cuts[0].Streams[0].EndError.Available = false
	if err := mediatools.ValidateConditioningCutEvidence(m, intended); err == nil {
		t.Fatal("missing measured child edge was accepted")
	}
}

func TestValidateConditioningCutIdentityFailsClosed(t *testing.T) {
	base := validPersistedConditioningMeasurement()
	want := mediatools.Interval{StartMs: 1_000, EndMs: 31_000}
	for _, tc := range []struct {
		name   string
		mutate func(*mediatools.ConditioningMeasurement)
	}{
		{name: "missing", mutate: func(m *mediatools.ConditioningMeasurement) { m.Cuts[0].Intended = mediatools.Interval{} }},
		{name: "different start", mutate: func(m *mediatools.ConditioningMeasurement) { m.Cuts[0].Intended.StartMs++ }},
		{name: "different end", mutate: func(m *mediatools.ConditioningMeasurement) { m.Cuts[0].Intended.EndMs++ }},
		{name: "malformed", mutate: func(m *mediatools.ConditioningMeasurement) {
			m.Cuts[0].Intended = mediatools.Interval{StartMs: 31_000, EndMs: 1_000}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			tc.mutate(&m)
			if err := mediatools.ValidateConditioningCutEvidence(m, want); err == nil {
				t.Fatal("invalid persisted interval identity was accepted")
			}
		})
	}
	reordered := []mediatools.ConditioningCutMeasurement{{Intended: mediatools.Interval{StartMs: 2_000, EndMs: 3_000}}, {Intended: mediatools.Interval{StartMs: 1_000, EndMs: 2_000}}}
	if err := mediatools.ValidateConditioningCutIdentities(reordered, []mediatools.Interval{{StartMs: 2_000, EndMs: 3_000}, {StartMs: 1_000, EndMs: 2_000}}); err == nil {
		t.Fatal("reordered persisted interval identities were accepted")
	}
}

func TestValidateConditioningPairRequiresExactStreamIdentityEquality(t *testing.T) {
	before := validPersistedConditioningMeasurement()
	after := validPersistedConditioningMeasurement()
	for i := range after.Cuts[0].Streams {
		after.Cuts[0].Streams[i].StartError = mediatools.OptionalMilliseconds{}
		after.Cuts[0].Streams[i].EndError = mediatools.OptionalMilliseconds{}
	}
	if err := mediatools.ValidateConditioningPair(before, after); err != nil {
		t.Fatalf("valid pair: %v", err)
	}
	after.Streams[1].Index = 2
	after.Cuts[0].Streams[1].Index = 2
	if err := mediatools.ValidateConditioningPair(before, after); err == nil {
		t.Fatal("before/after stream identity drift was accepted")
	}
	after = validPersistedConditioningMeasurement()
	after.Cuts[0].Intended.EndMs++
	if err := mediatools.ValidateConditioningPair(before, after); err == nil {
		t.Fatal("before/after cut interval drift was accepted")
	}
}

func TestValidateConditioningEvidenceAcceptsAvailableNegativeStreamStarts(t *testing.T) {
	measurement := validPersistedConditioningMeasurement()
	measurement.Streams[0].Start.Milliseconds = -100
	measurement.Streams[0].Duration.Milliseconds = 30_100
	measurement.Streams[1].Start.Milliseconds = -40
	measurement.Streams[1].Duration.Milliseconds = 30_040
	measurement.AVSkew.Start.Milliseconds = 60
	measurement.AVSkew.End.Milliseconds = 0

	if err := mediatools.ValidateConditioningEvidence(measurement, mediatools.ConditioningBeforeRewrite); err != nil {
		t.Fatalf("available negative presentation starts were rejected: %v", err)
	}
}

func TestDeriveConditioningParentEdgesUsesValidatedStreamIdentityAndCheckedArithmetic(t *testing.T) {
	before := validPersistedConditioningMeasurement()
	after := validPersistedConditioningMeasurement()
	for i := range after.Cuts[0].Streams {
		after.Cuts[0].Streams[i].StartError = mediatools.OptionalMilliseconds{}
		after.Cuts[0].Streams[i].EndError = mediatools.OptionalMilliseconds{}
	}
	after.Streams[0].Start.Milliseconds = 5
	after.Streams[0].Duration.Milliseconds = 29_995
	after.Streams[1].Start.Milliseconds = 125
	after.Streams[1].Duration.Milliseconds = 29_875

	got, err := mediatools.DeriveConditioningParentEdges(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got.Intended != before.Cuts[0].Intended {
		t.Fatalf("derived interval identity = %+v, want %+v", got.Intended, before.Cuts[0].Intended)
	}
	want := []mediatools.ConditioningCutStream{
		{Kind: mediatools.StreamVideo, Index: 0,
			StartError: mediatools.OptionalMilliseconds{Milliseconds: 8, Available: true},
			EndError:   mediatools.OptionalMilliseconds{Milliseconds: -4, Available: true}},
		{Kind: mediatools.StreamAudio, Index: 1,
			StartError: mediatools.OptionalMilliseconds{Milliseconds: 10, Available: true},
			EndError:   mediatools.OptionalMilliseconds{Milliseconds: -2, Available: true}},
	}
	if len(got.Streams) != len(want) {
		t.Fatalf("derived streams = %+v", got.Streams)
	}
	for i := range want {
		if got.Streams[i] != want[i] {
			t.Fatalf("derived stream %d = %+v, want %+v", i, got.Streams[i], want[i])
		}
	}
}

func TestDeriveConditioningParentEdgesRejectsUnavailableAndOverflowingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*mediatools.ConditioningMeasurement, *mediatools.ConditioningMeasurement)
	}{
		{name: "missing direct parent edge", mutate: func(before, _ *mediatools.ConditioningMeasurement) {
			before.Cuts[0].Streams[0].StartError = mediatools.OptionalMilliseconds{}
		}},
		{name: "derived start overflow", mutate: func(before, after *mediatools.ConditioningMeasurement) {
			before.Cuts[0].Streams[0].StartError.Milliseconds = math.MaxInt64
			after.Streams[0].Start.Milliseconds = 1
			after.Streams[0].Duration.Milliseconds = 29_999
			after.Streams[1].Start.Milliseconds = 121
			after.Streams[1].Duration.Milliseconds = 29_879
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := validPersistedConditioningMeasurement()
			after := validPersistedConditioningMeasurement()
			for i := range after.Cuts[0].Streams {
				after.Cuts[0].Streams[i].StartError = mediatools.OptionalMilliseconds{}
				after.Cuts[0].Streams[i].EndError = mediatools.OptionalMilliseconds{}
			}
			tc.mutate(&before, &after)
			if _, err := mediatools.DeriveConditioningParentEdges(before, after); err == nil {
				t.Fatal("invalid derived-edge evidence was accepted")
			}
		})
	}
}
