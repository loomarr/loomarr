//go:build ffmpeg

package mediatools_test

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The floor is scored on the committed synthetic corpus (#2034 step 4c): per era band, it must
// fail outright no more clips labelled pass than the corpus allows, and must fail or send to the
// judge at least its share of the clips labelled fail. The table is logged for calibration.
func TestDefectCorpus_ScoresEachEraBand(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffmpeg")
	}
	corpus, err := defects.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var results []defects.LabelledResult
	var table strings.Builder
	fmt.Fprintf(&table, "%s, floor %s\n", corpus.Version, defects.FloorVersion)
	for _, c := range corpus.Cases {
		file := testkit.FillerDefectClip(t, dir, testkit.DefectDamage(c.Damage), c.Severity)
		r, err := mediatools.MeasureDefects(context.Background(), ffmpeg, file, 640, 480, true)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&table, "%-9s %5g  block %5.1f blur %5.2f tout %.4f idet %.2f window %.3f ssim2 %.4f ssim4 %.4f flat %.1f |",
			c.Damage, c.Severity, r.BlockMean, r.BlurMean, r.OutlierShare, r.InterlacedShare,
			measured(r, defects.CheckWindowbox), r.UpscaleSSIM, r.UpscaleSSIM4, r.FlatFactor)
		for _, era := range corpus.Eras {
			label := c.Labels[strconv.Itoa(era)]
			outcome := defects.Assess(r, era).Outcome
			results = append(results, defects.LabelledResult{Era: era, Label: label, Outcome: outcome})
			mark := " "
			if (outcome == defects.Fail && label == defects.LabelPass) || (outcome == defects.Pass && label == defects.LabelFail) {
				mark = "!"
			}
			fmt.Fprintf(&table, " %d %s→%s%s", era, label, outcome, mark)
		}
		table.WriteString("\n")
	}
	t.Log("\n" + table.String())

	falseFails := 0
	for _, s := range defects.Score(results) {
		t.Logf("%-4s precision %.2f caught %.2f (failed %d, judged %d, missed %d, false fails %d, borderline passes %d)",
			s.Band, s.Precision(), s.Caught(), s.Failed, s.Judged, s.Missed, s.FalseFails, s.BorderlinePass)
		falseFails += s.FalseFails
		if s.Caught() < corpus.Thresholds.MinCaught {
			t.Errorf("%s band caught %.2f of clips labelled fail, want at least %.2f", s.Band, s.Caught(), corpus.Thresholds.MinCaught)
		}
	}
	if falseFails > corpus.Thresholds.MaxFalseFails {
		t.Errorf("%d clips labelled pass were failed outright, want at most %d", falseFails, corpus.Thresholds.MaxFalseFails)
	}
}

// measured is what one check read, as the floor reads it.
func measured(r defects.Readings, check defects.Check) float64 {
	for _, f := range defects.Assess(r, 2002).Findings {
		if f.Check == check {
			return f.Measured
		}
	}
	return 0
}
