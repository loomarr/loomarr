package inventory_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// A measured source with no audio is a known answer (audioStreams empty), not an unmeasured one:
// the audio-track pick reads that coverage to decide there is nothing to choose.
func TestNewFactsMeasurement_CoversTheStreamsItObserved(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	video := inventory.Stream{Index: 0, Kind: inventory.StreamVideo, Codec: "h264"}
	audio := inventory.Stream{Index: 1, Kind: inventory.StreamAudio, Codec: "aac"}

	for _, tc := range []struct {
		name      string
		streams   []inventory.Stream
		wantAudio inventory.Coverage
	}{
		{"video and audio", []inventory.Stream{video, audio}, inventory.CoveragePresent},
		{"silent video", []inventory.Stream{video}, inventory.CoverageEmpty},
		{"no streams", nil, inventory.CoverageEmpty},
	} {
		facts := inventory.SourceFacts{Container: "matroska", Streams: tc.streams}
		got := inventory.NewFactsMeasurement("src-1", "stat:1:2", at, facts)

		if got.SourceID != "src-1" || got.Revision != "stat:1:2" {
			t.Errorf("%s: measurement is for %s@%s, want src-1@stat:1:2", tc.name, got.SourceID, got.Revision)
		}
		if got.Observation.SchemaVersion != 1 || !got.Observation.ObservedAt.Equal(at) {
			t.Errorf("%s: observation schema %d at %v", tc.name, got.Observation.SchemaVersion, got.Observation.ObservedAt)
		}
		if got.Observation.Coverage["streams"] != inventory.CoveragePresent {
			t.Errorf("%s: streams coverage = %q, want present", tc.name, got.Observation.Coverage["streams"])
		}
		if got.Observation.Coverage["audioStreams"] != tc.wantAudio {
			t.Errorf("%s: audioStreams coverage = %q, want %q", tc.name, got.Observation.Coverage["audioStreams"], tc.wantAudio)
		}
		if got.Observation.Facts.Container != "matroska" {
			t.Errorf("%s: facts not carried: %+v", tc.name, got.Observation.Facts)
		}
		if _, err := inventory.ValidateMeasurement(got); err != nil {
			t.Errorf("%s: the measurement does not validate: %v", tc.name, err)
		}
	}
}
