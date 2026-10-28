package filler

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
)

type defectReportMemStore struct {
	defectMemStore
	labels  map[string]DefectLabel
	corrupt bool
	set     []string
}

func (m *defectReportMemStore) ListClipDefects(ctx context.Context) (map[string]DefectRecord, error) {
	out, _ := m.defectMemStore.ListClipDefects(ctx)
	if m.corrupt {
		return out, ErrDefectRecordsCorrupt
	}
	return out, nil
}

func (m *defectReportMemStore) ListClipDefectLabels(context.Context) (map[string]DefectLabel, error) {
	return m.labels, nil
}

func (m *defectReportMemStore) SetClipDefectLabel(_ context.Context, hash string, label defects.Label, at time.Time) error {
	m.set = append(m.set, hash+"="+string(label)+"@"+at.Format(time.RFC3339))
	return nil
}

func reportRecord(hash string, era int, blur float64, j *defects.Judgement) DefectRecord {
	r := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: blur}
	a := defects.Assess(r, era)
	verdict, reason := defects.Decide(a, j)
	return DefectRecord{ClipHash: hash, Readings: r, Era: era, Assessment: a, Judgement: j, Verdict: verdict, VerdictReason: reason}
}

// The report counts every record, scores only labels given for the band a clip is judged in now,
// and pages what the floor would retire first.
func TestDefectReport_CountsScoresAndPagesWorstFirst(t *testing.T) {
	ruledPass := &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingPass, Model: "m"}
	pending := reportRecord("pending", 1988, 8.5, nil)
	pending.Verdict = defects.VerdictPending
	st := &defectReportMemStore{
		defectMemStore: defectMemStore{records: map[string]DefectRecord{
			"soft-tape":  reportRecord("soft-tape", 1988, 9.5, nil),    // fail, labelled fail
			"clean-hd":   reportRecord("clean-hd", 2015, 5, nil),       // pass, labelled pass
			"judged":     reportRecord("judged", 1988, 8.5, ruledPass), // borderline, judge passed a fail
			"pending":    pending,                                      // borderline, not yet ruled
			"re-dated":   reportRecord("re-dated", 2015, 9.5, nil),     // fail; labelled when it was tape
			"orphan-rec": reportRecord("orphan-rec", 2015, 5, nil),     // no catalog clip: not reported
		}},
		labels: map[string]DefectLabel{
			"soft-tape": {Label: defects.LabelFail, Era: 1988},
			"clean-hd":  {Label: defects.LabelPass, Era: 2012},
			"judged":    {Label: defects.LabelFail, Era: 1990},
			"re-dated":  {Label: defects.LabelPass, Era: 1988},
		},
	}
	for _, name := range []string{"soft-tape", "clean-hd", "judged", "pending", "re-dated", "unmeasured"} {
		c := DefectClip{}
		c.Hash, c.Name = name, name
		st.clips = append(st.clips, c)
	}
	reports := NewDefectReports(st, time.Now)

	r, err := reports.Report(context.Background(), DefectReportFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Clips != 6 || r.Unmeasured != 1 || r.Labelled != 3 || r.Total != 5 {
		t.Fatalf("report = %d clips, %d unmeasured, %d labelled, %d rows; want 6, 1, 3, 5", r.Clips, r.Unmeasured, r.Labelled, r.Total)
	}
	if want := map[defects.Verdict]int{defects.VerdictFail: 2, defects.VerdictPass: 2, defects.VerdictPending: 1}; len(r.Verdicts) != len(want) ||
		r.Verdicts[defects.VerdictFail] != 2 || r.Verdicts[defects.VerdictPass] != 2 || r.Verdicts[defects.VerdictPending] != 1 {
		t.Fatalf("verdicts = %v, want %v", r.Verdicts, want)
	}
	want := []defects.BandScore{
		{Band: defects.BandTape, Failed: 1, Judged: 1, JudgeWrong: 1, Total: 2},
		{Band: defects.BandHD, Total: 1},
	}
	if len(r.Scores) != len(want) || r.Scores[0] != want[0] || r.Scores[1] != want[1] {
		t.Fatalf("scores = %+v, want %+v: the re-dated clip's label answers an old question", r.Scores, want)
	}
	var order []string
	for _, row := range r.Rows {
		order = append(order, row.Clip.Hash)
	}
	if got, want := order, []string{"re-dated", "soft-tape", "pending", "clean-hd", "judged"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v: fails, then pending, then passes, by name", got, want)
	}
	if row := r.Rows[0]; row.Label == nil || row.LabelCurrent {
		t.Fatalf("re-dated row = %+v, want its label shown but not current", row)
	}

	page, err := reports.Report(context.Background(), DefectReportFilter{Unlabelled: true, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Rows) != 1 || page.Rows[0].Clip.Hash != "pending" {
		t.Fatalf("unlabelled page 2 = %d total, %+v; want re-dated then pending, page 2 is pending", page.Total, page.Rows)
	}
	if beyond, _ := reports.Report(context.Background(), DefectReportFilter{Offset: 99}); len(beyond.Rows) != 0 || beyond.Total != 5 {
		t.Fatalf("offset past the end = %d rows of %d", len(beyond.Rows), beyond.Total)
	}
	fails, _ := reports.Report(context.Background(), DefectReportFilter{Verdict: defects.VerdictFail})
	if fails.Total != 2 || fails.Labelled != 3 {
		t.Fatalf("fail filter = %d rows, %d labelled; the filter narrows rows, never the scores", fails.Total, fails.Labelled)
	}

	st.corrupt = true
	if _, err := reports.Report(context.Background(), DefectReportFilter{}); err != nil {
		t.Fatalf("a corrupt record failed the whole report: %v", err)
	}
}

func TestDefectReport_LabelValidatesAndStamps(t *testing.T) {
	st := &defectReportMemStore{}
	at := time.Unix(1_800_000_000, 0).UTC()
	reports := NewDefectReports(st, func() time.Time { return at })
	if err := reports.Label(context.Background(), "c1", "maybe"); err == nil {
		t.Fatal("an unknown label was accepted")
	}
	if err := errors.Join(reports.Label(context.Background(), "c1", defects.LabelFail),
		reports.Label(context.Background(), "c1", "")); err != nil {
		t.Fatal(err)
	}
	if want := []string{"c1=fail@" + at.Format(time.RFC3339), "c1=@" + at.Format(time.RFC3339)}; !slices.Equal(st.set, want) {
		t.Fatalf("labels written = %v, want %v", st.set, want)
	}
}
