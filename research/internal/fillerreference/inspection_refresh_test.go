package fillerreference

import (
	"encoding/json"
	"testing"
)

func TestRebindInspectionSeedPreservesSelectionAndPassesCurrentGate(t *testing.T) {
	raw, current := inspectionInputs(t)
	legacy := current
	legacy.ContractVersion = legacyReferenceContractVersion
	legacy.SourceAuditSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	legacy.DuplicateAuditSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	legacyRaw := mustJSON(t, legacy)

	result, err := RebindInspectionSeed(legacyRaw, raw.Audit, raw.Families)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Inputs.LegacySeedSHA256 != SHA256(legacyRaw) || result.Report.OutputSeedSHA256 != SHA256(result.Seed) || result.Report.SelectedCaseCount != 50 {
		t.Fatalf("report=%+v", result.Report)
	}
	var got InspectionSeed
	if err := json.Unmarshal(result.Seed, &got); err != nil {
		t.Fatal(err)
	}
	if got.ContractVersion != ContractVersion || got.SourceAuditSHA256 != SHA256(raw.Audit) || got.DuplicateAuditSHA256 != SHA256(raw.Families) {
		t.Fatalf("rebound identity=%+v", got)
	}
	legacy.ContractVersion, legacy.SourceAuditSHA256, legacy.DuplicateAuditSHA256 = got.ContractVersion, got.SourceAuditSHA256, got.DuplicateAuditSHA256
	if !equalJSON(t, legacy, got) {
		t.Fatal("rebind changed seed semantics")
	}
	if _, err := BuildInspectionPlan(RawInspectionInputs{Audit: raw.Audit, Families: raw.Families, Seed: result.Seed}); err != nil {
		t.Fatalf("current gate rejected rebound seed: %v", err)
	}
}

func TestRebindInspectionSeedRejectsCurrentOrMalformedLegacySeed(t *testing.T) {
	raw, seed := inspectionInputs(t)
	if _, err := RebindInspectionSeed(raw.Seed, raw.Audit, raw.Families); err == nil {
		t.Fatal("current seed accepted as legacy input")
	}
	seed.ContractVersion = legacyReferenceContractVersion
	seed.SourceAuditSHA256 = "not-a-sha"
	malformed := mustJSON(t, seed)
	if _, err := RebindInspectionSeed(malformed, raw.Audit, raw.Families); err == nil {
		t.Fatal("malformed legacy binding accepted")
	}
	unknown := append(malformed[:len(malformed)-1], []byte(`,"unknown":true}`)...)
	if _, err := RebindInspectionSeed(unknown, raw.Audit, raw.Families); err == nil {
		t.Fatal("unknown legacy field accepted")
	}
}

func equalJSON(t *testing.T, left, right any) bool {
	t.Helper()
	leftRaw := mustJSON(t, left)
	rightRaw := mustJSON(t, right)
	return string(leftRaw) == string(rightRaw)
}
