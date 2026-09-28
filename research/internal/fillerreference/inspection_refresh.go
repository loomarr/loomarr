package fillerreference

import (
	"fmt"
	"reflect"
)

const (
	inspectionSeedRebindKind          = "filler_reference_inspection_seed_rebind"
	inspectionSeedRebindSchemaVersion = 1
)

type InspectionSeedRebindInputs struct {
	LegacySeedSHA256 string `json:"legacySeedSha256"`
	AuditSHA256      string `json:"auditSha256"`
	FamilySHA256     string `json:"familyAuditSha256"`
}

type InspectionSeedRebindReport struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	Kind              string                     `json:"kind"`
	FromContract      string                     `json:"fromContract"`
	ToContract        string                     `json:"toContract"`
	Inputs            InspectionSeedRebindInputs `json:"inputs"`
	OutputSeedSHA256  string                     `json:"outputSeedSha256"`
	SelectedCaseCount int                        `json:"selectedCaseCount"`
}

type ReboundInspectionSeed struct {
	Seed   []byte
	Report InspectionSeedRebindReport
}

// RebindInspectionSeed advances the one retained inspection selection to the
// current audit identities without changing what was selected or why.
func RebindInspectionSeed(legacySeedRaw, auditRaw, familyRaw []byte) (ReboundInspectionSeed, error) {
	seed, err := decodeStrictJSON[InspectionSeed](legacySeedRaw)
	if err != nil {
		return ReboundInspectionSeed{}, fmt.Errorf("legacy inspection seed: %w", err)
	}
	if seed.SchemaVersion != 1 || seed.Kind != "filler_reference_inspection_seed" || seed.Status != "preliminary_requires_full_playback" || seed.ContractVersion != legacyReferenceContractVersion || !validSHA256(seed.SourceAuditSHA256) || !validSHA256(seed.DuplicateAuditSHA256) {
		return ReboundInspectionSeed{}, fmt.Errorf("legacy inspection seed identity is invalid")
	}

	legacySemantics := inspectionSeedSemantics(seed)
	seed.ContractVersion = ContractVersion
	seed.SourceAuditSHA256 = SHA256(auditRaw)
	seed.DuplicateAuditSHA256 = SHA256(familyRaw)
	seedRaw, err := marshalIndented(seed)
	if err != nil {
		return ReboundInspectionSeed{}, err
	}
	if _, err := BuildInspectionPlan(RawInspectionInputs{Audit: auditRaw, Families: familyRaw, Seed: seedRaw}); err != nil {
		return ReboundInspectionSeed{}, fmt.Errorf("rebound inspection seed: %w", err)
	}
	if !reflect.DeepEqual(legacySemantics, inspectionSeedSemantics(seed)) {
		return ReboundInspectionSeed{}, fmt.Errorf("inspection seed semantics changed during rebind")
	}

	return ReboundInspectionSeed{
		Seed: seedRaw,
		Report: InspectionSeedRebindReport{
			SchemaVersion: inspectionSeedRebindSchemaVersion,
			Kind:          inspectionSeedRebindKind,
			FromContract:  legacyReferenceContractVersion,
			ToContract:    ContractVersion,
			Inputs: InspectionSeedRebindInputs{
				LegacySeedSHA256: SHA256(legacySeedRaw),
				AuditSHA256:      SHA256(auditRaw),
				FamilySHA256:     SHA256(familyRaw),
			},
			OutputSeedSHA256:  SHA256(seedRaw),
			SelectedCaseCount: len(seed.SelectedCaseIDs),
		},
	}, nil
}

type inspectionSeedSemanticFields struct {
	SchemaVersion    int
	Kind             string
	Status           string
	TriageEvidence   TriageEvidence
	SelectionPolicy  InspectionSelection
	SelectedCaseIDs  []string
	ExplicitFindings []ExplicitTriageFinding
	RequiredNextGate string
}

func inspectionSeedSemantics(seed InspectionSeed) inspectionSeedSemanticFields {
	return inspectionSeedSemanticFields{
		SchemaVersion: seed.SchemaVersion, Kind: seed.Kind, Status: seed.Status,
		TriageEvidence: seed.TriageEvidence, SelectionPolicy: seed.SelectionPolicy,
		SelectedCaseIDs: seed.SelectedCaseIDs, ExplicitFindings: seed.ExplicitFindings,
		RequiredNextGate: seed.RequiredNextGate,
	}
}
