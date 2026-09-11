package fillercandidatepool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeRejectsLegacyUnknownAndTrailingData(t *testing.T) {
	valid := testPool()
	raw := marshalPool(t, valid)
	if _, err := Decode(raw); err != nil {
		t.Fatalf("Decode(valid): %v", err)
	}
	legacy := valid
	legacy.SchemaVersion = 0
	unknown := append(raw[:len(raw)-2], []byte(",\n  \"eligible\": true\n}\n")...)
	for name, candidate := range map[string][]byte{
		"legacy":   marshalPool(t, legacy),
		"unknown":  unknown,
		"trailing": append(raw, []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(candidate); err == nil {
				t.Fatalf("Decode accepted %s data", name)
			}
		})
	}
}

func TestValidateRejectsAuthorityAndRoleEscalation(t *testing.T) {
	tests := map[string]func(*Pool){
		"input digest swap": func(pool *Pool) { pool.Inputs[0].SHA256 = strings.Repeat("b", 64)[:63] },
		"downstream grant":  func(pool *Pool) { pool.ProductionAdmissionAllowed = true },
		"hand role": func(pool *Pool) {
			pool.Candidates[1].Role = "trailer"
		},
		"missing remote URL": func(pool *Pool) { pool.Candidates[0].Source.MediaURL = "" },
		"held without reason": func(pool *Pool) {
			pool.Candidates[0].Disposition = DispositionHeld
		},
		"exposed family duplication": func(pool *Pool) {
			pool.PriorExposure.FamilyIDs = []string{"family-1", "family-1"}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			pool := testPool()
			mutate(&pool)
			if err := Validate(pool); err == nil {
				t.Fatalf("Validate accepted %s", name)
			}
		})
	}
}

func TestValidateAcceptsExplicitHeldCandidate(t *testing.T) {
	pool := testPool()
	pool.Candidates[0].Disposition = DispositionHeld
	pool.Candidates[0].HoldReasons = []string{HoldSuitability}
	pool.Candidates[0].Role = ""
	pool.Candidates[0].Transition = nil
	if err := Validate(pool); err != nil {
		t.Fatalf("Validate(held): %v", err)
	}
}

func testPool() Pool {
	at := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	sha := strings.Repeat("a", 64)
	programmeSHA := strings.Repeat("b", 64)
	return Pool{
		SchemaVersion: SchemaVersion, ContractVersion: ContractVersion, GeneratedAt: at,
		Inputs:        []Input{{Name: "inventory", SHA256: sha}},
		PriorExposure: Exposure{SourceSHA256: []string{}, FamilyIDs: []string{}, ProgrammeProvenance: []ProgrammeProvenance{}},
		Candidates: []Candidate{
			{
				CaseID: "example.org/anchor", Kind: KindStandaloneAnchor, Disposition: DispositionEligible, HoldReasons: []string{}, FamilyID: "family-1", Role: "commercial",
				Source:     Source{ID: "candidate-anchor", Path: "media/anchor.mp4", SHA256: sha, Bytes: 100, DurationMS: 30_000, Transport: TransportHTTPS, Authority: "example.org", ItemID: "anchor", ItemURL: "https://example.org/items/anchor", MediaURL: "https://example.org/media/anchor.mp4", MetadataSHA256: sha, MetadataRetrievedAt: at, SoundtrackStatus: "present_expected", SoundtrackEvidence: "first_party_metadata", SoundtrackEvidenceSHA: sha},
				Transition: &Transition{EvidenceAlias: "evidence-anchor", Head: Edge{StartMS: 0, EndMS: 1_000}, Tail: Edge{StartMS: 29_000, EndMS: 30_000}},
			},
			{
				CaseID: "example.org/programme", Kind: KindProgrammeParent, Disposition: DispositionEligible, HoldReasons: []string{}, FamilyID: "family-2",
				Source: Source{ID: "candidate-programme", Path: "media/programme.mp4", SHA256: programmeSHA, Bytes: 200, DurationMS: 120_000, Transport: TransportLocal, Authority: "example.org", ItemID: "programme", ItemURL: "https://example.org/items/programme", MetadataSHA256: sha, MetadataRetrievedAt: at, SoundtrackStatus: "present_expected", SoundtrackEvidence: "reviewed_source_manifest", SoundtrackEvidenceSHA: sha},
			},
		},
	}
}

func marshalPool(t *testing.T, pool Pool) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(pool, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}
