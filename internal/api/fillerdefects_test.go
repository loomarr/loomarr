package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
)

type defectReportStore struct {
	clips   []filler.DefectClip
	records map[string]filler.DefectRecord
	labels  map[string]filler.DefectLabel
}

func (s *defectReportStore) ListDefectClips(context.Context) ([]filler.DefectClip, error) {
	return s.clips, nil
}

func (s *defectReportStore) ListClipDefects(context.Context) (map[string]filler.DefectRecord, error) {
	return s.records, nil
}

func (s *defectReportStore) ListClipDefectLabels(context.Context) (map[string]filler.DefectLabel, error) {
	return s.labels, nil
}

func (s *defectReportStore) SetClipDefectLabel(_ context.Context, hash string, label defects.Label, at time.Time) error {
	rec, ok := s.records[hash]
	if !ok {
		return filler.ErrDefectNotMeasured
	}
	if label == "" {
		delete(s.labels, hash)
		return nil
	}
	s.labels[hash] = filler.DefectLabel{Label: label, Era: rec.Era, LabelledAt: at}
	return nil
}

type defectReportWire struct {
	FloorVersion string
	Clips        int
	Unmeasured   int
	Labelled     int
	Total        int
	Verdicts     struct{ Pass, Fail, Undecided, Pending int }
	Scores       []struct {
		Band           string
		Precision      float64
		Failed, Total  int
		JudgeRight     int
		BorderlinePass int
		FalseFails     int
	}
	Rows []struct {
		ClipID, Name, Band, Outcome, Verdict, VerdictReason string
		Era                                                 int
		Findings                                            []struct {
			Check, Outcome string
			Measured, Bar  float64
		}
		Label *struct {
			Label   string
			Era     int
			Current bool
		}
	}
}

func TestFillerDefectReport_AdminReadsAndLabelsMeasuredClips(t *testing.T) {
	soft := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: 9.5}
	a := defects.Assess(soft, 1988)
	verdict, reason := defects.Decide(a, nil)
	st := &defectReportStore{
		records: map[string]filler.DefectRecord{"c1": {ClipHash: "c1", Readings: soft, Era: 1988, Assessment: a,
			Verdict: verdict, VerdictReason: reason, MeasuredAt: time.Unix(1_800_000_000, 0)}},
		labels: map[string]filler.DefectLabel{},
	}
	for _, hash := range []string{"c1", "c2"} {
		c := filler.DefectClip{}
		c.Hash, c.Name = hash, "Clip "+hash
		st.clips = append(st.clips, c)
	}
	harness := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log,
			FillerDefects: filler.NewDefectReports(st, time.Now),
		})
	})

	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/filler/defects", ""},
		{http.MethodPut, "/v1/filler/defects/c1/label", `{"label":"fail"}`},
		{http.MethodDelete, "/v1/filler/defects/c1/label", ""},
	} {
		res := harness.Do(call.method, call.path, memberToken, call.body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("member %s %s = %d, want 403: labels and the report are the maintainer's", call.method, call.path, res.StatusCode)
		}
	}

	res := harness.Do(http.MethodPut, "/v1/filler/defects/c2/label", adminToken, `{"label":"pass"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("labelling an unmeasured clip = %d, want 404", res.StatusCode)
	}
	res = harness.Do(http.MethodPut, "/v1/filler/defects/c1/label", adminToken, `{"label":"maybe"}`)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown label = %d, want 422", res.StatusCode)
	}
	res = harness.Do(http.MethodPut, "/v1/filler/defects/c1/label", adminToken, `{"label":"fail"}`)
	_ = res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("label = %d", res.StatusCode)
	}

	report := readDefectReport(t, harness, "/v1/filler/defects")
	if report.FloorVersion != defects.FloorVersion || report.Clips != 2 || report.Unmeasured != 1 || report.Labelled != 1 ||
		report.Verdicts.Fail != 1 || report.Total != 1 {
		t.Fatalf("report = %+v", report)
	}
	row := report.Rows[0]
	if row.ClipID != "c1" || row.Band != "tape" || row.Verdict != "fail" || row.Label == nil || row.Label.Label != "fail" ||
		row.Label.Era != 1988 || !row.Label.Current || len(row.Findings) == 0 {
		t.Fatalf("row = %+v", row)
	}
	if len(report.Scores) != 1 || report.Scores[0].Band != "tape" || report.Scores[0].Failed != 1 || report.Scores[0].Precision != 1 {
		t.Fatalf("scores = %+v", report.Scores)
	}
	if unlabelled := readDefectReport(t, harness, "/v1/filler/defects?unlabelled=true"); unlabelled.Total != 0 {
		t.Fatalf("unlabelled rows = %d, want 0", unlabelled.Total)
	}

	res = harness.Do(http.MethodDelete, "/v1/filler/defects/c1/label", adminToken, "")
	_ = res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("clear label = %d", res.StatusCode)
	}
	if cleared := readDefectReport(t, harness, "/v1/filler/defects?verdict=fail"); cleared.Labelled != 0 || cleared.Rows[0].Label != nil {
		t.Fatalf("after clearing = %+v", cleared)
	}
}

func readDefectReport(t *testing.T, harness *apiHarness, path string) defectReportWire {
	t.Helper()
	res := harness.Do(http.MethodGet, path, adminToken, "")
	raw, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s %v", path, res.StatusCode, raw, err)
	}
	var out defectReportWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
