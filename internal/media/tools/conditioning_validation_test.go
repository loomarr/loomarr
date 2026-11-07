package tools_test

import (
	"math"
	"testing"

	"github.com/loomarr/loomarr/internal/media/tools"
)

func validPersistedConditioningMeasurement() tools.ConditioningMeasurement {
	available := func(ms int64) tools.OptionalMilliseconds {
		return tools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	return tools.ConditioningMeasurement{
		ContainerDurationMs: 30_000,
		Streams: []tools.ConditioningStream{
			{Kind: tools.StreamVideo, Index: 0, Start: available(0), Duration: available(30_000),
				Cadence: &tools.Rational{Numerator: 30_000, Denominator: 1001}},
			{Kind: tools.StreamAudio, Index: 1, Start: available(120), Duration: available(29_880)},
		},
		AVSkew: tools.ConditioningSkew{Start: available(120), End: available(0)},
		Loudness: tools.ConditioningLoudness{
			IntegratedLUFS: -23.1, Available: true,
			TruePeak: tools.ConditioningTruePeak{State: tools.TruePeakFinite, DBTP: -2.1},
		},
		Quality: tools.MediaQuality{
			EvidenceVersion: tools.MediaQualityEvidenceV1,
			Provenance:      tools.MediaQualityProvenanceFFmpegDetectors,
			DurationMs:      30_000,
			Black:           []tools.Interval{{StartMs: 1_000, EndMs: 2_000}, {StartMs: 4_000, EndMs: 5_000}},
		},
		Cuts: []tools.ConditioningCutMeasurement{{Intended: tools.Interval{StartMs: 1_000, EndMs: 31_000}, Streams: []tools.ConditioningCutStream{
			{Kind: tools.StreamVideo, Index: 0, StartError: available(3), EndError: available(-4)},
			{Kind: tools.StreamAudio, Index: 1, StartError: available(5), EndError: available(-2)},
		}}},
	}
}

func TestValidateConditioningEvidenceRejectsNonCanonicalPersistedFacts(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*tools.ConditioningMeasurement)
	}{
		{name: "stream index reused across kinds", mutate: func(m *tools.ConditioningMeasurement) { m.Streams[1].Index = 0; m.Cuts[0].Streams[1].Index = 0 }},
		{name: "second audio stream", mutate: func(m *tools.ConditioningMeasurement) { m.Streams = append(m.Streams, m.Streams[1]) }},
		{name: "negative stream ends before artifact", mutate: func(m *tools.ConditioningMeasurement) {
			m.Streams[0].Start.Milliseconds = -30_001
			m.AVSkew.Start.Milliseconds = 30_121
			m.AVSkew.End.Milliseconds = 30_001
		}},
		{name: "stream end arithmetic overflows", mutate: func(m *tools.ConditioningMeasurement) { m.Streams[0].Start.Milliseconds = int64(^uint64(0) >> 1) }},
		{name: "quality duration differs", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.DurationMs-- }},
		{name: "quality evidence version missing", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.EvidenceVersion = 0 }},
		{name: "quality provenance unknown", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.Provenance = "invented" }},
		{name: "detector intervals unsorted", mutate: func(m *tools.ConditioningMeasurement) {
			m.Quality.Black[0], m.Quality.Black[1] = m.Quality.Black[1], m.Quality.Black[0]
		}},
		{name: "detector intervals overlap", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.Black[1].StartMs = 1_500 }},
		{name: "detector intervals touch instead of being normalized", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.Black[1].StartMs = 2_000 }},
		{name: "detector interval exceeds duration", mutate: func(m *tools.ConditioningMeasurement) { m.Quality.Black[1].EndMs = 30_001 }},
		{name: "cut identity differs", mutate: func(m *tools.ConditioningMeasurement) { m.Cuts[0].Streams[1].Index = 7 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			measurement := validPersistedConditioningMeasurement()
			tc.mutate(&measurement)
			if err := tools.ValidateConditioningEvidence(measurement, tools.ConditioningBeforeRewrite); err == nil {
				t.Fatal("invalid persisted conditioning evidence was accepted")
			}
		})
	}
}

