package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
)

type transcodeStore struct {
	oldHash string
	clip    StoreClip
	err     error
}

func (s *transcodeStore) ReplaceClipIdentity(_ context.Context, oldHash string, c StoreClip) error {
	s.oldHash, s.clip = oldHash, c
	err := s.err
	s.err = nil
	return err
}

func TestTranscodeStage_RecoversAPublishedPairAfterStoreFailure(t *testing.T) {
	dir := t.TempDir()
	oldHash := writeContentAddressedClip(t, dir, []byte("original"), ".mkv")
	oldRel := filepath.ToSlash(ClipRelPath(oldHash, ".mkv"))
	stored := &transcodeStore{err: errors.New("commit failed")}
	probe := func(context.Context, string) (Probed, error) {
		return Probed{DurationMs: 30_000, Height: 480}, nil
	}
	stage := NewTranscodeStage(stored, probe, dir, mediatools.DefaultMezzanine(), nil, nil, time.Now)
	stage.transcode = func(_ context.Context, req mediatools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
		return MediaQuality{}, os.WriteFile(req.Out, []byte("replacement"), 0o644)
	}
	clip := StoreClip{Clip: Clip{Hash: oldHash, Path: oldRel, Name: "Named advert", Kind: Commercial}}

	if _, err := stage.Run(context.Background(), clip); err == nil {
		t.Fatal("first run succeeded despite the simulated store failure")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldRel))); err != nil {
		t.Fatalf("store failure removed the original: %v", err)
	}
	// The verified replacement is deliberately retained. The retry recognizes its marker and
	// finishes the database re-key instead of rejecting the clip as a collision forever.
	out, err := stage.Run(context.Background(), clip)
	if err != nil {
		t.Fatal(err)
	}
	if out.Clip.Hash == oldHash || stored.clip.Hash != out.Clip.Hash {
		t.Errorf("retry did not complete replacement: %+v / %+v", out.Clip, stored.clip)
	}
}

