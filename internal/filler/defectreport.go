package filler

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

// The household defect report (#2034 step 4c): what the floor would retire on this install, and
// how its bars and judge score against the maintainer's own labels. The synthetic corpus proves
// each bar's direction; real clips, which never leave the machine, set where it sits.

// DefectLabel is the maintainer's label on a clip's defect record, and the era it was given for.
type DefectLabel struct {
	Label      defects.Label
	Era        int
	LabelledAt time.Time
}

// DefectReportStore reads the floor's records and the labels on them.
type DefectReportStore interface {
	ListDefectClips(ctx context.Context) ([]DefectClip, error)
	ListClipDefects(ctx context.Context) (map[string]DefectRecord, error)
	ListClipDefectLabels(ctx context.Context) (map[string]DefectLabel, error)
	// SetClipDefectLabel labels a record for the era it was judged against; an empty label clears
	// it. ErrDefectNotMeasured for a clip with no record.
	SetClipDefectLabel(ctx context.Context, clipHash string, label defects.Label, at time.Time) error
}

// ErrDefectNotMeasured: only a clip the floor has measured can be labelled.
var ErrDefectNotMeasured = errors.New("the defect floor has not measured this clip")

// DefectReportFilter narrows the rows; the summary and scores always cover every record.
type DefectReportFilter struct {
	Verdict defects.Verdict // empty for every verdict
	// Unlabelled keeps only rows with no current label: the maintainer's queue.
	Unlabelled    bool
	Limit, Offset int
}

// DefectReport is the floor's standing on this install.
type DefectReport struct {
	FloorVersion string
	// Clips is the catalog; Unmeasured have no readable record yet.
	Clips, Unmeasured int
	Verdicts          map[defects.Verdict]int
	// Labelled counts current labels: given for the era band the clip is judged in now.
	Labelled int
	Scores   []defects.BandScore
	// Total is how many rows match the filter; Rows is the requested page of them, worst first.
	Total int
	Rows  []DefectReportRow
}

// DefectReportRow is one measured clip.
type DefectReportRow struct {
	Clip   StoreClip
	Record DefectRecord
	Label  *DefectLabel
	// LabelCurrent is false when the clip's era was corrected into another band after labelling:
	// the label answers a question the floor no longer asks, and is not scored.
	LabelCurrent bool
}

// DefectReports reads the household report and writes the maintainer's labels.
type DefectReports struct {
	store DefectReportStore
	now   func() time.Time
}

func NewDefectReports(store DefectReportStore, now func() time.Time) *DefectReports {
	return &DefectReports{store: store, now: now}
}

// Label labels a measured clip pass or fail for the era it is judged in now; an empty label
// clears it.
func (d *DefectReports) Label(ctx context.Context, clipHash string, label defects.Label) error {
	if label != "" && label != defects.LabelPass && label != defects.LabelFail {
		return fmt.Errorf("label defect: unknown label %q", label)
	}
	return d.store.SetClipDefectLabel(ctx, clipHash, label, d.now())
}

// Report reads every record once. Corrupt records are left out, as unmeasured: the floor measures
// them again.
func (d *DefectReports) Report(ctx context.Context, f DefectReportFilter) (DefectReport, error) {
	st := d.store
	clips, err := st.ListDefectClips(ctx)
	if err != nil {
		return DefectReport{}, fmt.Errorf("defect report: %w", err)
	}
	records, err := st.ListClipDefects(ctx)
	if err != nil && !errors.Is(err, ErrDefectRecordsCorrupt) {
		return DefectReport{}, fmt.Errorf("defect report: %w", err)
	}
	labels, err := st.ListClipDefectLabels(ctx)
	if err != nil {
		return DefectReport{}, fmt.Errorf("defect report: %w", err)
	}
	report := DefectReport{FloorVersion: defects.FloorVersion, Clips: len(clips), Verdicts: map[defects.Verdict]int{}}
	var rows []DefectReportRow
	var scored []defects.LabelledResult
	for _, c := range clips {
		rec, ok := records[c.Hash]
		if !ok {
			report.Unmeasured++
			continue
		}
		report.Verdicts[rec.Verdict]++
		row := DefectReportRow{Clip: c.StoreClip, Record: rec}
		if l, ok := labels[c.Hash]; ok {
			row.Label = &l
			row.LabelCurrent = defects.BandFor(l.Era) == rec.Assessment.Band
		}
		if row.LabelCurrent {
			report.Labelled++
			r := defects.LabelledResult{Era: rec.Era, Label: row.Label.Label, Outcome: rec.Assessment.Outcome}
			if rec.Judgement != nil {
				r.Ruling = rec.Judgement.Ruling
			}
			scored = append(scored, r)
		}
		if (f.Verdict == "" || rec.Verdict == f.Verdict) && (!f.Unlabelled || !row.LabelCurrent) {
			rows = append(rows, row)
		}
	}
	report.Scores = defects.Score(scored)
	slices.SortStableFunc(rows, func(a, b DefectReportRow) int {
		return cmp.Or(cmp.Compare(verdictRank(a.Record.Verdict), verdictRank(b.Record.Verdict)),
			cmp.Compare(a.Clip.Name, b.Clip.Name), cmp.Compare(a.Clip.Hash, b.Clip.Hash))
	})
	report.Total = len(rows)
	lo := min(max(f.Offset, 0), len(rows))
	hi := len(rows)
	if f.Limit > 0 {
		hi = min(lo+f.Limit, len(rows))
	}
	report.Rows = rows[lo:hi]
	return report, nil
}

// verdictRank puts what the floor would retire first, then what it has not settled.
func verdictRank(v defects.Verdict) int {
	switch v {
	case defects.VerdictFail:
		return 0
	case defects.VerdictPending:
		return 1
	case defects.VerdictUndecided:
		return 2
	}
	return 3
}