func TestValidateConditioningCutEvidenceFailsClosedWithoutMeasuredEdges(t *testing.T) {
	m := validPersistedConditioningMeasurement()
	intended := tools.Interval{StartMs: 1_000, EndMs: 31_000}
	if err := tools.ValidateConditioningCutEvidence(m, intended); err != nil {
		t.Fatal(err)
	}
	m.Cuts[0].Streams[0].EndError.Available = false
	if err := tools.ValidateConditioningCutEvidence(m, intended); err == nil {
		t.Fatal("missing measured child edge was accepted")
	}
}

func TestValidateConditioningCutIdentityFailsClosed(t *testing.T) {
	base := validPersistedConditioningMeasurement()
	want := tools.Interval{StartMs: 1_000, EndMs: 31_000}
	for _, tc := range []struct {
		name   string
		mutate func(*tools.ConditioningMeasurement)
	}{
		{name: "missing", mutate: func(m *tools.ConditioningMeasurement) { m.Cuts[0].Intended = tools.Interval{} }},
		{name: "different start", mutate: func(m *tools.ConditioningMeasurement) { m.Cuts[0].Intended.StartMs++ }},
		{name: "different end", mutate: func(m *tools.ConditioningMeasurement) { m.Cuts[0].Intended.EndMs++ }},
		{name: "malformed", mutate: func(m *tools.ConditioningMeasurement) {
			m.Cuts[0].Intended = tools.Interval{StartMs: 31_000, EndMs: 1_000}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			tc.mutate(&m)
			if err := tools.ValidateConditioningCutEvidence(m, want); err == nil {
				t.Fatal("invalid persisted interval identity was accepted")
			}
		})
	}
	reordered := []tools.ConditioningCutMeasurement{{Intended: tools.Interval{StartMs: 2_000, EndMs: 3_000}}, {Intended: tools.Interval{StartMs: 1_000, EndMs: 2_000}}}
	if err := tools.ValidateConditioningCutIdentities(reordered, []tools.Interval{{StartMs: 2_000, EndMs: 3_000}, {StartMs: 1_000, EndMs: 2_000}}); err == nil {
		t.Fatal("reordered persisted interval identities were accepted")
	}
}

