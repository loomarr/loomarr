package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/creative"
	"github.com/loomarr/loomarr/internal/defects"
)

type defectMemStore struct {
	clips   []DefectClip
	records map[string]DefectRecord
	fps     map[string]creative.Fingerprint
	retired []string
}

func (m *defectMemStore) RetireDefectiveCopy(_ context.Context, hash string, at time.Time) error {
	for i := range m.clips {
		if m.clips[i].Hash == hash {
			m.clips[i].RemovedAt = at
			m.retired = append(m.retired, hash)
			return nil
		}
	}
	return errors.New("no such clip")
}

func (m *defectMemStore) ListClipFingerprints(context.Context, string) (map[string]creative.Fingerprint, error) {
	out := map[string]creative.Fingerprint{}
	for k, v := range m.fps {
		out[k] = v
	}
	return out, nil
}

func (m *defectMemStore) UpsertClipFingerprint(_ context.Context, hash, _ string, fp creative.Fingerprint) error {
	if m.fps == nil {
		m.fps = map[string]creative.Fingerprint{}
	}
	m.fps[hash] = fp
	return nil
}

// someFingerprint stands in for a real one: the floor only needs one to exist.
func someFingerprint(context.Context, string, int64) (creative.Fingerprint, error) {
	return creative.Fingerprint{DurationMs: 30_000, Video: []uint64{1}}, nil
}

func (m *defectMemStore) ListDefectClips(context.Context) ([]DefectClip, error) { return m.clips, nil }
func (m *defectMemStore) ListClipDefects(context.Context) (map[string]DefectRecord, error) {
	out := make(map[string]DefectRecord, len(m.records))
	for k, v := range m.records {
		out[k] = v
	}
	return out, nil
}
func (m *defectMemStore) UpsertClipDefects(_ context.Context, r DefectRecord) error {
	m.records[r.ClipHash] = r
	return nil
}

