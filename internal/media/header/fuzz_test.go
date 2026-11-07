package header

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// fuzzSeeds are small real files (when ffmpeg is present) plus hand-built fragments, so mutation
// starts from shapes the parsers actually walk.
func fuzzSeeds(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("\x1A\x45\xDF\xA3\x84\x42\x82\x81m"))
	f.Add([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom\x00\x00\x00\x08moov"))
	f.Add([]byte("\x00\x00\x00\x01moov\xff\xff\xff\xff\xff\xff\xff\xff"))
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return
	}
	dir := f.TempDir()
	for _, c := range []struct{ name, codec string }{{"a.mkv", "libx264"}, {"b.mp4", "libx264"}, {"c.mkv", "libx265"}} {
		path := filepath.Join(dir, c.name)
		if exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=64x64:r=5:d=1",
			"-c:v", c.codec, "-preset", "ultrafast", path).Run() == nil {
			if b, err := readFile(path); err == nil {
				f.Add(b)
			}
		}
	}
}

// No input, however malformed, panics, loops or reads out of bounds.
func FuzzRead(f *testing.F) {
	fuzzSeeds(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		r := bytesReaderAt(b)
		h, ok := Read(r, int64(len(b)))
		if ok && (h.DurationMillis < 0 || h.Container == "") {
			t.Fatalf("accepted with nonsense facts: %+v", h)
		}
		_, _, _ = Keyframes(r, int64(len(b)))
	})
}

// The codec configuration parsers take bytes straight from the file.
func FuzzCodecConfig(f *testing.F) {
	f.Add([]byte{1, 0x64, 0, 0x28, 0xFF, 0xE1, 0, 4, 0x67, 0x64, 0, 0x28})
	f.Add([]byte{0x12, 0x10})
	f.Add([]byte{0x81, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, codec := range []string{"h264", "hevc", "av1", "vp8", "mpeg2video", "mpeg4"} {
			_, _ = videoConfig(codec, b)
		}
		_, _ = parseVPConfig(b)
		_, _ = matroskaVP9(b)
		_, _ = parseAudioSpecificConfig(b)
		_, _ = esdsDecoderInfo(b)
		_ = neroChapters(b, 1000)
		_, _, _ = sttsTotals(b)
	})
}
