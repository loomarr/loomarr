package fillerreference

import (
	"fmt"
	"testing"
)

func TestInspectionCasesRequiresBoundFiftyCaseSeed(t *testing.T) {
	audit := Audit{Cases: make([]Case, 50)}
	seed := InspectionSeed{
		SchemaVersion: 1, Kind: "filler_reference_inspection_seed", Status: "preliminary_requires_full_playback", ContractVersion: ContractVersion,
		SourceAuditSHA256: "audit", DuplicateAuditSHA256: "family", TriageEvidence: TriageEvidence{SampledFramesPerSource: 4},
		SelectionPolicy: InspectionSelection{TargetSources: 50, FinalAcceptedClips: 32}, RequiredNextGate: "play all",
	}
	fingerprints := make([]FamilyFingerprint, 50)
	for i := range audit.Cases {
		id := fmt.Sprintf("case-%02d", i)
		audit.Cases[i] = Case{CaseID: id, ContentSHA256: "content", SourceLocalFile: id + ".mp4", Disposition: DispositionCandidate, Media: MediaSummary{SourceDurationMS: 30_000}}
		seed.SelectedCaseIDs = append(seed.SelectedCaseIDs, id)
		fingerprints[i] = FamilyFingerprint{CaseID: id}
	}
	families := FamilyAudit{SourceAudit: "audit", Fingerprints: fingerprints}
	inputs := MeasurementInputIdentity{AuditSHA256: "audit", FamilySHA256: "family", SeedSHA256: "seed"}
	got, err := InspectionCases(seed, audit, families, inputs)
	if err != nil || len(got) != 50 {
		t.Fatalf("selected=%d err=%v", len(got), err)
	}
	seed.SelectedCaseIDs[49] = seed.SelectedCaseIDs[0]
	if _, err := InspectionCases(seed, audit, families, inputs); err == nil {
		t.Fatal("duplicate selected case accepted")
	}
}