func TestTranscodeStage_RekeysBytesAndPreservesHumanMetadata(t *testing.T) {
	dir := t.TempDir()
	oldBytes := []byte("the original container bytes")
	newBytes := []byte("the verified mezzanine bytes are different")
	oldHash := writeContentAddressedClip(t, dir, oldBytes, ".mkv")
	oldRel := filepath.ToSlash(ClipRelPath(oldHash, ".mkv"))
	oldFull := filepath.Join(dir, filepath.FromSlash(oldRel))

	stored := &transcodeStore{}
	probe := func(_ context.Context, path string) (Probed, error) {
		return Probed{DurationMs: 30_000, Height: 480}, nil
	}
	stage := NewTranscodeStage(stored, probe, dir, mediatools.DefaultMezzanine(), nil, nil,
		func() time.Time { return time.Unix(1_900_000_000, 0) })
	stage.transcode = func(_ context.Context, req mediatools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
		return MediaQuality{}, os.WriteFile(req.Out, newBytes, 0o644)
	}

	in := StoreClip{Clip: Clip{
		Hash: oldHash, Path: oldRel, Name: "McDonald's 1993", Kind: Commercial,
		Era: 1993, Audience: Kids, Category: "fast_food", Source: "archive:waga",
		ParentHash: "parent-reel", Held: true, Confidence: 91,
	}, UpdatedAt: time.Unix(1_800_000_000, 0)}
	out, err := stage.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	newHash, err := ClipID(filepath.Join(dir, filepath.FromSlash(out.Clip.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if out.Clip.Hash != newHash || stored.clip.Hash != newHash {
		t.Fatalf("transformed identity = %q / stored %q, want content hash %q", out.Clip.Hash, stored.clip.Hash, newHash)
	}
	if want := filepath.ToSlash(ClipRelPath(newHash, ".mp4")); out.Clip.Path != want {
		t.Errorf("transformed path = %q, want canonical %q", out.Clip.Path, want)
	}
	if stored.oldHash != oldHash {
		t.Errorf("replaced hash = %q, want %q", stored.oldHash, oldHash)
	}
	if stored.clip.Name != in.Name || stored.clip.Source != in.Source || stored.clip.ParentHash != in.ParentHash {
		t.Errorf("human metadata did not follow the transform: %+v", stored.clip.Clip)
	}
	if _, err := os.Stat(oldFull); !os.IsNotExist(err) {
		t.Errorf("old media still exists after durable replacement: %v", err)
	}

	tags, ok := ReadSidecarTags(filepath.Join(dir, filepath.FromSlash(out.Clip.Path)))
	if !ok {
		t.Fatal("transformed clip has no sidecar")
	}
	if tags.OriginalName != in.Name {
		t.Errorf("originalName = %q, want display name %q — a split child must not rescan as a hash", tags.OriginalName, in.Name)
	}
	if tags.Mezzanine != mediatools.DefaultMezzanine().ID() || tags.Era != in.Era || tags.Category != in.Category {
		t.Errorf("sidecar did not carry the transform metadata: %+v", tags)
	}

	// Drive the real scanner over the transformed layout. This is the lifecycle seam that
	// originally replaced the title with the stale hash on the pass after transcode.
	clips, skipped, err := ScanDir(context.Background(), dir, probe)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(clips) != 1 {
		t.Fatalf("rescan = %d clips / %d skipped, want one clean clip", len(clips), skipped)
	}
	if clips[0].ID != newHash || clips[0].Name != in.Name || clips[0].Path != out.Clip.Path {
		t.Errorf("rescan lost identity/title/path: %+v", clips[0])
	}
}

func TestScanDir_IgnoresTranscodeStaging(t *testing.T) {
	dir := t.TempDir()
	stageDir := filepath.Join(dir, transcodeStagingDir)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "partial.mp4"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	clips, skipped, err := ScanDir(context.Background(), dir, func(context.Context, string) (Probed, error) {
		return Probed{DurationMs: 30_000, Height: 480}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 0 || skipped != 0 {
		t.Fatalf("staging leaked into catalog: clips=%d skipped=%d", len(clips), skipped)
	}
}

func TestTranscodeStage_RejectsNearTotalBlackContentAndPersistsEvidence(t *testing.T) {
	dir := t.TempDir()
	oldHash := writeContentAddressedClip(t, dir, []byte("original"), ".mkv")
	oldRel := filepath.ToSlash(ClipRelPath(oldHash, ".mkv"))
	stored := &transcodeStore{}
	probe := func(context.Context, string) (Probed, error) {
		return Probed{DurationMs: 30_000, Height: 480}, nil
	}
	stage := NewTranscodeStage(stored, probe, dir, mediatools.DefaultMezzanine(), nil, nil, time.Now)
	stage.transcode = func(_ context.Context, req mediatools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
		return MediaQuality{DurationMs: 30_000, Black: []Interval{{StartMs: 0, EndMs: 28_000}}},
			os.WriteFile(req.Out, []byte("replacement"), 0o644)
	}
	out, err := stage.Run(context.Background(), StoreClip{Clip: Clip{
		Hash: oldHash, Path: oldRel, Name: "Named advert", Kind: Commercial,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != VerdictReject || out.Reason != ReasonBlackContent {
		t.Fatalf("quality verdict = %v/%q (%s)", out.Verdict, out.Reason, out.Detail)
	}
	tags, ok := ReadSidecarTags(filepath.Join(dir, filepath.FromSlash(out.Clip.Path)))
	if !ok || tags.MediaQuality == nil || len(tags.MediaQuality.Black) != 1 {
		t.Fatalf("sidecar quality evidence = %+v ok=%v", tags.MediaQuality, ok)
	}
}

func TestTranscodeStage_BackfillsQualityWithoutReencodingOldMezzanine(t *testing.T) {
	dir := t.TempDir()
	hash := writeContentAddressedClip(t, dir, []byte("old mezzanine"), ".mp4")
	rel := filepath.ToSlash(ClipRelPath(hash, ".mp4"))
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := WriteSidecarTags(full, SidecarTags{
		OriginalName: "Old advert", Mezzanine: mediatools.DefaultMezzanine().ID(),
	}, false); err != nil {
		t.Fatal(err)
	}
	stage := NewTranscodeStage(&transcodeStore{}, func(context.Context, string) (Probed, error) {
		return Probed{DurationMs: 30_000, Height: 480}, nil
	}, dir, mediatools.DefaultMezzanine(), nil, nil, time.Now)
	stage.transcode = func(context.Context, mediatools.TranscodeRequest, func(int)) (MediaQuality, error) {
		t.Fatal("an existing mezzanine must not be re-encoded to backfill quality")
		return MediaQuality{}, nil
	}
	stage.inspect = func(context.Context, string, string, int64, bool) (MediaQuality, error) {
		return MediaQuality{DurationMs: 30_000, Silence: []Interval{{StartMs: 0, EndMs: 29_000}}}, nil
	}
	out, err := stage.Run(context.Background(), StoreClip{Clip: Clip{
		Hash: hash, Path: rel, Name: "Old advert", Kind: Commercial,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != VerdictReject || out.Reason != ReasonSilentContent {
		t.Fatalf("backfill verdict = %v/%q", out.Verdict, out.Reason)
	}
	tags, ok := ReadSidecarTags(full)
	if !ok || tags.MediaQuality == nil {
		t.Fatal("quality backfill was not persisted beside the old mezzanine")
	}
	if applies, _ := stage.Applies(context.Background(), out.Clip); !applies {
		t.Fatal("an anomalous report must remain applicable until its airability gate is durable")
	}
	stage.inspect = func(context.Context, string, string, int64, bool) (MediaQuality, error) {
		t.Fatal("retry must use the persisted quality evidence")
		return MediaQuality{}, nil
	}
	retry, err := stage.Run(context.Background(), out.Clip)
	if err != nil || retry.Verdict != VerdictReject || retry.Reason != ReasonSilentContent {
		t.Fatalf("persisted quality retry = %+v, %v", retry, err)
	}
}

func writeContentAddressedClip(t *testing.T, dir string, body []byte, ext string) string {
	t.Helper()
	tmp := filepath.Join(dir, "source"+ext)
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := ClipID(tmp)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, ClipRelPath(hash, ext))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		t.Fatal(err)
	}
	return hash
}
