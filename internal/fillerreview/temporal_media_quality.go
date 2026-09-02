package fillerreview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/mediatools"
)

const (
	TemporalMediaQualitySchemaVersion   = 1
	TemporalMediaQualityContractVersion = "filler-temporal-media-quality-v1"
	mediaQualityContinue                = "continue"
	mediaQualityReview                  = "review"
	mediaQualityReject                  = "reject"
)

type TemporalMediaQualityConfig struct {
	HumanPackagePath     string
	HumanPrivateMapPath  string
	HumanAssessmentPath  string
	HumanAttestationPath string
	FFmpegPath           string
	MeasuredAt           time.Time
	ExpectedCases        int
	OutputPath           string
	Progress             func(done, total int, evidenceAlias string)
}

type TemporalMediaQualityReport struct {
	SchemaVersion              int                        `json:"schemaVersion"`
	ContractVersion            string                     `json:"contractVersion"`
	MeasuredAt                 time.Time                  `json:"measuredAt"`
	HumanPackageSHA256         string                     `json:"humanPackageSha256"`
	HumanPrivateMapSHA256      string                     `json:"humanPrivateMapSha256"`
	HumanAssessmentSetSHA256   string                     `json:"humanAssessmentSetSha256"`
	HumanAttestationFileSHA256 string                     `json:"humanAttestationFileSha256"`
	EvidenceManifestSHA256     string                     `json:"evidenceManifestSha256"`
	SelectionSHA256            string                     `json:"selectionSha256"`
	Cases                      int                        `json:"cases"`
	HumanUnusableCases         int                        `json:"humanUnusableCases"`
	OperationalFailures        int                        `json:"operationalFailures"`
	PolicyRejectCases          int                        `json:"policyRejectCases"`
	PolicyReviewCases          int                        `json:"policyReviewCases"`
	PolicyContinueCases        int                        `json:"policyContinueCases"`
	HumanUnusableHeld          int                        `json:"humanUnusableHeld"`
	HumanUnusableContinued     int                        `json:"humanUnusableContinued"`
	OtherHumanLabelsHeld       int                        `json:"otherHumanLabelsHeld"`
	OtherHumanLabelsContinued  int                        `json:"otherHumanLabelsContinued"`
	CaseMeasurements           []TemporalMediaQualityCase `json:"caseMeasurements"`
	ProductionAdmissionAllowed bool                       `json:"productionAdmissionAllowed"`
	NextAction                 string                     `json:"nextAction"`
}

type TemporalMediaQualityCase struct {
	EvidenceAlias      string                   `json:"evidenceAlias"`
	HumanUnit          fillereval.UnitKind      `json:"humanUnit"`
	DurationMS         int64                    `json:"durationMs"`
	HadAudio           bool                     `json:"hadAudio"`
	Measurement        *mediatools.MediaQuality `json:"measurement,omitempty"`
	BlackPercent       int64                    `json:"blackPercent,omitempty"`
	SilencePercent     int64                    `json:"silencePercent,omitempty"`
	LongestBlackMS     int64                    `json:"longestBlackMs,omitempty"`
	LongestSilenceMS   int64                    `json:"longestSilenceMs,omitempty"`
	LongestFreezeMS    int64                    `json:"longestFreezeMs,omitempty"`
	PolicyVerdict      string                   `json:"policyVerdict,omitempty"`
	PolicyReason       filler.RejectReason      `json:"policyReason,omitempty"`
	PolicyDetail       string                   `json:"policyDetail,omitempty"`
	OperationalFailure string                   `json:"operationalFailure,omitempty"`
}

type temporalMediaQualityInput struct {
	EvidenceAlias string
	HumanUnit     fillereval.UnitKind
	DurationMS    int64
	HadAudio      bool
	Path          string
}

type temporalQualityInspector func(context.Context, string, int64, bool) (mediatools.MediaQuality, error)

// MeasureTemporalMediaQuality applies the production decoder-quality policy to the exact locked
// human panel. It does not reinterpret unsafe or semantically invalid content as media damage.
func MeasureTemporalMediaQuality(ctx context.Context, config TemporalMediaQualityConfig) (TemporalMediaQualityReport, error) {
	loaded, inputs, err := loadTemporalMediaQuality(config)
	if err != nil {
		return TemporalMediaQualityReport{}, err
	}
	inspector := func(ctx context.Context, path string, durationMS int64, hadAudio bool) (mediatools.MediaQuality, error) {
		return mediatools.InspectQuality(ctx, config.FFmpegPath, path, durationMS, hadAudio)
	}
	probe := filler.FFprobeNextTo(config.FFmpegPath)
	report := TemporalMediaQualityReport{
		SchemaVersion: TemporalMediaQualitySchemaVersion, ContractVersion: TemporalMediaQualityContractVersion,
		MeasuredAt: config.MeasuredAt.UTC(), HumanPackageSHA256: loaded.packageSHA,
		HumanPrivateMapSHA256: loaded.mapSHA, HumanAssessmentSetSHA256: loaded.assessmentSHA,
		HumanAttestationFileSHA256: loaded.attestationFileSHA,
		EvidenceManifestSHA256:     loaded.pack.EvidenceManifestSHA256, SelectionSHA256: loaded.pack.SelectionSHA256,
		Cases: len(inputs), ProductionAdmissionAllowed: false,
		NextAction: "separate_media_quality_ground_truth_before_threshold_changes",
	}
	for index, input := range inputs {
		probed, probeErr := probe(ctx, input.Path)
		var measurement TemporalMediaQualityCase
		switch {
		case probeErr != nil:
			measurement = TemporalMediaQualityCase{EvidenceAlias: input.EvidenceAlias, HumanUnit: input.HumanUnit, DurationMS: input.DurationMS, OperationalFailure: probeErr.Error()}
		case probed.NoVideo:
			measurement = temporalMediaQualityProbeRejection(input, filler.ReasonNoVideo, "The review artifact has no video stream.")
		case probed.Silent:
			measurement = temporalMediaQualityProbeRejection(input, filler.ReasonNoAudio, "The review artifact has no audio stream.")
		default:
			input.HadAudio = !probed.Silent
			measurement = measureTemporalMediaQualityCase(ctx, input, inspector)
		}
		report.CaseMeasurements = append(report.CaseMeasurements, measurement)
		accumulateTemporalMediaQuality(&report, measurement)
		if config.Progress != nil {
			config.Progress(index+1, len(inputs), input.EvidenceAlias)
		}
	}
	return report, nil
}

