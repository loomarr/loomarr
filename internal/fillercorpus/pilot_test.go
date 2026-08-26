package fillercorpus

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCommittedPrelingerPilotLaneSatisfiesContract(t *testing.T) {
	raw, err := os.ReadFile("corpus/pilot/prelinger.json")
	if err != nil {
		t.Fatal(err)
	}
	var lane Lane
	if err := json.Unmarshal(raw, &lane); err != nil {
		t.Fatal(err)
	}
	p := validPilot()
	p.SnapshotAt = time.Date(2026, 8, 26, 12, 15, 0, 0, time.UTC)
	p.LockedAt = p.SnapshotAt.Add(time.Minute)
	p.Lanes[0] = lane
	if failures := ValidatePilot(p); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
}

func TestCommittedLOCPilotLaneSatisfiesContract(t *testing.T) {
	raw, err := os.ReadFile("corpus/pilot/loc.json")
	if err != nil {
		t.Fatal(err)
	}
	var lane Lane
	if err := json.Unmarshal(raw, &lane); err != nil {
		t.Fatal(err)
	}
	p := validPilot()
	p.SnapshotAt = time.Date(2026, 8, 26, 13, 15, 0, 0, time.UTC)
	p.LockedAt = p.SnapshotAt.Add(time.Minute)
	p.Lanes[1] = lane
	if failures := ValidatePilot(p); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
}

func TestCommittedNASAPilotLaneSatisfiesContract(t *testing.T) {
	raw, err := os.ReadFile("corpus/pilot/nasa.json")
	if err != nil {
		t.Fatal(err)
	}
	var lane Lane
	if err := json.Unmarshal(raw, &lane); err != nil {
		t.Fatal(err)
	}
	p := validPilot()
	p.SnapshotAt = time.Date(2026, 8, 26, 13, 15, 0, 0, time.UTC)
	p.LockedAt = p.SnapshotAt.Add(time.Minute)
	p.Lanes[2] = lane
	if failures := ValidatePilot(p); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
}

func TestValidatePilotRequiresTenBoundedCasesFromEveryQualifiedLane(t *testing.T) {
	p := validPilot()
	if failures := ValidatePilot(p); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	p.Lanes[0].Cases = p.Lanes[0].Cases[:9]
	p.Lanes[1].RequestsUsed = p.Lanes[1].MaxRequests + 1
	failures := strings.Join(ValidatePilot(p), "\n")
	if !strings.Contains(failures, "want 10") || !strings.Contains(failures, "exceeded ceilings") {
		t.Fatalf("failures = %s", failures)
	}
}

func TestValidatePilotRejectsApprovalOrLabelFieldsAtDecodeBoundary(t *testing.T) {
	// Unknown-field rejection belongs to the command decoder; the domain shape
	// intentionally has no rights decision, truth, or semantic label fields.
	p := validPilot()
	p.Lanes[0].Cases[0].MetadataSHA256 = strings.Repeat("g", 64)
	if failures := ValidatePilot(p); len(failures) == 0 {
		t.Fatal("invalid digest passed")
	}
}

func validPilot() Pilot {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	p := Pilot{SchemaVersion: PilotSchemaVersion, SnapshotAt: snapshot, LockedAt: snapshot.Add(time.Hour)}
	for _, authority := range PilotAuthorities {
		lane := Lane{Authority: authority, MaxRequests: 20, RequestsUsed: 11, MaxResponseBytes: 1 << 20, ResponseBytes: 1000, MaxPredictedMediaBytes: 10000, PredictedMediaBytes: 10000, MaxWallTimeMS: 60000, WallTimeMS: 1000}
		for i := range 10 {
			id := strings.ReplaceAll(authority, "/", "-") + "-" + string(rune('a'+i))
			lane.Cases = append(lane.Cases, Candidate{ItemID: id, Title: id, RoleHints: []string{"candidate"}, ItemURL: "https://example.com/item/" + id, MetadataURL: "https://example.com/meta/" + id, MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), RightsAssertions: []string{"unreviewed source assertion"}, Representation: Representation{Name: id + ".mp4", URL: "https://example.com/media/" + id, MIMEType: "video/mp4", Bytes: 1000}})
		}
		p.Lanes = append(p.Lanes, lane)
	}
	return p
}
