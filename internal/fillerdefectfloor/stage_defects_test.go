package fillerdefectfloor

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/filler"
)

type fakeDefectVerdicts map[string]DefectRecord

func (f fakeDefectVerdicts) GetClipDefects(_ context.Context, hash string) (DefectRecord, bool, error) {
	r, ok := f[hash]
	return r, ok, nil
}

// #2036: the defects rung admits a copy the floor passed, refuses one that definitively failed
// with the failing checks as the detail, sends one no judge could rule on to review, and holds
// every other clip until the floor has a current verdict.
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
	failed := current(defects.VerdictFail,
		defects.Finding{Check: defects.CheckBlocks, Outcome: defects.Fail, Measured: 61, Bar: 50},
		defects.Finding{Check: defects.CheckBlur, Outcome: defects.Pass, Measured: 2, Bar: 9})
	failed.Assessment.Outcome = defects.Fail
	borderline := current(defects.VerdictFail,
		defects.Finding{Check: defects.CheckBlur, Outcome: defects.Borderline, Measured: 10, Bar: 9})
	borderline.Assessment.Outcome = defects.Borderline
	unjudged, abstained, ruled := borderline, borderline, borderline
	unjudged.Judgement = &defects.Judgement{Ruling: defects.RulingNoJudge}
	abstained.Judgement = &defects.Judgement{Ruling: defects.RulingAbstain}
	ruled.Judgement = &defects.Judgement{Ruling: defects.RulingFail, Reason: "smeared"}
	ruled.VerdictReason = "judge: smeared"
	cases := []struct {
		name   string
		rec    *DefectRecord
		want   filler.Verdict
		detail string
	}{
		{"unmeasured", nil, filler.VerdictDefer, ""},
		{"under an older floor", &stale, filler.VerdictDefer, ""},
		{"judged against another era", &otherEra, filler.VerdictDefer, ""},
		{"waiting for the judge", ptr(current(defects.VerdictPending)), filler.VerdictDefer, ""},
		{"passed", ptr(current(defects.VerdictPass)), filler.VerdictContinue, ""},
		{"too little picture", ptr(current(defects.VerdictUndecided)), filler.VerdictContinue, ""},
		{"failed by the detectors", &failed, filler.VerdictReject, "blocks 61; bar 50"},
		{"failed by the judge", &ruled, filler.VerdictReject, "judge: smeared"},
		// Failed only because nobody could look: the operator's call, never refused and never an
		// endless wait.
		{"borderline with no judge", &unjudged, filler.VerdictReview, ""},
		{"borderline the judge abstained on", &abstained, filler.VerdictReview, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdicts := fakeDefectVerdicts{}
			if tc.rec != nil {
				verdicts["h"] = *tc.rec
			}
			out, err := NewDefectsStage(verdicts).Run(t.Context(), filler.StoreClip{Clip: filler.Clip{Hash: "h", Era: 1988}})
			if err != nil {
				t.Fatal(err)
			}
			if out.Verdict != tc.want || out.Detail != tc.detail {
				t.Fatalf("verdict %v detail %q, want %v %q", out.Verdict, out.Detail, tc.want, tc.detail)
			}
			if tc.want == filler.VerdictReject && out.Reason != filler.ReasonDefect {
				t.Fatalf("reason %q, want %q", out.Reason, filler.ReasonDefect)
			}
		})
	}
}

// A clip in the conveyor is not placed until the score rung, so it reads as not playable; the rung
// must still judge it, or nothing new is ever held to the floor.
func TestDefectsStage_AppliesToAHeldClipNotYetPlaced(t *testing.T) {
	c := filler.StoreClip{Clip: filler.Clip{Hash: "h", Held: true, Placement: filler.PlacementNotPlayable, Kind: filler.Unclassified}}
	if ok, why := NewDefectsStage(fakeDefectVerdicts{}).Applies(t.Context(), c); !ok {
		t.Fatalf("a new arrival skipped the floor: %s", why)
	}
}

func TestDefectsStage_SkipsARecordedBreak(t *testing.T) {
	if ok, _ := NewDefectsStage(fakeDefectVerdicts{}).Applies(t.Context(), filler.StoreClip{Clip: filler.Clip{Hash: "r", IsComposite: true}}); ok {
		t.Fatal("a composite recording was put to the floor; its segments are judged instead")
	}
}

func ptr[T any](v T) *T { return &v }