func temporalMediaQualityProbeRejection(input temporalMediaQualityInput, reason filler.RejectReason, detail string) TemporalMediaQualityCase {
	return TemporalMediaQualityCase{
		EvidenceAlias: input.EvidenceAlias, HumanUnit: input.HumanUnit, DurationMS: input.DurationMS,
		PolicyVerdict: mediaQualityReject, PolicyReason: reason, PolicyDetail: detail,
	}
}

func measureTemporalMediaQualityCase(ctx context.Context, input temporalMediaQualityInput, inspect temporalQualityInspector) TemporalMediaQualityCase {
	result := TemporalMediaQualityCase{EvidenceAlias: input.EvidenceAlias, HumanUnit: input.HumanUnit, DurationMS: input.DurationMS, HadAudio: input.HadAudio}
	quality, err := inspect(ctx, input.Path, input.DurationMS, input.HadAudio)
	if err != nil {
		result.OperationalFailure = err.Error()
		return result
	}
	result.Measurement = &quality
	blackTotal, longestBlack := temporalQualityIntervalFacts(quality.Black)
	silenceTotal, longestSilence := temporalQualityIntervalFacts(quality.Silence)
	_, longestFreeze := temporalQualityIntervalFacts(quality.Freeze)
	result.LongestBlackMS = longestBlack
	result.LongestSilenceMS = longestSilence
	result.LongestFreezeMS = longestFreeze
	if quality.DurationMs > 0 {
		result.BlackPercent = blackTotal * 100 / quality.DurationMs
		result.SilencePercent = silenceTotal * 100 / quality.DurationMs
	}
	verdict, reason, detail := filler.EvaluateMediaQuality(quality)
	result.PolicyVerdict, result.PolicyReason, result.PolicyDetail = temporalMediaQualityVerdictName(verdict), reason, detail
	return result
}

func temporalMediaQualityVerdictName(verdict filler.Verdict) string {
	switch verdict {
	case filler.VerdictReject:
		return mediaQualityReject
	case filler.VerdictReview:
		return mediaQualityReview
	default:
		return mediaQualityContinue
	}
}

func temporalQualityIntervalFacts(spans []mediatools.Interval) (total, longest int64) {
	for _, span := range spans {
		if duration := span.EndMs - span.StartMs; duration > 0 {
			total += duration
			if duration > longest {
				longest = duration
			}
		}
	}
	return total, longest
}

func accumulateTemporalMediaQuality(report *TemporalMediaQualityReport, item TemporalMediaQualityCase) {
	unusable := item.HumanUnit == fillereval.UnitUnusable
	if unusable {
		report.HumanUnusableCases++
	}
	if item.OperationalFailure != "" {
		report.OperationalFailures++
		return
	}
	held := item.PolicyVerdict == mediaQualityReject || item.PolicyVerdict == mediaQualityReview
	switch item.PolicyVerdict {
	case mediaQualityReject:
		report.PolicyRejectCases++
	case mediaQualityReview:
		report.PolicyReviewCases++
	default:
		report.PolicyContinueCases++
	}
	if unusable && held {
		report.HumanUnusableHeld++
	} else if unusable {
		report.HumanUnusableContinued++
	} else if held {
		report.OtherHumanLabelsHeld++
	} else {
		report.OtherHumanLabelsContinued++
	}
}

func PublishTemporalMediaQuality(ctx context.Context, config TemporalMediaQualityConfig) (TemporalMediaQualityReport, string, error) {
	if strings.TrimSpace(config.OutputPath) == "" {
		return TemporalMediaQualityReport{}, "", fmt.Errorf("temporal media quality output path is required")
	}
	report, err := MeasureTemporalMediaQuality(ctx, config)
	if err != nil {
		return TemporalMediaQualityReport{}, "", err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return TemporalMediaQualityReport{}, "", err
	}
	raw = append(raw, '\n')
	if err := writeTemporalTruthNew(config.OutputPath, raw, 0o600); err != nil {
		return TemporalMediaQualityReport{}, "", fmt.Errorf("publish temporal media quality: %w", err)
	}
	return report, hashBytes(raw), nil
}