// One pass measures what has no current record, re-judges a record whose era was corrected without
// decoding again, skips what is current, and leaves composites and unplayable clips alone. A clip
// that cannot be measured is counted and tried again next pass.
func TestDefectFloor_KeepsEveryRecordCurrentAndDecodesOnlyWhatIsNew(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	soft := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 8.5}

	clip := func(body string, era int, edit func(*DefectClip)) DefectClip {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		c := DefectClip{CreatedAt: t0}
		c.Hash, c.Path, c.Era, c.Kind, c.Placement = hash, ClipRelPath(hash, ".mp4"), era, Commercial, PlacementBreakBody
		if edit != nil {
			edit(&c)
		}
		return c
	}
	fresh := clip("fresh", 1988, nil)
	held := clip("held", 2012, func(c *DefectClip) { c.Held = true })
	current := clip("current", 1988, nil)
	recorrected := clip("re-dated", 2015, nil) // judged as 1988 before an operator fixed its era
	broken := clip("broken", 1999, nil)
	composite := clip("reel", 1990, func(c *DefectClip) { c.IsComposite = true })
	unplayable := clip("unplayable", 1990, func(c *DefectClip) { c.Placement = PlacementNotPlayable })

	st := &defectMemStore{
		clips: []DefectClip{fresh, held, current, recorrected, broken, composite, unplayable},
		records: map[string]DefectRecord{
			current.Hash:     {ClipHash: current.Hash, Readings: soft, Era: 1988, Assessment: defects.Assess(soft, 1988)},
			recorrected.Hash: {ClipHash: recorrected.Hash, Readings: soft, Era: 1988, Assessment: defects.Assess(soft, 1988)},
		},
	}
	var decoded []string
	measure := func(_ context.Context, file string, w, h int, hasAudio bool) (defects.Readings, error) {
		hash := strings.TrimSuffix(filepath.Base(file), ".mp4")
		decoded = append(decoded, hash)
		if hash == broken.Hash {
			return defects.Readings{}, errors.New("decoder gave up")
		}
		if w != 640 || h != 480 || !hasAudio {
			t.Errorf("measured %s as %dx%d audio=%v, want the probed 640x480 with sound", hash, w, h, hasAudio)
		}
		return soft, nil
	}
	probe := func(context.Context, string) (Probed, error) { return Probed{Width: 640, Height: 480}, nil }

	res, err := NewDefectFloor(st, probe, measure, someFingerprint, nil, dir, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Ruled: fresh and current are borderline tape softness, recorded as having no judge.
	if res != (DefectFloorResult{Measured: 2, Reassessed: 1, Failed: 1, Ruled: 2, Retired: 2}) {
		t.Fatalf("pass = %+v, want 2 measured (fresh, held), 1 re-judged, 1 failed, 2 ruled, 2 retired", res)
	}
	// #2036: the HD-era copies fail on the detectors' own reading and are retired, held or on air,
	// files and all. The tape-era one failed only for want of a judge, so it keeps its file.
	if !slices.Equal(st.retired, []string{held.Hash, recorrected.Hash}) {
		t.Fatalf("retired %v, want the held clip first, then the re-dated one", st.retired)
	}
	for _, c := range []DefectClip{held, recorrected} {
		if _, err := os.Stat(filepath.Join(dir, c.Path)); !os.IsNotExist(err) {
			t.Errorf("retired copy %s is still on disk (err %v)", c.Hash, err)
		}
		if _, ok := st.fps[c.Hash]; !ok {
			t.Errorf("retired copy %s has no fingerprint to match a better copy against", c.Hash)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, fresh.Path)); err != nil {
		t.Errorf("the provisionally failed copy lost its file: %v", err)
	}
	if len(decoded) != 3 {
		t.Fatalf("decoded %d clips, want only fresh, held and broken", len(decoded))
	}
	if got := st.records[recorrected.Hash]; got.Era != 2015 || got.Assessment.Outcome != defects.Fail {
		t.Fatalf("re-dated record = era %d %q, want the HD bar's fail at 2015", got.Era, got.Assessment.Outcome)
	}
	if got := st.records[fresh.Hash]; got.Assessment.Outcome != defects.Borderline || got.Readings.Version != defects.MeasurementVersion ||
		got.Verdict != defects.VerdictFail || got.Judgement == nil || got.Judgement.Ruling != defects.RulingNoJudge {
		t.Fatalf("fresh tape-era record = %+v, want borderline softness failed for want of a judge", got)
	}
	for _, skipped := range []string{composite.Hash, unplayable.Hash} {
		if _, ok := st.records[skipped]; ok {
			t.Errorf("clip %s got a record", skipped)
		}
	}
	if got := st.records[broken.Hash]; got.Assessment.Outcome != defects.Undecided || got.Readings.Unmeasurable == "" {
		t.Fatalf("broken record = %+v, want undecided with the reason it could not be measured", got)
	}

	// The next pass decodes nothing: the failure is recorded, not retried every hour.
	decoded = nil
	res, err = NewDefectFloor(st, probe, measure, someFingerprint, nil, dir, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil || len(decoded) != 0 || res != (DefectFloorResult{}) {
		t.Fatalf("second pass = %+v, decoded %v, err %v; want nothing to do", res, decoded, err)
	}
}

// #2036: retiring a copy deletes only what is its own. A source master another clip's sidecar also
// names stays. A copy that cannot be fingerprinted keeps its file until a later pass can: without a
// fingerprint, no cleaner copy of the ad could be recognised.
func TestDefectFloor_RetiresOnlyWithAFingerprintAndKeepsSharedAssets(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	blocky := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlockMean: 90}

	master := func(name string) MediaAssetIdentity {
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		hash, err := ClipID(src)
		if err != nil {
			t.Fatal(err)
		}
		m, err := preserveSourceMaster(ctx, dir, src, hash, SidecarTags{})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	clip := func(body string, m MediaAssetIdentity) DefectClip {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		if err := WriteSidecarTags(filepath.Join(dir, ClipRelPath(hash, ".mp4")), SidecarTags{MediaAssets: &MediaAssetManifest{
			Version: mediaAssetManifestVersion, SourceMaster: m,
		}}, false); err != nil {
			t.Fatal(err)
		}
		c := DefectClip{CreatedAt: t0}
		c.Hash, c.Path, c.Era, c.Kind, c.Placement, c.DurationMs = hash, ClipRelPath(hash, ".mp4"), 1988, Commercial, PlacementBreakBody, 30_000
		return c
	}
	shared, own := master("shared-source.mkv"), master("own-source.mkv")
	failing := clip("blocky copy", own)
	sibling := clip("blocky cut", shared) // also fails, but shares its master with a clean clip
	clean := clip("clean copy", shared)
	unfingerprintable := clip("blocky, no picture to fingerprint", MediaAssetIdentity{})

	st := &defectMemStore{clips: []DefectClip{failing, sibling, clean, unfingerprintable}, records: map[string]DefectRecord{}}
	measure := func(_ context.Context, file string, _, _ int, _ bool) (defects.Readings, error) {
		if strings.Contains(file, clean.Hash) {
			return defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40}, nil
		}
		return blocky, nil
	}
	fingerprint := func(ctx context.Context, file string, ms int64) (creative.Fingerprint, error) {
		if strings.Contains(file, unfingerprintable.Hash) {
			return creative.Fingerprint{}, errors.New("no frames")
		}
		return someFingerprint(ctx, file, ms)
	}
	probe := func(context.Context, string) (Probed, error) { return Probed{Width: 640, Height: 480}, nil }
	res, err := NewDefectFloor(st, probe, measure, fingerprint, nil, dir, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Retired != 2 || !slices.Contains(st.retired, failing.Hash) || !slices.Contains(st.retired, sibling.Hash) {
		t.Fatalf("pass = %+v retired %v, want the two blocky copies that could be fingerprinted", res, st.retired)
	}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}
	if exists(own.Path) || exists(failing.Path) || exists(sibling.Path) {
		t.Error("a retired copy's own file or master survived")
	}
	if !exists(shared.Path) || !exists(clean.Path) {
		t.Error("a master the clean copy still names, or the clean copy itself, was deleted")
	}
	if !exists(unfingerprintable.Path) {
		t.Error("a failing copy with no fingerprint was deleted, losing the ad")
	}
}

