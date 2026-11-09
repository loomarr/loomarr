package filler

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// airingStore records the switch ReplaceAiring commits.
type airingStore struct {
	calls   int
	oldHash string
	clip    StoreClip
	record  defects.Record
	err     error
}

func (s *airingStore) ReplaceAiringClip(_ context.Context, oldHash string, c StoreClip, rec defects.Record) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	s.oldHash, s.clip, s.record = oldHash, c, rec
	return nil
}

// derivativeStage is a transcode stage whose one encode writes body, on host when one is given.
func derivativeStage(t *testing.T, dir string, host *encode.HostProfile, body string) *TranscodeStage {
	t.Helper()
	probe := func(context.Context, string) (Probed, error) { return Probed{DurationMs: 30_000, Height: 480}, nil }
	stage := NewTranscodeStage(&transcodeStore{}, probe, dir, tools.DefaultMezzanine(), nil, nil, time.Now).WithMediaDerivatives()
	if host != nil {
		stage.WithEncodeHost(func() encode.HostProfile { return *host })
	}
	stage.identifyFFmpeg = func(context.Context, string) (tools.MediaToolIdentity, error) {
		return tools.MediaToolIdentity{Name: "ffmpeg", Version: "fixture", ExecutableSHA256: strings.Repeat("a", 64)}, nil
	}
	stage.verifyDerivative = func(_ context.Context, _, _ string, durationMs int64, keyframeSeconds int, hadAudio bool, targetLUFS float64) (tools.DerivativeQC, error) {
		return fixtureDerivativeQC(durationMs, keyframeSeconds, hadAudio, targetLUFS), nil
	}
	stage.evidenceTranscode = func(_ context.Context, request tools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
		return MediaQuality{DurationMs: 30_000}, os.WriteFile(request.Out, []byte(body), 0o600)
	}
	stage.transcode = func(context.Context, tools.TranscodeRequest, func(int)) (MediaQuality, error) {
		t.Fatal("a top-level clip's playback rendition was encoded a second time")
		return MediaQuality{}, nil
	}
	return stage
}

var nvencHost = encode.HostProfile{Family: encode.FamilyNVENC, Encoder: encode.EncoderNVENC}

// airingFixture is a clip on air whose playback was encoded from its retained master under the
// software profile, and a stage on an NVENC host that re-encodes it, gated by check.
func airingFixture(t *testing.T, check ReplacementCheck) (string, *TranscodeStage, *airingStore, StoreClip) {
	t.Helper()
	dir := t.TempDir()
	hash := writeContentAddressedClip(t, dir, []byte("the airing clip's source master"), ".mkv")
	out, err := derivativeStage(t, dir, nil, "software derivative").Run(context.Background(),
		StoreClip{Clip: Clip{Hash: hash, Path: filepath.ToSlash(ClipRelPath(hash, ".mkv")), Name: "Advert", Role: Commercial}})
	if err != nil {
		t.Fatal(err)
	}
	store := &airingStore{}
	stage := derivativeStage(t, dir, &nvencHost, "nvenc derivative").WithAiringReplacement(store, check)
	return dir, stage, store, out.Clip
}

func passingCheck(staged *string) ReplacementCheck {
	return func(_ context.Context, c StoreClip, path string) (defects.Record, error) {
		if staged != nil {
			*staged = path
		}
		return defects.Record{ClipHash: c.Hash, Verdict: defects.VerdictPass, VerdictReason: "detectors"}, nil
	}
}

