package fillerreference

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestBuildInspectionPlanRequiresBoundFiftyCaseSeed(t *testing.T) {
	raw, seed := inspectionInputs(t)
	got, err := BuildInspectionPlan(raw)
	if err != nil || len(got.Cases) != 50 {
		t.Fatalf("selected=%d err=%v", len(got.Cases), err)
	}
	if got.Inputs.AuditSHA256 != SHA256(raw.Audit) || got.Inputs.FamilySHA256 != SHA256(raw.Families) || got.Inputs.SeedSHA256 != SHA256(raw.Seed) {
		t.Fatalf("input identities=%+v", got.Inputs)
	}

	seed.SelectedCaseIDs[49] = seed.SelectedCaseIDs[0]
	raw.Seed = mustJSON(t, seed)
	if _, err := BuildInspectionPlan(raw); err == nil {
		t.Fatal("duplicate selected case accepted")
	}
}

func TestBuildInspectionPlanRecomputesFamilyEvidence(t *testing.T) {
	raw, _ := inspectionInputs(t)
	var families FamilyAudit
	if err := json.Unmarshal(raw.Families, &families); err != nil {
		t.Fatal(err)
	}
	families.Summary.RelatedPairs++
	raw.Families = mustJSON(t, families)
	if _, err := BuildInspectionPlan(raw); err == nil {
		t.Fatal("mutated family summary accepted")
	}
}

func TestBuildInspectionPlanRequiresFamilyHoldForRelatedRenditions(t *testing.T) {
	raw, seed := inspectionInputs(t)
	seed.ExplicitFindings = seed.ExplicitFindings[1:]
	raw.Seed = mustJSON(t, seed)
	if _, err := BuildInspectionPlan(raw); err == nil {
		t.Fatal("related selected renditions accepted without explicit family hold")
	}
}

func inspectionInputs(t *testing.T) (RawInspectionInputs, InspectionSeed) {
	t.Helper()
	auditTime := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	familyTime := auditTime.Add(time.Hour)
	fingerprints := make([]FamilyFingerprint, 50)
	for i := range fingerprints {
		id := fmt.Sprintf("case-%02d", i)
		frames := make([]uint64, 12)
		for index := range frames {
			frames[index] = uint64(index+5+i*97) * 0x0102040810204081
		}
		if i == 1 {
			frames = append([]uint64(nil), fingerprints[0].FrameHashes...)
		}
		fingerprints[i] = FamilyFingerprint{
			CaseID: id, ContentSHA256: fmt.Sprintf("%064x", i+1000), LocalFile: id + ".mp4",
			FrameHashes: frames, AudioRMS: []uint32{1},
		}
	}
	auditRaw := familySourceAudit(t, fingerprints, auditTime)
	var audit Audit
	if err := json.Unmarshal(auditRaw, &audit); err != nil {
		t.Fatal(err)
	}
	for index := range fingerprints {
		audit.Cases[index].Media.SourceDurationMS = 30_000
	}
	auditRaw = mustJSON(t, audit)
	families, err := BuildFamilyAudit(auditRaw, fingerprints, familyTime)
	if err != nil {
		t.Fatal(err)
	}
	familyRaw := mustJSON(t, families)
	seed := InspectionSeed{
		SchemaVersion: 1, Kind: "filler_reference_inspection_seed", Status: "preliminary_requires_full_playback", ContractVersion: ContractVersion,
		SourceAuditSHA256: SHA256(auditRaw), DuplicateAuditSHA256: SHA256(familyRaw),
		TriageEvidence: TriageEvidence{Method: "four ordered frames", SampledFramesPerSource: 4, Limitations: "full playback required"},
		SelectionPolicy: InspectionSelection{
			TargetSources: 50, FinalAcceptedClips: 32, Prefer: []string{"bounded inspection"},
			DoNotBackfill: []string{"missing roles"}, CoverageShortfall: "coverage remains unaccepted",
		},
		ExplicitFindings: []ExplicitTriageFinding{
			{CaseIDPattern: "duplicate-family-*", Disposition: "inspection_hold", Evidence: "play related renditions"},
			{CaseIDPattern: "*", Disposition: "no_editorial_acceptance", Evidence: "measurement is not acceptance"},
		},
		RequiredNextGate: "play every selected source",
	}
	for _, fingerprint := range fingerprints {
		seed.SelectedCaseIDs = append(seed.SelectedCaseIDs, fingerprint.CaseID)
	}
	seedRaw := mustJSON(t, seed)
	return RawInspectionInputs{Audit: auditRaw, Families: familyRaw, Seed: seedRaw}, seed
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
