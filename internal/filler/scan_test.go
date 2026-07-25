package filler_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// fakeProbe reports a fixed duration, so the scanner is testable without ffmpeg.
func fakeProbe(ms int64) filler.Prober {
	return func(context.Context, string) (int64, error) { return ms, nil }
}

func writeFile(t *testing.T, dir, rel string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The identity is the path RELATIVE to FILLER_DIR, with forward slashes. Relative because
// FILLER_DIR differs between a host and a container and absolute ids would break on a remount;
// forward slashes so a Windows-authored catalog and a Linux one agree (the pod seed hashes ids,
// so a differing separator would silently change pod contents rather than failing).
func TestScanDir_IdentityIsTheRelativePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "1994/toys-transformers.mp4")
	writeFile(t, dir, "station-id.mp4")

	clips, _, err := filler.ScanDir(context.Background(), dir, fakeProbe(30000))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range clips {
		got[c.Path] = true
	}
	for _, want := range []string{"1994/toys-transformers.mp4", "station-id.mp4"} {
		if !got[want] {
			t.Errorf("missing %q; got %v", want, got)
		}
	}
	for p := range got {
		if filepath.IsAbs(p) {
			t.Errorf("id %q is absolute — it would break the first time the mount moves", p)
		}
		if strings.Contains(p, `\`) {
			t.Errorf("id %q has a backslash — ids must be platform-independent", p)
		}
	}
}

// A drop-folder accumulates junk. Non-media files must be ignored silently rather than probed.
func TestScanDir_IgnoresNonMediaFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ad.mp4")
	for _, junk := range []string{".DS_Store", "notes.txt", "poster.jpg", "half.mp4.part"} {
		writeFile(t, dir, junk)
	}

	clips, _, err := filler.ScanDir(context.Background(), dir, fakeProbe(15000))
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 || clips[0].Path != "ad.mp4" {
		t.Errorf("want only ad.mp4, got %+v", clips)
	}
}

// One bad file must not cost the channel all of its commercials. A half-copied download or a
// file with no video stream is normal in an operator-managed folder.
func TestScanDir_SkipsUnprobeableFilesRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.mp4")
	writeFile(t, dir, "broken.mp4")

	probe := func(_ context.Context, path string) (int64, error) {
		if strings.Contains(path, "broken") {
			return 0, os.ErrInvalid
		}
		return 30000, nil
	}
	clips, skipped, err := filler.ScanDir(context.Background(), dir, probe)
	if err != nil {
		t.Fatalf("one bad file failed the whole scan: %v", err)
	}
	if len(clips) != 1 || clips[0].Path != "good.mp4" {
		t.Errorf("want the good clip only, got %+v", clips)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 so the caller can log it", skipped)
	}
}

// A zero duration is not usable: the pod assembler fills a break to a target length, so a
// clip that occupies no time would either be skipped downstream or break the arithmetic.
func TestScanDir_RejectsZeroDuration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.mp4")

	clips, skipped, err := filler.ScanDir(context.Background(), dir, fakeProbe(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 0 {
		t.Errorf("a zero-duration clip was admitted: %+v", clips)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

// An unset FILLER_DIR is "filler not configured" — an empty catalog, not an error. A missing
// one IS an error: it is almost always a misconfigured path, and silently returning nothing
// presents as "filler mysteriously does nothing".
func TestScanDir_UnsetIsEmptyButMissingIsAnError(t *testing.T) {
	clips, _, err := filler.ScanDir(context.Background(), "", fakeProbe(1000))
	if err != nil || len(clips) != 0 {
		t.Errorf("unset dir: got %d clips, err %v — want empty and no error", len(clips), err)
	}

	if _, _, err := filler.ScanDir(context.Background(), "/definitely/not/here", fakeProbe(1000)); err == nil {
		t.Error("a missing FILLER_DIR must be reported, not silently empty")
	}
}

// Filename tagging is the cheapest tier (§10). Without it a clip lands as a generic
// interstitial the pod assembler can never place, so filler would silently never build.
func TestScanDir_InfersKindAndEraFromTheFilename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "1994-toys-ad.mp4")
	writeFile(t, dir, "bumper-back-soon.mp4")

	clips, _, err := filler.ScanDir(context.Background(), dir, fakeProbe(20000))
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]filler.RawClip{}
	for _, c := range clips {
		byPath[c.Path] = c
	}
	if got := byPath["1994-toys-ad.mp4"]; got.Era != 1994 {
		t.Errorf("era not inferred from the filename: %+v", got)
	}
	if got := byPath["bumper-back-soon.mp4"]; got.Kind != filler.Bumper {
		t.Errorf("kind not inferred from the filename: %+v", got)
	}
}

// --- ClipPath: the containment boundary ---

// ⚠ A clip id reaches ClipPath from the DATABASE and from API callers, so a crafted `../`
// would otherwise make playout stream an arbitrary file off the host.
func TestClipPath_RefusesToEscapeTheFillerDir(t *testing.T) {
	dir := t.TempDir()

	for name, id := range map[string]string{
		"parent":          "../secrets.env",
		"nested parent":   "ads/../../../../etc/passwd",
		"absolute":        "/etc/passwd",
		"absolute nested": "/var/lib/loomarr/loomarr.db",
		"bare parent":     "..",
		"empty":           "",
	} {
		if got, err := filler.ClipPath(dir, id); err == nil {
			t.Errorf("%s: %q resolved to %q instead of being refused", name, id, got)
		}
	}
}

// …while a legitimate id resolves, including one in a subfolder.
func TestClipPath_ResolvesLegitimateIDs(t *testing.T) {
	dir := t.TempDir()

	for _, id := range []string{"ad.mp4", "1994/toys.mp4", "a/b/c/deep.mp4"} {
		got, err := filler.ClipPath(dir, id)
		if err != nil {
			t.Errorf("%q was refused: %v", id, err)
			continue
		}
		if !strings.HasPrefix(got, dir) {
			t.Errorf("%q resolved outside the filler dir: %q", id, got)
		}
		if !strings.HasSuffix(got, filepath.FromSlash(id)) {
			t.Errorf("%q resolved to %q, which is not that clip", id, got)
		}
	}
}

// An id that merely LOOKS like traversal after cleaning is fine — "a/../b.mp4" is "b.mp4",
// still inside. Rejecting on the raw string would refuse it; checking the cleaned path does not.
func TestClipPath_AllowsInternalDotDotThatStaysInside(t *testing.T) {
	dir := t.TempDir()
	got, err := filler.ClipPath(dir, "ads/../bumpers/x.mp4")
	if err != nil {
		t.Fatalf("a path that stays inside was refused: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("bumpers", "x.mp4")) {
		t.Errorf("resolved to %q", got)
	}
}

// No FILLER_DIR means no clip can be resolved — never a bare relative path handed to ffmpeg,
// which would resolve against the process's working directory.
func TestClipPath_RefusesWhenUnconfigured(t *testing.T) {
	if _, err := filler.ClipPath("", "ad.mp4"); err == nil {
		t.Error("resolved a clip with no FILLER_DIR configured")
	}
}

// --- DirSource: the scan is the source of truth, Tunarr only annotates ---

type stubTunarr struct {
	ids map[string]string
	err error
}

func (s stubTunarr) EnsureLocalSource(context.Context, string) error { return nil }
func (s stubTunarr) LocalClipIDsByName(context.Context) (map[string]string, error) {
	return s.ids, s.err
}

// THE POINT OF THE CHANGE: no Tunarr still yields a full catalog. Previously clips were
// discovered BY asking Tunarr, so an internal-playout install with no Tunarr had no commercials.
func TestDirSource_NoTunarrStillProducesAFullCatalog(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ad1.mp4")
	writeFile(t, dir, "ad2.mp4")

	src := filler.DirSource{Dir: func() string { return dir }, Probe: fakeProbe(30000)}
	clips, err := src.ListLocalClips(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 2 {
		t.Fatalf("want 2 clips with no Tunarr configured, got %d", len(clips))
	}
	for _, c := range clips {
		if c.Path == "" {
			t.Error("a clip has no identity")
		}
		if c.TunarrProgramID != "" {
			t.Errorf("clip %q has a Tunarr id with no Tunarr configured", c.Path)
		}
	}
}

// When Tunarr IS configured its uuids are attached, so Tunarr-backed channels still build
// filler-lists from the same catalog.
func TestDirSource_AnnotatesWithTunarrIDsWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ad1.mp4")

	src := filler.DirSource{
		Dir: func() string { return dir }, Probe: fakeProbe(30000),
		Tunarr: stubTunarr{ids: map[string]string{"ad1": "tun-123"}},
	}
	clips, err := src.ListLocalClips(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 || clips[0].TunarrProgramID != "tun-123" {
		t.Errorf("Tunarr id not attached: %+v", clips)
	}
	if clips[0].Path != "ad1.mp4" {
		t.Errorf("identity should still be the path, got %q", clips[0].Path)
	}
}

// A Tunarr failure must not fail the scan: the catalog is already complete and internal
// playout needs nothing more. Only Tunarr-backed channels lose anything, and only until the
// next sync.
func TestDirSource_TunarrFailureDoesNotFailTheScan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ad1.mp4")

	src := filler.DirSource{
		Dir: func() string { return dir }, Probe: fakeProbe(30000),
		Tunarr: stubTunarr{err: os.ErrDeadlineExceeded},
	}
	clips, err := src.ListLocalClips(context.Background())
	if err != nil {
		t.Fatalf("a Tunarr outage failed the whole scan: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("want the clip anyway, got %d", len(clips))
	}
	if clips[0].TunarrProgramID != "" {
		t.Error("a uuid appeared despite the Tunarr call failing")
	}
}

// The prober takes the FFMPEG path and derives ffprobe from it — the two ship together, so an
// operator who moved one moved both, and a second setting would have only one correct value.
//
// This test exists because the original API took a bare `bin` and the call site handed it
// `playout.ffmpeg_path`. That ran `ffmpeg -show_entries …`, which is not valid, so every probe
// failed — and since ScanDir skips unprobeable files by design, the catalog came back silently
// empty with no error anywhere. Only running it end to end surfaced that.
func TestFFprobeDurationNextTo_DerivesFfprobeFromTheFfmpegPath(t *testing.T) {
	// A path that does not exist, so the error text tells us which binary it TRIED.
	probe := filler.FFprobeDurationNextTo("/opt/custom/bin/ffmpeg")
	_, err := probe(context.Background(), "whatever.mp4")
	if err == nil {
		t.Fatal("expected a failure for a nonexistent binary")
	}
	if strings.Contains(err.Error(), "ffmpeg") && !strings.Contains(err.Error(), "ffprobe") {
		t.Errorf("the prober invoked ffmpeg rather than ffprobe: %v", err)
	}
}
