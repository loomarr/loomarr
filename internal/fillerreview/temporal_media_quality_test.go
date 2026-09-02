package fillerreview

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestMeasureTemporalMediaQualityCaseUsesProductionPolicy(t *testing.T) {
	input := temporalMediaQualityInput{EvidenceAlias: "evidence-a", HumanUnit: fillereval.UnitUnusable, DurationMS: 30_000, HadAudio: true, Path: "review.mp4"}
	got := measureTemporalMediaQualityCase(context.Background(), input, func(_ context.Context, path string, durationMS int64, hadAudio bool) (mediatools.MediaQuality, error) {
		if path != input.Path || durationMS != input.DurationMS || !hadAudio {
			t.Fatal("inspector did not receive the bound case")
		}
		return mediatools.MediaQuality{DurationMs: durationMS, Black: []mediatools.Interval{{StartMs: 0, EndMs: 6_000}}, Silence: []mediatools.Interval{{StartMs: 0, EndMs: 1_500}}, Freeze: []mediatools.Interval{{StartMs: 3_000, EndMs: 14_000}}}, nil
	})
	if got.PolicyVerdict != mediaQualityReview || got.BlackPercent != 20 || got.SilencePercent != 5 || got.LongestBlackMS != 6_000 || got.LongestSilenceMS != 1_500 || got.LongestFreezeMS != 11_000 {
		t.Fatalf("unexpected measurement: %#v", got)
	}
}

func TestAccumulateTemporalMediaQualitySeparatesHumanLabels(t *testing.T) {
	var report TemporalMediaQualityReport
	items := []TemporalMediaQualityCase{
		{HumanUnit: fillereval.UnitUnusable, PolicyVerdict: mediaQualityReview},
		{HumanUnit: fillereval.UnitUnusable, PolicyVerdict: mediaQualityContinue},
		{HumanUnit: fillereval.UnitStandalone, PolicyVerdict: mediaQualityReject},
		{HumanUnit: fillereval.UnitStandalone, PolicyVerdict: mediaQualityContinue},
		{HumanUnit: fillereval.UnitUnclear, OperationalFailure: "decode failed"},
	}
	for _, item := range items {
		accumulateTemporalMediaQuality(&report, item)
	}
	if report.HumanUnusableCases != 2 || report.HumanUnusableHeld != 1 || report.HumanUnusableContinued != 1 || report.OtherHumanLabelsHeld != 1 || report.OtherHumanLabelsContinued != 1 || report.OperationalFailures != 1 || report.PolicyRejectCases != 1 || report.PolicyReviewCases != 1 || report.PolicyContinueCases != 2 {
		t.Fatalf("unexpected summary: %#v", report)
	}
}

func TestMeasureTemporalMediaQualityCaseRecordsOperationalFailure(t *testing.T) {
	want := errors.New("decode failed")
	got := measureTemporalMediaQualityCase(context.Background(), temporalMediaQualityInput{EvidenceAlias: "evidence-a", DurationMS: 1}, func(context.Context, string, int64, bool) (mediatools.MediaQuality, error) {
		return mediatools.MediaQuality{}, want
	})
	if got.OperationalFailure != want.Error() || got.Measurement != nil || got.PolicyVerdict != "" {
		t.Fatalf("unexpected failure result: %#v", got)
	}
}

func TestTemporalMediaQualityProbeRejectionUsesProductionReason(t *testing.T) {
	got := temporalMediaQualityProbeRejection(temporalMediaQualityInput{EvidenceAlias: "evidence-a", HumanUnit: fillereval.UnitUnusable, DurationMS: 30_000}, filler.ReasonNoAudio, "no audio")
	if got.PolicyVerdict != mediaQualityReject || got.PolicyReason != filler.ReasonNoAudio || got.PolicyDetail != "no audio" || got.OperationalFailure != "" {
		t.Fatalf("unexpected probe rejection: %#v", got)
	}
}

func TestTemporalMediaQualityContractVersionsTogether(t *testing.T) {
	if TemporalMediaQualitySchemaVersion != 2 || TemporalMediaQualityContractVersion != "filler-temporal-media-quality-v2" {
		t.Fatalf("media quality schema and contract drifted: %d %q", TemporalMediaQualitySchemaVersion, TemporalMediaQualityContractVersion)
	}
}
