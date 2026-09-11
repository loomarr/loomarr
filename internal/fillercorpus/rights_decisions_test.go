package fillercorpus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeRightsDecisionsIsStrictAndUnique(t *testing.T) {
	decision := RightsDecision{CaseID: "authority/item"}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeRightsDecisions(append(raw, '\n')); err != nil || len(got) != 1 {
		t.Fatalf("DecodeRightsDecisions(valid) = %d, %v", len(got), err)
	}
	for name, candidate := range map[string][]byte{
		"unknown":   []byte(`{"caseId":"authority/item","unknown":true}`),
		"trailing":  append(raw, []byte(` {}`)...),
		"duplicate": append(append(append([]byte{}, raw...), '\n'), raw...),
		"blank":     append(append(append([]byte{}, raw...), '\n'), '\n'),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRightsDecisions(candidate); err == nil {
				t.Fatalf("accepted %s decisions", name)
			}
		})
	}
}

func TestValidateRightsDecisionRejectsLegacyAndHeldGrants(t *testing.T) {
	at := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	item := InventoryCase{CaseID: "authority/item", CaptureIDs: []string{"capture"}, Authority: "authority", ItemID: "item", MetadataRetrievedAt: at, MetadataSHA256: strings.Repeat("a", 64)}
	decision := RightsDecision{
		InventorySHA256: strings.Repeat("b", 64), CaseID: item.CaseID, CaptureIDs: item.CaptureIDs,
		Authority: item.Authority, ItemID: item.ItemID, MetadataSHA256: item.MetadataSHA256,
		ReviewerID: "reviewer", ReviewedAt: at.Add(time.Minute), Decision: "approved", Basis: "complete review", Redistributable: true,
	}
	if err := ValidateRightsDecision(item, decision.InventorySHA256, decision, RightsProfileDevelopment, at.Add(time.Hour)); err != nil {
		t.Fatalf("ValidateRightsDecision(valid): %v", err)
	}
	legacy := decision
	legacy.InventorySHA256 = strings.Repeat("c", 64)
	if err := ValidateRightsDecision(item, decision.InventorySHA256, legacy, RightsProfileDevelopment, at.Add(time.Hour)); err == nil {
		t.Fatal("accepted digest-swapped decision")
	}
	held := decision
	held.Decision = "held"
	if err := ValidateRightsDecision(item, decision.InventorySHA256, held, RightsProfileDevelopment, at.Add(time.Hour)); err == nil {
		t.Fatal("accepted held decision with redistribution grant")
	}
}
