package mediaheader

import (
	"os"
	"testing"

	"github.com/loomarr/loomarr/internal/inventory"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// avReduce spells frame rates exactly as ffmpeg's av_reduce does.
func TestAvReduceMatchesFFmpeg(t *testing.T) {
	for _, c := range []struct {
		num, den, limit int64
		want            string
	}{
		{1_000_000_000, 40_000_000, 30000, "25/1"},
		{1_000_000_000, 41_708_333, 30000, "24000/1001"},
		{1_000_000_000, 33_366_667, 30000, "30000/1001"},
		// ffmpeg caps Matroska rates at 30000: ffprobe really reports 59.94 as 19001/317.
		{1_000_000_000, 16_683_333, 30000, "19001/317"},
		{12800 * 75, 75 * 512, 1<<31 - 1, "25/1"},
	} {
		if got := rational(avReduce(c.num, c.den, c.limit)); got != c.want {
			t.Errorf("avReduce(%d, %d) = %s, want %s", c.num, c.den, got, c.want)
		}
	}
}

func TestAudioSpecificConfig(t *testing.T) {
	for _, c := range []struct {
		name     string
		asc      []byte
		profile  string
		rate, ch int
	}{
		{"LC 48 kHz stereo", []byte{0x11, 0x90}, "LC", 48000, 2},
		{"LC 44.1 kHz 5.1", []byte{0x12, 0x30}, "LC", 44100, 6},
		{"HE-AAC explicit, 24 kHz core → 48 kHz", []byte{0x2B, 0x09, 0x88, 0x00}, "HE-AAC", 48000, 1},
	} {
		got, ok := parseAudioSpecificConfig(c.asc)
		if !ok || got.profile != c.profile || got.sampleRate != c.rate || got.channels != c.ch {
			t.Errorf("%s: %+v ok=%v", c.name, got, ok)
		}
	}
}

func TestPixelFormatNames(t *testing.T) {
	for _, c := range []struct {
		chroma, depth int
		jpeg          bool
		want          string
	}{
		{1, 8, false, "yuv420p"}, {1, 10, false, "yuv420p10le"}, {2, 10, false, "yuv422p10le"},
		{3, 12, false, "yuv444p12le"}, {0, 8, false, "gray"}, {1, 8, true, "yuvj420p"}, {1, 11, false, ""},
	} {
		if got := pixelFormat(c.chroma, c.depth, c.jpeg); got != c.want {
			t.Errorf("pixelFormat(%d, %d, %v) = %q, want %q", c.chroma, c.depth, c.jpeg, got, c.want)
		}
	}
}

// A 10-bit stream whose colour no one signalled might carry HDR only a decoder sees: decline.
func TestTenBitWithoutColourDeclines(t *testing.T) {
	bits := videoBits{chroma: 1, depth: 10}
	stream := inventory.Stream{Codec: "av1"}
	if describeVideo(&stream, bits, colourInfo{}) {
		t.Fatal("described 10-bit video with unknown colour")
	}
	if !describeVideo(&stream, bits, colourInfo{primaries: 9, transfer: 16, matrix: 9}) || !stream.HDR {
		t.Fatalf("container colour must decide: %+v", stream)
	}
}
