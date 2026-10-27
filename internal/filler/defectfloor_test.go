package filler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

type defectMemStore struct {
	clips   []DefectClip
	records map[string]DefectRecord
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
	soft := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 10.5}

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

	res, err := NewDefectFloor(st, probe, measure, dir, func() time.Time { return t0 }, nil).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res != (DefectFloorResult{Measured: 2, Reassessed: 1, Failed: 1}) {
		t.Fatalf("pass = %+v, want 2 measured (fresh, held), 1 re-judged, 1 failed", res)
	}
	if len(decoded) != 3 {
		t.Fatalf("decoded %d clips, want only fresh, held and broken", len(decoded))
	}
	if got := st.records[recorrected.Hash]; got.Era != 2015 || got.Assessment.Outcome != defects.Fail {
		t.Fatalf("re-dated record = era %d %q, want the HD bar's fail at 2015", got.Era, got.Assessment.Outcome)
	}
	if got := st.records[fresh.Hash]; got.Assessment.Outcome != defects.Borderline || got.Readings.Version != defects.MeasurementVersion {
		t.Fatalf("fresh tape-era record = %+v, want borderline softness", got)
	}
	for _, skipped := range []string{composite.Hash, unplayable.Hash, broken.Hash} {
		if _, ok := st.records[skipped]; ok {
			t.Errorf("clip %s got a record", skipped)
		}
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
	res, err := NewDefectFloor(st, probe, measure, dir, func() time.Time { return clock }, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Measured != 2 || res.Pending != 1 {
		t.Fatalf("pass = %+v, want 2 measured before the budget ran out and 1 pending", res)
	}
}
