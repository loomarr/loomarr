package fillerscreening

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerairworthiness"
)

func screeningProfileFixture(axis filler.SegmentScreeningAxis, digit string) SegmentScreeningAxisProfile {
	profile := SegmentScreeningAxisProfile{
		Axis: axis, EvidenceContract: "axis-evidence-v1",
		PolicySHA256: strings.Repeat(digit, 64), CertificationSHA256: strings.Repeat("a", 64), ImplementationSHA256: strings.Repeat("b", 64),
	}
	airworthiness, safety := airworthinessAxis(axis)
	if safety {
		for _, flag := range fillerairworthiness.Vocabulary() {
			if slices.Contains(fillerairworthiness.AxesForFlag(flag), airworthiness) {
				profile.CertifiedSuitabilityFlags = append(profile.CertifiedSuitabilityFlags, flag)
			}
		}
	}
	return profile
}

func screeningSubjectFixture(t *testing.T) filler.SegmentScreeningSubject {
	t.Helper()
	manifest := screeningSubjectManifest(t)
	subject, err := filler.NewSegmentScreeningSubject(manifest.Playback.Asset.ClipHash, filler.SidecarTags{MediaAssets: &manifest})
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func screeningChildSubjectFixture(t *testing.T) filler.SegmentScreeningSubject {
	t.Helper()
	tags := screeningChildTagsFixture(t)
	subject, err := filler.NewSegmentScreeningSubject(tags.MediaAssets.Playback.Asset.ClipHash, tags)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func screeningChildTagsFixture(t *testing.T) filler.SidecarTags {
	t.Helper()
	manifest := screeningSubjectManifest(t)
	lineage := filler.ConditioningLineage{
		ChildHash: manifest.SourceMaster.ClipHash, ParentHash: strings.Repeat("7", 64),
		ParentAssetRole: string(filler.SplitSourceEvidence), ParentAssetSHA256: strings.Repeat("8", 64),
		StructureDecisionSHA256: strings.Repeat("a", 64), StructureRole: filler.SegmentRoleCommercial,
		IntendedStartMs: 1_000, IntendedEndMs: 31_000,
	}
	measurement := completeConditioningMeasurement(-23)
	conditioning := filler.ConditioningEvidence{
		BeforeRewriteHash: lineage.ChildHash, AfterRewriteHash: manifest.Playback.Asset.ClipHash,
		BeforeRewrite: measurement, AfterRewrite: measurement,
		DerivedParentEdgesAfterRewrite: measurement.Cuts[0],
	}
	return filler.SidecarTags{
		SourceID: "archive:commercials", AcquisitionID: "acq-17", MediaAssets: &manifest,
		ConditioningLineage: &lineage, Conditioning: &conditioning,
	}
}

func passingAxisEvidence(t *testing.T, subject filler.SegmentScreeningSubject) []RecordedSegmentScreeningAxisEvidence {
	t.Helper()
	records := make([]RecordedSegmentScreeningAxisEvidence, 0, len(segmentScreeningAxisOrder))
	for index, axis := range segmentScreeningAxisOrder {
		reason := "policy_clear"
		switch axis {
		case filler.ScreenPlayback:
			reason = "playback_verified"
		}
		profile := screeningProfileFixture(axis, string(rune('1'+index)))
		raw := []byte("recorded-" + string(axis))
		recorded, err := NewSegmentScreeningAxisEvidence(
			subject, profile, filler.ScreenPass, reason, screeningSuitabilityForOutcome(subject, profile, filler.ScreenPass, raw),
			raw, time.Date(2026, time.September, 12, 4, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, recorded)
	}
	return records
}

func screeningSuitabilityForOutcome(subject filler.SegmentScreeningSubject, profile SegmentScreeningAxisProfile, outcome filler.SegmentScreeningOutcome, raw []byte) *fillerairworthiness.AxisEvidence {
	axisProfile, err := segmentAirworthinessProfile(profile)
	if err != nil {
		return nil
	}
	coverage := fillerairworthiness.CoverageComplete
	if outcome == filler.ScreenHold {
		coverage = fillerairworthiness.CoverageIncomplete
	}
	if outcome == filler.ScreenReject {
		coverage = fillerairworthiness.CoverageConflict
	}
	return &fillerairworthiness.AxisEvidence{
		SubjectSHA256: subject.SHA256, Profile: axisProfile, Coverage: coverage,
		EvidenceSHA256: filler.ScreeningBytesSHA256(raw), Observations: []fillerairworthiness.Observation{},
	}
}

func screeningAirworthinessEvaluator(t *testing.T, profiles []SegmentScreeningAxisProfile) *fillerairworthiness.Evaluator {
	t.Helper()
	evaluator, err := NewSegmentAirworthinessEvaluator(fillerairworthiness.ProfileAllAges, profiles)
	if err != nil {
		t.Fatal(err)
	}
	return evaluator
}

func screeningAirworthinessDecision(t *testing.T, subject filler.SegmentScreeningSubject, records []RecordedSegmentScreeningAxisEvidence) fillerairworthiness.Decision {
	t.Helper()
	profiles := make([]SegmentScreeningAxisProfile, 0, len(records))
	for _, record := range records {
		profiles = append(profiles, record.Evidence.Profile)
	}
	decision, err := evaluateSegmentAirworthiness(subject, records, screeningAirworthinessEvaluator(t, profiles))
	if err != nil {
		t.Fatal(err)
	}
	return decision
}
