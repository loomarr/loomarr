package fillerrelease_test

import (
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/fillerrelease"
)

func TestSelectHouseholdCohortUsesFrozenOrderAndKeepsFiveReserves(t *testing.T) {
	seed, candidates := cohortFixtures()

	selection, err := fillerrelease.SelectHouseholdCohort(seed, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Selected) != 32 || len(selection.Reserves) != 5 || len(selection.Excluded) != 13 {
		t.Fatalf("selection = %d selected, %d reserves, %d excluded", len(selection.Selected), len(selection.Reserves), len(selection.Excluded))
	}
	if selection.Selected[0].CaseID != "case-00" || selection.Selected[31].CaseID != "case-31" || selection.Reserves[0].CaseID != "case-32" {
		t.Fatalf("selection order drifted: first=%q last=%q reserve=%q", selection.Selected[0].CaseID, selection.Selected[31].CaseID, selection.Reserves[0].CaseID)
	}
}

func TestSelectHouseholdCohortSkipsKnownDuplicateFamilyAndAdvancesReserve(t *testing.T) {
	seed, candidates := cohortFixtures()
	candidates[0].DuplicateFamilyIdentity = "family-a"
	candidates[1].DuplicateFamilyIdentity = "family-a"

	selection, err := fillerrelease.SelectHouseholdCohort(seed, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Selected[1].CaseID != "case-02" || selection.Selected[31].CaseID != "case-32" {
		t.Fatalf("duplicate family did not advance the ordered reserve: %#v", selection.Selected)
	}
	found := false
	for _, excluded := range selection.Excluded {
		found = found || excluded.CaseID == "case-01" && excluded.Reason == "duplicate_family"
	}
	if !found {
		t.Fatalf("duplicate exclusion absent: %#v", selection.Excluded)
	}
}

func TestSelectHouseholdCohortFailsClosedOnInventoryDriftAndShortfall(t *testing.T) {
	seed, candidates := cohortFixtures()
	for _, test := range []struct {
		name   string
		mutate func(*[]string, *[]fillerrelease.CohortCandidate)
	}{
		{"missing case", func(_ *[]string, candidates *[]fillerrelease.CohortCandidate) { *candidates = (*candidates)[:49] }},
		{"duplicate case", func(_ *[]string, candidates *[]fillerrelease.CohortCandidate) { (*candidates)[49] = (*candidates)[48] }},
		{"seed duplicate", func(seed *[]string, _ *[]fillerrelease.CohortCandidate) { (*seed)[49] = (*seed)[48] }},
		{"identity drift", func(_ *[]string, candidates *[]fillerrelease.CohortCandidate) {
			for index := range 19 {
				(*candidates)[index].PlaybackDerivativeSHA256 = "changed"
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			localSeed := append([]string(nil), seed...)
			localCandidates := append([]fillerrelease.CohortCandidate(nil), candidates...)
			test.mutate(&localSeed, &localCandidates)
			if _, err := fillerrelease.SelectHouseholdCohort(localSeed, localCandidates); err == nil {
				t.Fatal("selection succeeded")
			}
		})
	}
}

func cohortFixtures() ([]string, []fillerrelease.CohortCandidate) {
	seed := make([]string, 50)
	candidates := make([]fillerrelease.CohortCandidate, 50)
	for index := range seed {
		caseID := fmt.Sprintf("case-%02d", index)
		seed[index] = caseID
		candidate := fillerrelease.CohortCandidate{
			CaseID: caseID, ClipHash: digest(fmt.Sprintf("clip-%02d", index)),
			SourceIdentity: "archive.org:fixture", SourceMasterSHA256: digest(fmt.Sprintf("source-%02d", index)),
			LineageSHA256:            digest(fmt.Sprintf("lineage-%02d", index)),
			PlaybackDerivativeSHA256: digest(fmt.Sprintf("playback-%02d", index)),
			SidecarSHA256:            digest(fmt.Sprintf("sidecar-%02d", index)), Disposition: "ready", RangePlayback: true,
		}
		if index >= 37 {
			candidate.Disposition = "rejected"
		}
		candidates[index] = candidate
	}
	return seed, candidates
}