func TestValidateConditioningPairRequiresExactStreamIdentityEquality(t *testing.T) {
	before := validPersistedConditioningMeasurement()
	after := validPersistedConditioningMeasurement()
	for i := range after.Cuts[0].Streams {
		after.Cuts[0].Streams[i].StartError = tools.OptionalMilliseconds{}
		after.Cuts[0].Streams[i].EndError = tools.OptionalMilliseconds{}
	}
	if err := tools.ValidateConditioningPair(before, after); err != nil {
		t.Fatalf("valid pair: %v", err)
	}
	after.Streams[1].Index = 2
	after.Cuts[0].Streams[1].Index = 2
	if err := tools.ValidateConditioningPair(before, after); err == nil {
		t.Fatal("before/after stream identity drift was accepted")
	}
	after = validPersistedConditioningMeasurement()
	after.Cuts[0].Intended.EndMs++
	if err := tools.ValidateConditioningPair(before, after); err == nil {
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

	if err := tools.ValidateConditioningEvidence(measurement, tools.ConditioningBeforeRewrite); err != nil {
		t.Fatalf("available negative presentation starts were rejected: %v", err)
	}
}

// reencodedChildMeasurement is the after-rewrite shape of #1540's 0-12000 ms cut, as the
// DefaultMezzanine transcode leaves it: ffprobe's format.duration (12.121 s) is measured from
// format.start_time (0.045 s, the AAC priming offset), so the audio stream ends at 12.166 s.
func reencodedChildMeasurement() tools.ConditioningMeasurement {
	available := func(ms int64) tools.OptionalMilliseconds {
		return tools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	m := validPersistedConditioningMeasurement()
	m.ContainerStart = available(45)
	m.ContainerDurationMs = 12_121
	m.Streams[0].Start, m.Streams[0].Duration = available(67), available(12_012)
	m.Streams[1].Start, m.Streams[1].Duration = available(45), available(12_121)
	m.AVSkew = tools.ConditioningSkew{Start: available(-22), End: available(87)}
	m.Quality.DurationMs = 12_121
	m.Quality.Black = nil
	m.Cuts[0].Intended = tools.Interval{StartMs: 0, EndMs: 12_000}
	for i := range m.Cuts[0].Streams {
		m.Cuts[0].Streams[i].StartError = tools.OptionalMilliseconds{}
		m.Cuts[0].Streams[i].EndError = tools.OptionalMilliseconds{}
	}
	return m
}

func TestValidateConditioningEvidenceBoundsStreamsByTheContainerTimeline(t *testing.T) {
	if err := tools.ValidateConditioningEvidence(reencodedChildMeasurement(), tools.ConditioningAfterRewrite); err != nil {
		t.Fatalf("re-encoded child whose container starts after zero was rejected: %v", err)
	}
	// #2032's un-levelled re-encode read video 67+12513 against container 45+12534: one instant,
	// rounded twice per side.
	rounded := reencodedChildMeasurement()
	rounded.Streams[1].Duration.Milliseconds += 2
	rounded.AVSkew.End.Milliseconds += 2
	if err := tools.ValidateConditioningEvidence(rounded, tools.ConditioningAfterRewrite); err != nil {
		t.Fatalf("stream end within millisecond rounding of the container end was rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*tools.ConditioningMeasurement)
	}{
		{name: "stream ends past container start plus duration and its rounding", mutate: func(m *tools.ConditioningMeasurement) {
			m.Streams[1].Duration.Milliseconds += 3
			m.AVSkew.End.Milliseconds += 3
		}},
		{name: "container starts after a stream starts", mutate: func(m *tools.ConditioningMeasurement) { m.ContainerStart.Milliseconds = 46 }},
		{name: "container start overflows its end", mutate: func(m *tools.ConditioningMeasurement) {
			m.ContainerStart.Milliseconds = math.MaxInt64
		}},
		{name: "unavailable container start carries a guessed value", mutate: func(m *tools.ConditioningMeasurement) {
			m.ContainerStart.Available = false
		}},
		{name: "legacy evidence without a container start keeps the zero-based bound", mutate: func(m *tools.ConditioningMeasurement) {
			m.ContainerStart = tools.OptionalMilliseconds{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := reencodedChildMeasurement()
			tc.mutate(&m)
			if err := tools.ValidateConditioningEvidence(m, tools.ConditioningAfterRewrite); err == nil {
				t.Fatal("stream timing outside the container timeline was accepted")
			}
		})
	}
}

func TestDeriveConditioningParentEdgesUsesValidatedStreamIdentityAndCheckedArithmetic(t *testing.T) {
	before := validPersistedConditioningMeasurement()
	after := validPersistedConditioningMeasurement()
	for i := range after.Cuts[0].Streams {
		after.Cuts[0].Streams[i].StartError = tools.OptionalMilliseconds{}
		after.Cuts[0].Streams[i].EndError = tools.OptionalMilliseconds{}
	}
	after.Streams[0].Start.Milliseconds = 5
	after.Streams[0].Duration.Milliseconds = 29_995
	after.Streams[1].Start.Milliseconds = 125
	after.Streams[1].Duration.Milliseconds = 29_875

	got, err := tools.DeriveConditioningParentEdges(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got.Intended != before.Cuts[0].Intended {
		t.Fatalf("derived interval identity = %+v, want %+v", got.Intended, before.Cuts[0].Intended)
	}
	want := []tools.ConditioningCutStream{
		{Kind: tools.StreamVideo, Index: 0,
			StartError: tools.OptionalMilliseconds{Milliseconds: 8, Available: true},
			EndError:   tools.OptionalMilliseconds{Milliseconds: -4, Available: true}},
		{Kind: tools.StreamAudio, Index: 1,
			StartError: tools.OptionalMilliseconds{Milliseconds: 10, Available: true},
			EndError:   tools.OptionalMilliseconds{Milliseconds: -2, Available: true}},
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
		mutate func(*tools.ConditioningMeasurement, *tools.ConditioningMeasurement)
	}{
		{name: "missing direct parent edge", mutate: func(before, _ *tools.ConditioningMeasurement) {
			before.Cuts[0].Streams[0].StartError = tools.OptionalMilliseconds{}
		}},
		{name: "derived start overflow", mutate: func(before, after *tools.ConditioningMeasurement) {
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
				after.Cuts[0].Streams[i].StartError = tools.OptionalMilliseconds{}
				after.Cuts[0].Streams[i].EndError = tools.OptionalMilliseconds{}
			}
			tc.mutate(&before, &after)
			if _, err := tools.DeriveConditioningParentEdges(before, after); err == nil {
				t.Fatal("invalid derived-edge evidence was accepted")
			}
		})
	}
}
