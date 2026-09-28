package fillerreference

import (
	"os"
	"testing"
)

func TestBuildInspectionPlanRealHeldArtifacts(t *testing.T) {
	auditPath := os.Getenv("FILLER_REFERENCE_AUDIT")
	familyPath := os.Getenv("FILLER_REFERENCE_FAMILIES")
	seedPath := os.Getenv("FILLER_REFERENCE_SEED")
	if auditPath == "" || familyPath == "" || seedPath == "" {
		t.Skip("set FILLER_REFERENCE_AUDIT, FILLER_REFERENCE_FAMILIES, and FILLER_REFERENCE_SEED for the held-artifact regression")
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	families, err := os.ReadFile(familyPath)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildInspectionPlan(RawInspectionInputs{Audit: audit, Families: families, Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Cases) != 50 {
		t.Fatalf("selected cases=%d, want 50", len(plan.Cases))
	}
}
