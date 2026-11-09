package defectfloor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// #2121: the bytes that would replace an airing clip are held to the floor before the switch, on
// the staged file itself, with the judge for a borderline picture. Nothing is saved: the switch
// commits the record, and a refusal leaves the airing clip's own.
func TestCheckReplacement_JudgesTheStagedBytesAndSavesNothing(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0).UTC()
	const staged = "/clips/.transcode/replacement.mp4"
	clip := clipstore.Clip{Clip: clips.Clip{Hash: "airing", Path: "ai/ri/airing.mp4", Era: 1988, Role: clips.Commercial}}
	readings := func(blur float64) defects.Readings {
		return defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: blur}
	}
	for _, tc := range []struct {
		name     string
		blur     float64
		judge    DefectJudge
		measured error
		want     defects.Verdict
		wantErr  bool
	}{
		{name: "clear picture", blur: 4, want: defects.VerdictPass},
		{name: "borderline, judge passes it", blur: 8.5, judge: &scriptedJudge{model: "m1"}, want: defects.VerdictPass},
		{name: "borderline, judge fails it", blur: 8.5, want: defects.VerdictFail,
			judge: &scriptedJudge{model: "m1", rulings: map[string]func() (defects.Judgement, error){"airing": ruling(defects.RulingFail, "m1")}}},
		{name: "borderline, no judge configured", blur: 8.5, want: defects.VerdictFail},
		{name: "borderline, judge unreachable", blur: 8.5, wantErr: true,
			judge: &scriptedJudge{model: "m1", rulings: map[string]func() (defects.Judgement, error){
				"airing": func() (defects.Judgement, error) { return defects.Judgement{}, errors.New("connection refused") }}}},
		{name: "unmeasurable", measured: errors.New("decoder gave up"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &defectMemStore{records: map[string]DefectRecord{}}
			var measuredFile string
			measure := func(_ context.Context, file string, _, _ int, _ bool) (defects.Readings, error) {
				measuredFile = file
				return readings(tc.blur), tc.measured
			}
			probe := func(context.Context, string) (tools.Probed, error) {
				return tools.Probed{Width: 640, Height: 480, DurationMs: 30_000}, nil
			}
			floor := NewDefectFloor(st, probe, measure, someFingerprint, tc.judge, "/clips", func() time.Time { return t0 }, nil)

			rec, err := floor.CheckReplacement(context.Background(), clip, staged)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("record %+v, want an error", rec)
				}
			} else if err != nil || rec.Verdict != tc.want || rec.ClipHash != clip.Hash || rec.Era != clip.Era {
				t.Fatalf("record %+v, %v; want verdict %s", rec, err, tc.want)
			}
			if measuredFile != staged {
				t.Errorf("measured %q, want the staged replacement", measuredFile)
			}
			if len(st.records) != 0 {
				t.Errorf("the check saved %+v", st.records)
			}
		})
	}
}
