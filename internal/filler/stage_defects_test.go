package filler

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/defects"
)

type fakeDefectVerdicts map[string]DefectRecord

func (f fakeDefectVerdicts) GetClipDefects(_ context.Context, hash string) (DefectRecord, bool, error) {
	r, ok := f[hash]
	return r, ok, nil
}

// #2036: the defects rung admits a copy the floor passed, refuses one it failed with the failing
// checks as the detail, and holds every other clip until the floor has a current verdict.
func TestDefectsStage_ActsOnTheFloorsCurrentVerdict(t *testing.T) {
	current := func(v defects.Verdict, findings ...defects.Finding) DefectRecord {
		return DefectRecord{
			Readings:   defects.Readings{Version: defects.MeasurementVersion},
			Assessment: defects.Assessment{Version: defects.FloorVersion, Findings: findings},
			Era:        1988, Verdict: v, VerdictReason: "detectors",
		}
	}
	stale := current(defects.VerdictPass)
	stale.Assessment.Version = "floor-v1"
	otherEra := current(defects.VerdictPass)
	otherEra.Era = 2005
	cases := []struct {
		name   string
		rec    *DefectRecord
		want   Verdict
		detail string
	}{
		{"unmeasured", nil, VerdictDefer, ""},
		{"under an older floor", &stale, VerdictDefer, ""},
		{"judged against another era", &otherEra, VerdictDefer, ""},
		{"waiting for the judge", ptr(current(defects.VerdictPending)), VerdictDefer, ""},
		{"passed", ptr(current(defects.VerdictPass)), VerdictContinue, ""},
		{"too little picture", ptr(current(defects.VerdictUndecided)), VerdictContinue, ""},
		{"failed", ptr(current(defects.VerdictFail,
			defects.Finding{Check: defects.CheckBlocks, Outcome: defects.Fail, Measured: 61, Bar: 50},
			defects.Finding{Check: defects.CheckBlur, Outcome: defects.Pass, Measured: 2, Bar: 9})),
			VerdictReject, "blocks 61; bar 50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdicts := fakeDefectVerdicts{}
			if tc.rec != nil {
				verdicts["h"] = *tc.rec
			}
			out, err := NewDefectsStage(verdicts).Run(t.Context(), StoreClip{Clip: Clip{Hash: "h", Era: 1988}})
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict != tc.want || out.Detail != tc.detail {
				t.Fatalf("verdict %v detail %q, want %v %q", out.Verdict, out.Detail, tc.want, tc.detail)
			}
			if tc.want == VerdictReject && out.Reason != ReasonDefect {
				t.Fatalf("reason %q, want %q", out.Reason, ReasonDefect)
			}
		})
	}
}

func TestDefectsStage_SkipsARecordedBreak(t *testing.T) {
	if ok, _ := NewDefectsStage(fakeDefectVerdicts{}).Applies(t.Context(), StoreClip{Clip: Clip{Hash: "r", IsComposite: true}}); ok {
		t.Fatal("a composite recording was put to the floor; its segments are judged instead")
	}
}

func ptr[T any](v T) *T { return &v }