// filesHolding lists every file under dir whose bytes are body.
func filesHolding(t *testing.T, dir, body string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil && bytes.Equal(raw, []byte(body)) {
			found = append(found, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// #2121 decisions 8 and 9: an airing clip is re-encoded from its master onto the current profile,
// judged in hidden staging, and switched in one commit that carries its new defect verdict. The
// old playback file goes; the master stays.
func TestReplaceAiring_SwitchesAnAiringClipToItsReencode(t *testing.T) {
	var staged string
	dir, stage, store, airing := airingFixture(t, passingCheck(&staged))
	oldFull := filepath.Join(dir, filepath.FromSlash(airing.Path))
	oldTags, _ := asset.ReadSidecarTags(oldFull)

	got, err := stage.ReplaceAiring(context.Background(), airing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(staged, filepath.Join(dir, transcodeStagingDir)+string(filepath.Separator)) {
		t.Errorf("the replacement was judged at %q, not in hidden staging", staged)
	}
	if store.calls != 1 || store.oldHash != airing.Hash || store.clip.Hash != got.Hash || got.Hash == airing.Hash {
		t.Fatalf("switch = %d calls %s → %s, returned %s", store.calls, store.oldHash, store.clip.Hash, got.Hash)
	}
	if store.record.Verdict != defects.VerdictPass {
		t.Errorf("the switch carried defect record %+v, not the replacement's", store.record)
	}
	newFull := filepath.Join(dir, filepath.FromSlash(got.Path))
	if raw, err := os.ReadFile(newFull); err != nil || string(raw) != "nvenc derivative" {
		t.Fatalf("new playback = %q, %v", raw, err)
	}
	tags, ok := asset.ReadSidecarTags(newFull)
	if !ok || tags.Mezzanine != tools.FileMezzanine(nvencHost).ID() || tags.MediaAssets == nil ||
		tags.MediaAssets.SourceMaster.Path != oldTags.MediaAssets.SourceMaster.Path {
		t.Fatalf("new sidecar = %+v", tags)
	}
	if _, err := os.Stat(oldFull); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old playback file is still on disk: %v", err)
	}
	if _, err := os.Stat(tools.SidecarPathFor(oldFull)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old sidecar is still on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldTags.MediaAssets.Evidence.Asset.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old evidence copy is still on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldTags.MediaAssets.SourceMaster.Path))); err != nil {
		t.Errorf("the source master is gone: %v", err)
	}
}

// #2121 decision 11: a replacement that does not pass is discarded and the clip airs on as it was:
// its file and sidecar untouched, nothing switched, and none of the replacement's bytes left.
func TestReplaceAiring_ARefusedReplacementLeavesTheClipAiring(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check ReplacementCheck
		want  error
	}{
		{"defect floor fails it", func(_ context.Context, c StoreClip, _ string) (defects.Record, error) {
			return defects.Record{ClipHash: c.Hash, Verdict: defects.VerdictFail, VerdictReason: "detectors"}, nil
		}, ErrReplacementRefused},
		{"judge has not ruled", func(_ context.Context, c StoreClip, _ string) (defects.Record, error) {
			return defects.Record{ClipHash: c.Hash, Verdict: defects.VerdictPending, VerdictReason: "waiting for the judge"}, nil
		}, ErrReplacementRefused},
		{"floor could not measure it", func(context.Context, StoreClip, string) (defects.Record, error) {
			return defects.Record{}, errors.New("decoder rejected the stream")
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, stage, store, airing := airingFixture(t, tc.check)
			oldFull := filepath.Join(dir, filepath.FromSlash(airing.Path))
			before, _ := os.ReadFile(tools.SidecarPathFor(oldFull))

			_, err := stage.ReplaceAiring(context.Background(), airing)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if store.calls != 0 {
				t.Fatalf("a refused replacement was switched")
			}
			if raw, err := os.ReadFile(oldFull); err != nil || string(raw) != "software derivative" {
				t.Fatalf("airing file = %q, %v", raw, err)
			}
			if after, _ := os.ReadFile(tools.SidecarPathFor(oldFull)); !bytes.Equal(before, after) {
				t.Fatalf("airing sidecar changed:\n%s\n→\n%s", before, after)
			}
			if left := filesHolding(t, dir, "nvenc derivative"); len(left) != 0 {
				t.Fatalf("the refused replacement left %v", left)
			}
		})
	}
}

// A switch that fails to commit takes the published replacement down again, so the next scan
// cannot catalogue it as a second clip beside the one still airing.
func TestReplaceAiring_AFailedSwitchTakesTheReplacementDown(t *testing.T) {
	dir, stage, store, airing := airingFixture(t, passingCheck(nil))
	store.err = errors.New("database is locked")

	if _, err := stage.ReplaceAiring(context.Background(), airing); err == nil {
		t.Fatal("a failed switch reported success")
	}
	if left := filesHolding(t, dir, "nvenc derivative"); len(left) != 0 {
		t.Fatalf("the uncommitted replacement left %v", left)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(airing.Path))); err != nil || string(raw) != "software derivative" {
		t.Fatalf("airing file = %q, %v", raw, err)
	}
}

// Only an airing top-level clip with an original to encode from, under another profile, on a
// hardware host, is replaced.
func TestNeedsReencode_OnlyAiringOriginalsUnderAnotherProfileOnHardware(t *testing.T) {
	dir, stage, _, airing := airingFixture(t, passingCheck(nil))
	if !stage.NeedsReencode(airing) {
		t.Fatal("the airing software-profile clip is not due a re-encode on an NVENC host")
	}
	held, removed, composite, child := airing, airing, airing, airing
	held.Held = true
	removed.RemovedAt = time.Now()
	composite.IsComposite = true
	child.ParentHash = strings.Repeat("b", 64)
	for name, c := range map[string]StoreClip{"held": held, "removed": removed, "composite": composite, "split child": child} {
		if stage.NeedsReencode(c) {
			t.Errorf("a %s clip is due a re-encode", name)
		}
	}

	software := encode.HostProfile{Family: encode.FamilySoftware}
	onCPU := derivativeStage(t, dir, &software, "cpu").WithAiringReplacement(&airingStore{}, passingCheck(nil))
	if onCPU.NeedsReencode(airing) {
		t.Error("a host without a hardware encoder re-encodes the library")
	}

	full := filepath.Join(dir, filepath.FromSlash(airing.Path))
	tags, _ := asset.ReadSidecarTags(full)
	tags.Mezzanine = tools.FileMezzanine(nvencHost).ID()
	if err := asset.WriteSidecarTags(full, tags, false); err != nil {
		t.Fatal(err)
	}
	if stage.NeedsReencode(airing) {
		t.Error("a clip already under the current profile is due a re-encode")
	}
	tags.Mezzanine, tags.MediaAssets.Playback = tools.DefaultMezzanine().ID(), nil
	if err := asset.WriteSidecarTags(full, tags, false); err != nil {
		t.Fatal(err)
	}
	if stage.NeedsReencode(airing) {
		t.Error("a clip whose playback is its own download is due a re-encode")
	}
}
