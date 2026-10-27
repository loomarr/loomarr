package filler

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

// defectMeasureBudget bounds how long one pass spends decoding. A pass that runs out leaves the rest
// for the next; only a first pass over a large catalog takes more than one.
const defectMeasureBudget = 40 * time.Second

// DefectRecord is one clip's latest defect measurement and the verdict it earned (#2034).
type DefectRecord struct {
	ClipHash   string
	Readings   defects.Readings
	Era        int
	Assessment defects.Assessment
	MeasuredAt time.Time
}

// ErrDefectRecordsCorrupt marks a ListClipDefects result that omitted unreadable rows; the rest of
// the map is usable. Any other error is a failed read.
var ErrDefectRecordsCorrupt = errors.New("some defect records are corrupt")

// DefectStore is what the defect floor reads and writes.
type DefectStore interface {
	// ListDefectClips is every catalog clip, held ones included: the floor is judged before a
	// clip is reviewed, so the record can say what it would retire.
	ListDefectClips(ctx context.Context) ([]DefectClip, error)
	ListClipDefects(ctx context.Context) (map[string]DefectRecord, error)
	UpsertClipDefects(ctx context.Context, r DefectRecord) error
}

// DefectClip is a catalog clip with its admission time, so a backlog is measured oldest first.
type DefectClip struct {
	StoreClip
	CreatedAt time.Time
}

// MeasureDefectsFunc decodes one file's defect readings (mediatools.MeasureDefects).
type MeasureDefectsFunc func(ctx context.Context, file string, width, height int, hasAudio bool) (defects.Readings, error)

// DefectFloor is the standing job that keeps every clip's defect record current (#2034). It only
// records: nothing is held or retired on a verdict until enforcement (#2036).
type DefectFloor struct {
	store   DefectStore
	probe   Prober
	measure MeasureDefectsFunc
	clipDir string
	now     func() time.Time
	log     *slog.Logger
}

// DefectFloorResult counts one pass.
type DefectFloorResult struct {
	Measured   int // decoded and judged
	Reassessed int // judged again from stored readings: a new floor version or a corrected era
	Pending    int // left for the next pass when the decode budget ran out
	Failed     int // could not be probed or measured; recorded as undecided with the reason
}

// NewDefectFloor builds the defect floor job.
func NewDefectFloor(store DefectStore, probe Prober, measure MeasureDefectsFunc, clipDir string,
	now func() time.Time, log *slog.Logger) *DefectFloor {
	return &DefectFloor{store: store, probe: probe, measure: measure, clipDir: clipDir, now: now, log: log}
}

// Run measures every clip without a current measurement and re-judges every record whose floor
// version or era is stale. Re-judging needs no decode, so a floor recalibration reaches the whole
// catalog in one pass.
func (d *DefectFloor) Run(ctx context.Context) (DefectFloorResult, error) {
	var res DefectFloorResult
	clips, err := d.store.ListDefectClips(ctx)
	if err != nil {
		return res, err
	}
	records, err := d.store.ListClipDefects(ctx)
	switch {
	case errors.Is(err, ErrDefectRecordsCorrupt):
		// Corrupt rows are omitted, so those clips are measured again.
		d.warn("defects: some records were unreadable", "err", err)
	case err != nil:
		// Not a partial read: every clip would look unmeasured and be decoded again.
		return res, err
	}
	slices.SortFunc(clips, func(a, b DefectClip) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.Hash, b.Hash))
	})
	deadline := d.now().Add(defectMeasureBudget)
	for _, c := range clips {
		if !defectCandidate(c.Clip) {
			continue
		}
		rec, known := records[c.Hash]
		if known && rec.Readings.Version == defects.MeasurementVersion {
			if rec.Assessment.Version == defects.FloorVersion && rec.Era == c.Era {
				continue
			}
			rec.Era, rec.Assessment = c.Era, defects.Assess(rec.Readings, c.Era)
			if err := d.store.UpsertClipDefects(ctx, rec); err != nil {
				return res, err
			}
			res.Reassessed++
			continue
		}
		if ctx.Err() != nil || d.now().After(deadline) {
			res.Pending++
			continue
		}
		readings, err := d.measureClip(ctx, c.Clip)
		if err != nil {
			if ctx.Err() != nil {
				res.Pending++ // the pass was cancelled, not the clip
				continue
			}
			// Recorded rather than retried: a clip with no picture, or one the decoder rejects, fails
			// the same way every hour and would keep spending the budget ahead of healthy clips. A
			// new MeasurementVersion measures it again.
			d.warn("defects: clip could not be measured", "clip", c.Hash, "err", err)
			readings = defects.Readings{Version: defects.MeasurementVersion, Unmeasurable: err.Error()}
			res.Failed++
		} else {
			res.Measured++
		}
		if err := d.store.UpsertClipDefects(ctx, DefectRecord{
			ClipHash: c.Hash, Readings: readings, Era: c.Era,
			Assessment: defects.Assess(readings, c.Era), MeasuredAt: d.now(),
		}); err != nil {
			return res, err
		}
	}
	if d.log != nil && (res.Measured > 0 || res.Reassessed > 0 || res.Failed > 0) {
		d.log.Info("defects: records current", "measured", res.Measured, "reassessed", res.Reassessed,
			"pending", res.Pending, "failed", res.Failed)
	}
	return res, ctx.Err()
}

// measureClip probes the playback file for its frame size and sound, then measures it. The file
// judged is the one that airs.
func (d *DefectFloor) measureClip(ctx context.Context, c Clip) (defects.Readings, error) {
	full, err := ClipPath(d.clipDir, c.Path, filepath.Ext(c.Path))
	if err != nil {
		return defects.Readings{}, err
	}
	p, err := d.probe(ctx, full)
	if err != nil {
		return defects.Readings{}, err
	}
	return d.measure(ctx, full, p.Width, p.Height, !p.Silent)
}

// defectCandidate is a clip that may air as an ad of its own once admitted. A composite is a
// recorded break and is judged through its segments; an unplayable clip has no picture to judge.
func defectCandidate(c Clip) bool {
	return !c.IsComposite && c.RemovedAt.IsZero() && c.Placement != PlacementNotPlayable
}

func (d *DefectFloor) warn(msg string, args ...any) {
	if d.log != nil {
		d.log.Warn(msg, args...)
	}
}