// #2036: an admitted clip keeps its source master and playback copy; its evidence copy is
// dropped, the sidecar first, and is rebuilt by a rewind that starts at the transcode. A copy
// still in the conveyor needs it, so one it shares stays on disk.
func TestDefectFloor_ReleasesAnAdmittedClipsEvidenceCopy(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	manifest := screeningSubjectManifest(t)
	evidenceFile := filepath.Join(dir, filepath.FromSlash(manifest.Evidence.Asset.Path))
	if err := os.MkdirAll(filepath.Dir(evidenceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidenceFile, []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	clip := func(body string, held bool) DefectClip {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		m := manifest
		if err := WriteSidecarTags(filepath.Join(dir, ClipRelPath(hash, ".mp4")), SidecarTags{MediaAssets: &m}, false); err != nil {
			t.Fatal(err)
		}
		c := DefectClip{CreatedAt: t0}
		c.Hash, c.Path, c.Era, c.Kind, c.Placement, c.Held = hash, ClipRelPath(hash, ".mp4"), 1988, Commercial, PlacementBreakBody, held
		return c
	}
	admitted, waiting := clip("an admitted ad", false), clip("an ad still at the defects rung", true)
	st := &defectMemStore{clips: []DefectClip{admitted, waiting}, records: map[string]DefectRecord{}}
	measure := func(context.Context, string, int, int, bool) (defects.Readings, error) {
		return defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40}, nil
	}
	probe := func(context.Context, string) (Probed, error) { return Probed{Width: 640, Height: 480}, nil }
	floor := NewDefectFloor(st, probe, measure, someFingerprint, nil, dir, func() time.Time { return t0 }, nil)

	res, err := floor.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Released != 1 {
		t.Fatalf("pass = %+v, want the admitted clip's evidence released", res)
	}
	if tags, ok := ReadSidecarTags(filepath.Join(dir, admitted.Path)); !ok || tags.MediaAssets == nil ||
		tags.MediaAssets.Evidence != nil || tags.MediaAssets.SourceMaster != manifest.SourceMaster || tags.MediaAssets.Playback == nil {
		t.Fatalf("admitted sidecar = %+v (ok %v), want the master and playback kept and no evidence", tags.MediaAssets, ok)
	}
	if tags, _ := ReadSidecarTags(filepath.Join(dir, waiting.Path)); tags.MediaAssets == nil || tags.MediaAssets.Evidence == nil {
		t.Fatal("a held clip's evidence was released before admission")
	}
	if _, err := os.Stat(evidenceFile); err != nil {
		t.Fatalf("evidence the held clip still names was deleted: %v", err)
	}
	if !(&Pipeline{clipDir: dir}).evidenceReleased(StoreClip{Clip: admitted.Clip}) ||
		(&Pipeline{clipDir: dir}).evidenceReleased(StoreClip{Clip: waiting.Clip}) {
		t.Fatal("a rewind would not know which clip must rebuild its evidence from the transcode")
	}

	// Admitted too, now: the last clip naming the file releases it, and the file goes.
	for i := range st.clips {
		if st.clips[i].Hash == waiting.Hash {
			st.clips[i].Held = false
		}
	}
	if res, err := floor.Run(ctx); err != nil || res.Released != 1 {
		t.Fatalf("second pass = %+v (err %v), want the other clip released", res, err)
	}
	if _, err := os.Stat(evidenceFile); !os.IsNotExist(err) {
		t.Fatalf("evidence no clip names any more is still on disk (err %v)", err)
	}
	if res, err := floor.Run(ctx); err != nil || res.Released != 0 {
		t.Fatalf("third pass = %+v (err %v), want nothing left to release", res, err)
	}

	// Two admitted clips sharing one evidence copy, released in the same pass: the file goes with
	// the last of them, not never.
	if err := os.WriteFile(evidenceFile, []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	pair := &defectMemStore{clips: []DefectClip{clip("one cut", false), clip("its twin", false)}, records: map[string]DefectRecord{}}
	res, err = NewDefectFloor(pair, probe, measure, someFingerprint, nil, dir, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil || res.Released != 2 {
		t.Fatalf("pair pass = %+v (err %v), want both released", res, err)
	}
	if _, err := os.Stat(evidenceFile); !os.IsNotExist(err) {
		t.Fatalf("evidence both clips released in one pass is still on disk (err %v)", err)
	}
}

type failingDefectStore struct{ defectMemStore }

func (failingDefectStore) ListClipDefects(context.Context) (map[string]DefectRecord, error) {
	return nil, errors.New("database is locked")
}

// A failed read is not a partial one: carrying on would make every clip look unmeasured.
func TestDefectFloor_AFailedRecordReadStopsThePass(t *testing.T) {
	st := &failingDefectStore{defectMemStore{clips: []DefectClip{{}}, records: map[string]DefectRecord{}}}
	measure := func(context.Context, string, int, int, bool) (defects.Readings, error) {
		t.Fatal("a clip was decoded after the record read failed")
		return defects.Readings{}, nil
	}
	if _, err := NewDefectFloor(st, nil, measure, someFingerprint, nil, t.TempDir(), time.Now, nil).Run(context.Background()); err == nil {
		t.Fatal("the pass carried on past a failed record read")
	}
}

// A pass that runs out of decode budget leaves the rest pending rather than overrunning its lease.
func TestDefectFloor_StopsDecodingWhenTheBudgetRunsOut(t *testing.T) {
	dir := t.TempDir()
	clock := time.Unix(1_800_000_000, 0).UTC()
	var clips []DefectClip
	for _, body := range []string{"a", "b", "c"} {
		hash := writeContentAddressedClip(t, dir, []byte(body), ".mp4")
		c := DefectClip{CreatedAt: clock}
		c.Hash, c.Path, c.Kind, c.Placement = hash, ClipRelPath(hash, ".mp4"), Commercial, PlacementBreakBody
		clips = append(clips, c)
	}
	st := &defectMemStore{clips: clips, records: map[string]DefectRecord{}}
	measure := func(context.Context, string, int, int, bool) (defects.Readings, error) {
		clock = clock.Add(defectMeasureBudget) // each decode spends the whole budget
		return defects.Readings{Version: defects.MeasurementVersion}, nil
	}
	probe := func(context.Context, string) (Probed, error) { return Probed{Width: 640, Height: 480}, nil }
	res, err := NewDefectFloor(st, probe, measure, someFingerprint, nil, dir, func() time.Time { return clock }, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Measured != 2 || res.Pending != 1 {
		t.Fatalf("pass = %+v, want 2 measured before the budget ran out and 1 pending", res)
	}
}
