package filler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The on-file loudness pass (§10 V42). ⚠ This is the ONE function in the package that modifies
// the operator's media file, and the original is unrecoverable — so these assert the SAFETY
// properties, not merely that ffmpeg ran.
//
// Skipped without ffmpeg/ffprobe, matching `languagesilence_test.go` and `ffmpegexec_test.go`.

func needsFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
}

// quietClip writes a 2s clip whose audio sits well below the target, so normalising it is a
// measurable change rather than a no-op.
func quietClip(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:d=2",
		"-af", "volume=-20dB",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest",
		"-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build clip: %v: %s", err, out)
	}
	return path
}

// measureLUFS reports a file's integrated loudness via ffmpeg's ebur128 filter.
func measureLUFS(t *testing.T, path string) float64 {
	t.Helper()
	out, _ := exec.Command("ffmpeg", "-nostdin", "-i", path,
		"-af", "ebur128=framelog=quiet", "-f", "null", "-").CombinedOutput()
	idx := strings.Index(string(out), "Integrated loudness")
	if idx < 0 {
		t.Fatalf("no integrated loudness in ffmpeg output for %s", path)
	}
	for _, line := range strings.Split(string(out)[idx:], "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "I:" && f[2] == "LUFS" {
			v, err := strconv.ParseFloat(f[1], 64)
			if err != nil {
				t.Fatalf("parse LUFS %q: %v", f[1], err)
			}
			return v
		}
	}
	t.Fatalf("no integrated loudness value for %s", path)
	return 0
}

func TestNormalizeInPlace_BringsAQuietClipToTarget(t *testing.T) {
	needsFFmpeg(t)
	dir := t.TempDir()
	path := quietClip(t, dir, "quiet.mp4")

	before := measureLUFS(t, path)
	if before > -30 {
		t.Fatalf("fixture is not quiet enough to be a real test: %.1f LUFS", before)
	}

	did, err := NormalizeInPlace(context.Background(), "", path, -23)
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Fatal("reported a skip on a clip that had never been normalised")
	}

	after := measureLUFS(t, path)
	// loudnorm lands NEAR the target, not exactly on it — V40 measured −26.8 → −23.4. 1.5 LU is
	// wider than its own error and far narrower than the ~11 dB spread this addresses.
	if after < -24.5 || after > -21.5 {
		t.Errorf("after normalising: %.1f LUFS, want about −23 (was %.1f)", after, before)
	}
}

// ⚠ THE test this feature is broken without. A normalised file is indistinguishable from any
// other file by inspection, so an unmarked pass would re-normalise on every scan and walk the
// loudness DOWN run after run — quieter each time, which is the opposite of the point.
func TestNormalizeInPlace_SkipsAnAlreadyNormalisedClip(t *testing.T) {
	needsFFmpeg(t)
	dir := t.TempDir()
	path := quietClip(t, dir, "once.mp4")

	if _, err := NormalizeInPlace(context.Background(), "", path, -23); err != nil {
		t.Fatal(err)
	}
	afterFirst := measureLUFS(t, path)

	did, err := NormalizeInPlace(context.Background(), "", path, -23)
	if err != nil {
		t.Fatal(err)
	}
	if did {
		t.Error("normalised a second time — the sidecar marker is not being honoured")
	}
	if got := measureLUFS(t, path); got < afterFirst-0.5 || got > afterFirst+0.5 {
		t.Errorf("loudness moved on the second pass: %.1f → %.1f", afterFirst, got)
	}
}

// ⚠ Lowering the target must re-normalise ONCE. The marker is compared against the CURRENT
// target, not merely "is it set" — otherwise changing `filler.target_lufs` would silently do
// nothing for every clip already processed.
func TestNormalizeInPlace_ReNormalisesWhenTheTargetChanges(t *testing.T) {
	needsFFmpeg(t)
	dir := t.TempDir()
	path := quietClip(t, dir, "retarget.mp4")

	if _, err := NormalizeInPlace(context.Background(), "", path, -23); err != nil {
		t.Fatal(err)
	}
	did, err := NormalizeInPlace(context.Background(), "", path, -16)
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Fatal("a changed target did not re-normalise — the marker is being read as a boolean")
	}
	if got := measureLUFS(t, path); got < -18 || got > -14 {
		t.Errorf("after re-targeting to −16: %.1f LUFS", got)
	}
}

func TestNormalizeInPlace_RecordsTheMarkerInTheSidecar(t *testing.T) {
	needsFFmpeg(t)
	dir := t.TempDir()
	path := quietClip(t, dir, "marked.mp4")

	if _, err := NormalizeInPlace(context.Background(), "", path, -23); err != nil {
		t.Fatal(err)
	}
	tags, ok := ReadSidecarTags(path)
	if !ok {
		t.Fatal("no sidecar written — a re-scan would normalise this clip again")
	}
	if tags.NormalizedLUFS != -23 {
		t.Errorf("marker = %v, want -23", tags.NormalizedLUFS)
	}
}

// ⚠ A failed encode must leave the ORIGINAL in place. The whole objection to on-file
// normalisation is that the original is unrecoverable; the least this can do is not destroy it
// for a clip it could not process.
func TestNormalizeInPlace_LeavesTheOriginalOnFailure(t *testing.T) {
	needsFFmpeg(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "notmedia.mp4")
	if err := os.WriteFile(path, []byte("this is not a media file"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NormalizeInPlace(context.Background(), "", path, -23); err == nil {
		t.Fatal("expected an error on an unreadable file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the original was destroyed by a failed normalise: %v", err)
	}
	if string(got) != "this is not a media file" {
		t.Errorf("the original was modified by a failed normalise: %q", got)
	}
	// The temp file must not be left behind either — the clip folder is scanned, and a stray
	// `.loudnorm.tmp.mp4` would be catalogued as a clip.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "loudnorm.tmp") {
			t.Errorf("left a temp file behind: %s", e.Name())
		}
	}
}
