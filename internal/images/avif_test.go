package images

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireFFmpeg skips when ffmpeg is absent.
//
// ⚠ A SKIP here is a real risk, not a formality: §10 shipped an ffmpeg helper with byte-identical
// duplicated code and *zero* coverage, discovered only when changing `-ar 16000` to `-ar 8000`
// failed nothing. CI installs ffmpeg precisely so these do not skip there — if this starts
// skipping in CI, that is a broken job, not an acceptable state.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
}

func TestFFmpegAVIFProducesARealAVIF(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()

	if !HasAVIFEncoder(ctx, "") {
		t.Skip("ffmpeg has no libaom-av1")
	}

	dst := filepath.Join(t.TempDir(), "out.avif")
	if err := FFmpegAVIF("")(ctx, Resize(testImage(1000, 1500), 500), dst); err != nil {
		t.Fatalf("FFmpegAVIF: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("wrote an empty file")
	}
	// Check the container rather than trusting the exit code: an AVIF is ISOBMFF, so bytes 4..8
	// are "ftyp" and the brand that follows must mention avif. This is the assertion that a
	// "did it error?" test cannot make.
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		t.Fatalf("output is not ISOBMFF (no ftyp box): % x", data[:min(16, len(data))])
	}
	if brand := string(data[8:12]); brand != "avif" && brand != "avis" {
		t.Errorf("ftyp brand = %q, want avif", brand)
	}
}

// A missing encoder must surface as an error the job can count, never as a silently absent
// rendition — the whole reason the AVIF path is allowed to be eventually-consistent is that its
// failures are visible.
func TestFFmpegAVIFReportsAMissingBinary(t *testing.T) {
	err := FFmpegAVIF("/nonexistent/ffmpeg")(context.Background(), testImage(32, 32),
		filepath.Join(t.TempDir(), "x.avif"))
	if err == nil {
		t.Fatal("a missing ffmpeg produced no error; the job would record a phantom success")
	}
}

func TestHasAVIFEncoderIsFalseForAMissingBinary(t *testing.T) {
	if HasAVIFEncoder(context.Background(), "/nonexistent/ffmpeg") {
		t.Fatal("HasAVIFEncoder said yes for a binary that does not exist")
	}
}

// The real cost of the format, on this machine, so the "AVIF is a job not a request" decision in
// §22 rests on a number rather than on a recollection.
func BenchmarkAVIFEncode(b *testing.B) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		b.Skip("ffmpeg not on PATH")
	}
	ctx := context.Background()
	enc := FFmpegAVIF("")
	img := Resize(testImage(2000, 3000), 500)
	dir := b.TempDir()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		if err := enc(ctx, img, filepath.Join(dir, "b.avif")); err != nil {
			b.Fatal(err)
		}
	}
}
