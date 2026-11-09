package acquisition_test

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
)

func candidate(source, id string, year, height int, license string) acquisition.Candidate {
	return acquisition.Candidate{
		Identity: acquisition.RemoteIdentity{Provider: "archive", SourceID: source, RemoteID: id},
		URL:      "https://archive.org/details/" + id, Title: id, License: license,
		ObservedYear: year, Height: height,
	}
}

func TestPlanAcquisition_IsDeterministicAndDiverse(t *testing.T) {
	intent := acquisition.AcquisitionIntent{Count: 3}
	input := []acquisition.Candidate{
		candidate("a", "third", 1992, 1080, ""),
		candidate("a", "first", 1990, 480, "cc-by"),
		candidate("b", "second", 1990, 720, "cc-by"),
		candidate("a", "fourth", 1993, 2160, "cc-by"),
	}
	want := []string{"fourth", "third", "second"}
	for run := 0; run < 2; run++ {
		plan, err := acquisition.PlanAcquisition(intent, input, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, len(plan.Selected))
		for i, decision := range plan.Selected {
			got[i] = decision.Candidate.Identity.RemoteID
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("selected = %v, want %v", got, want)
		}
		if len(plan.Rejected) != 1 || plan.Rejected[0].Disposition != acquisition.CandidateRankedBelowLimit {
			t.Fatalf("rejected = %+v", plan.Rejected)
		}
	}
}

func TestPlanAcquisition_UnknownNeverSatisfiesHardConstraints(t *testing.T) {
	base := candidate("classic", "unknown", 0, 0, "")
	checks := []struct {
		name   string
		intent acquisition.AcquisitionIntent
		want   acquisition.CandidateDisposition
	}{
		{"era", acquisition.AcquisitionIntent{EraStart: 1980, EraEnd: 1989}, acquisition.CandidateEraUnknown},
		{"duration", acquisition.AcquisitionIntent{MaxDurationMS: 120_000}, acquisition.CandidateDurationUnknown},
		{"quality", acquisition.AcquisitionIntent{MinHeight: 480}, acquisition.CandidateQualityUnknown},
		{"role", acquisition.AcquisitionIntent{Roles: []clips.Role{clips.Commercial}}, acquisition.CandidateRoleUnknown},
		{"audience", acquisition.AcquisitionIntent{Audiences: []clips.Audience{clips.Kids}}, acquisition.CandidateAudienceUnknown},
		{"taxonomy", acquisition.AcquisitionIntent{TaxonomyGaps: []string{"toys"}}, acquisition.CandidateTaxonomyUnknown},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := acquisition.PlanAcquisition(tc.intent, []acquisition.Candidate{base}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Selected) != 0 || len(plan.Rejected) != 1 || plan.Rejected[0].Disposition != tc.want {
				t.Fatalf("plan = %+v, want rejection %s", plan, tc.want)
			}
		})
	}
}

func TestPlanAcquisition_LicenseMetadataDoesNotAffectSelection(t *testing.T) {
	withLicense := candidate("classic", "z-declared", 1992, 720, "cc-by")
	withoutLicense := candidate("classic", "a-not-provided", 1992, 720, "")
	plan, err := acquisition.PlanAcquisition(acquisition.AcquisitionIntent{Count: 1}, []acquisition.Candidate{withLicense, withoutLicense}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 1 || plan.Selected[0].Candidate.Identity.RemoteID != "a-not-provided" {
		t.Fatalf("selected = %+v, want stable identity ordering independent of licence metadata", plan.Selected)
	}
	if plan.Selected[0].Candidate.License != "" || len(plan.Rejected) != 1 || plan.Rejected[0].Candidate.License != "cc-by" {
		t.Fatalf("plan did not preserve passive licence metadata: %+v", plan)
	}
}

func TestPlanAcquisition_ExplainsPriorStateAndDuplicates(t *testing.T) {
	first := candidate("classic", "same", 1992, 480, "cc-by")
	key := first.Identity.Key()
	plan, err := acquisition.PlanAcquisition(acquisition.AcquisitionIntent{Count: 5}, []acquisition.Candidate{
		first,
		first,
		candidate("classic", "queued", 1992, 480, "cc-by"),
		candidate("classic", "declined", 1992, 480, "cc-by"),
	}, map[string]acquisition.ExistingRemoteState{
		key: acquisition.RemoteCatalogued,
		candidate("classic", "queued", 0, 0, "").Identity.Key():   acquisition.RemoteQueued,
		candidate("classic", "declined", 0, 0, "").Identity.Key(): acquisition.RemoteDeclined,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[acquisition.CandidateDisposition]int{
		acquisition.CandidateAlreadyCatalogued:  1,
		acquisition.CandidateDuplicateRemote:    1,
		acquisition.CandidateAlreadyQueued:      1,
		acquisition.CandidatePreviouslyDeclined: 1,
	}
	got := map[acquisition.CandidateDisposition]int{}
	for _, decision := range plan.Rejected {
		got[decision.Disposition]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rejections = %v, want %v", got, want)
	}
}

func TestDefaultAcquisitionIntent_NamesTheCoverageProjection(t *testing.T) {
	intent := acquisition.DefaultAcquisitionIntent(pod.PoolReport{Channels: []pod.ChannelCoverage{{
		Name: "Saturday Mornings", Report: pod.CoverageReport{Level: pod.MatchBumperCard},
	}}}, pod.Geography{Country: "us", Place: "5128581"})
	if intent.CatalogReason != "Improve filler coverage for Saturday Mornings; its current match level is bumper_card." {
		t.Fatalf("reason = %q", intent.CatalogReason)
	}
	if intent.Geography.Country != "US" || intent.Count != 12 || intent.Version != acquisition.AcquisitionIntentVersion {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestPlanAcquisition_RejectsConstraintsOutsideTheClosedVocabulary(t *testing.T) {
	for _, intent := range []acquisition.AcquisitionIntent{
		{Count: -1},
		{Count: 51},
		{EraStart: 1799},
		{EraEnd: 2201},
		{MinHeight: 4321},
		{Roles: []clips.Role{clips.Unclassified}},
	} {
		if _, err := acquisition.PlanAcquisition(intent, nil, nil); err == nil {
			t.Errorf("PlanAcquisition(%+v) succeeded, want validation error", intent)
		}
	}
}

func TestAcquisitionIntentFamily_IgnoresPresentationButKeepsSelectionConstraints(t *testing.T) {
	base := acquisition.AcquisitionIntent{EraStart: 1990, EraEnd: 1999, Count: 2, CatalogReason: "first"}
	presentationChange := acquisition.AcquisitionIntent{EraStart: 1990, EraEnd: 1999, Count: 40, CatalogReason: "second"}
	constraintChange := acquisition.AcquisitionIntent{EraStart: 1980, EraEnd: 1989, Count: 2, CatalogReason: "first"}
	if base.FamilyKey() != presentationChange.FamilyKey() {
		t.Fatal("count/reason changed the semantic intent family")
	}
	if base.FamilyKey() == constraintChange.FamilyKey() {
		t.Fatal("era constraints did not change the semantic intent family")
	}
}
