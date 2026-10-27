package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/store"
)

// The report reads the real store through the floor's adapter: a label is stamped with the era its
// record was judged against, and a clip the floor has not measured cannot be labelled.
func TestFillerDefectReports_LabelThroughTheStore(t *testing.T) {
	ctx := t.Context()
	st, err := fillerstore.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	at := time.Unix(1_800_000_000, 0).UTC()
	for _, hash := range []string{"measured", "unmeasured"} {
		c := store.Clip{}
		c.Hash, c.Path, c.Name, c.Kind, c.Era = hash, hash+".mp4", hash, filler.Commercial, 1988
		if err := st.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	r := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 9.5}
	a := defects.Assess(r, 1988)
	verdict, reason := defects.Decide(a, nil)
	if err := st.UpsertClipDefects(ctx, store.ClipDefectRecord{ClipHash: "measured", Readings: r, Era: 1988,
		Assessment: a, Verdict: verdict, VerdictReason: reason, MeasuredAt: at}); err != nil {
		t.Fatal(err)
	}
	reports := filler.NewDefectReports(fillerDefectStoreAdapter{st}, func() time.Time { return at })

	if err := reports.Label(ctx, "unmeasured", defects.LabelPass); !errors.Is(err, filler.ErrDefectNotMeasured) {
		t.Fatalf("labelling an unmeasured clip = %v, want ErrDefectNotMeasured", err)
	}
	if err := reports.Label(ctx, "measured", defects.LabelFail); err != nil {
		t.Fatal(err)
	}
	report, err := reports.Report(ctx, filler.DefectReportFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Unmeasured != 1 || report.Labelled != 1 || len(report.Rows) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if got, want := *report.Rows[0].Label, (filler.DefectLabel{Label: defects.LabelFail, Era: 1988, LabelledAt: at}); got != want {
		t.Fatalf("label = %+v, want %+v", got, want)
	}
}
