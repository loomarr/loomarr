package fillercorpus

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecodeInventoryRejectsLegacyAndUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"schemaVersion":1,"source":"archive.org","collection":"prelinger","snapshotAt":"2026-08-26T00:00:00Z","cases":[]}`,
		`{"schemaVersion":2,"snapshotAt":"2026-08-26T00:00:00Z","captures":[],"cases":[],"legacy":true}`,
	} {
		if _, err := DecodeInventory(strings.NewReader(raw)); err == nil {
			t.Fatalf("DecodeInventory(%s) succeeded", raw)
		}
	}
}

func TestValidateInventoryAcceptsMixedAuthoritiesAndExactHosts(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	value := Inventory{SchemaVersion: InventorySchemaVersion, SnapshotAt: snapshot}
	for _, authority := range []string{"archive.org/prelinger", "loc.gov/national-screening-room"} {
		captureID := NewCaptureID(authority, "", "commercial")
		value.Captures = append(value.Captures, Capture{CaptureID: captureID, Transport: TransportHTTPS, Authority: authority, RoleHint: "commercial", SnapshotAt: snapshot, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 100, ResponseBytes: 50, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10})
		itemID := strings.ReplaceAll(authority, "/", "-")
		mediaHost := "archive.org"
		if authority == "loc.gov/national-screening-room" {
			mediaHost = "tile.loc.gov"
		}
		value.Cases = append(value.Cases, InventoryCase{
			CaseID: CaseID(authority, itemID), CaptureID: captureID, Authority: authority, ItemID: itemID, Title: "Clip", RoleHints: []string{"commercial"},
			RightsAssertions: []string{"public domain"}, ItemURL: "https://example.org/item", MetadataURL: "https://example.org/metadata",
			MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), AllowedMediaHosts: []string{mediaHost},
			Representation: InventoryRepresentation{Transport: TransportHTTPS, Name: "clip.mp4", URL: "https://" + mediaHost + "/clip.mp4?frozen=1", MIMEType: "video/mp4", Bytes: 100},
		})
	}
	if failures := ValidateInventory(value); len(failures) != 0 {
		t.Fatalf("ValidateInventory() = %v", failures)
	}
	value.Cases[0].Representation.URL = "https://cdn.example.org/clip.mp4"
	if failures := ValidateInventory(value); len(failures) == 0 {
		t.Fatal("ValidateInventory accepted a host outside the case allowlist")
	}
}

func TestMergeInventoriesIsDeterministicAndRejectsDuplicateCapture(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	base := validInventoryForMerge(snapshot, "archive.org/prelinger", "archive.org")
	later := validInventoryForMerge(snapshot.Add(time.Minute), "loc.gov/national-screening-room", "tile.loc.gov")
	merged, err := MergeInventories(later, base)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.SnapshotAt.Equal(later.SnapshotAt) || merged.Captures[0].Authority != "archive.org/prelinger" || len(merged.Cases) != 2 {
		t.Fatalf("merged = %+v", merged)
	}
	if _, err := MergeInventories(base, base); err == nil {
		t.Fatal("duplicate capture was merged")
	}
}

func validInventoryForMerge(snapshot time.Time, authority, host string) Inventory {
	role := "commercial"
	captureID := NewCaptureID(authority, "", role)
	itemID := strings.ReplaceAll(authority, "/", "-")
	return Inventory{SchemaVersion: InventorySchemaVersion, SnapshotAt: snapshot, Captures: []Capture{{CaptureID: captureID, Transport: TransportHTTPS, Authority: authority, RoleHint: role, SnapshotAt: snapshot, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 100, ResponseBytes: 50, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []InventoryCase{{CaseID: CaseID(authority, itemID), CaptureID: captureID, Authority: authority, ItemID: itemID, Title: "Clip", RoleHints: []string{role}, RightsAssertions: []string{"public domain"}, ItemURL: "https://example.org/item", MetadataURL: "https://example.org/metadata", MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), AllowedMediaHosts: []string{host}, Representation: InventoryRepresentation{Transport: TransportHTTPS, Name: "clip.mp4", URL: "https://" + host + "/clip.mp4", MIMEType: "video/mp4", Bytes: 100}}}}
}

func TestInventoryFromLanePreservesCaptureEvidenceWithoutGrantingAuthority(t *testing.T) {
	snapshot := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	lane := Lane{Authority: "images.nasa.gov", MaxRequests: 3, RequestsUsed: 3, MaxResponseBytes: 1000, ResponseBytes: 500, MaxPredictedMediaBytes: 200, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10, Cases: []Candidate{{ItemID: "nasa-1", Title: "Trailer", RoleHints: []string{"trailer"}, ItemURL: "https://images.nasa.gov/details/nasa-1", MetadataURL: "https://images-assets.nasa.gov/metadata.json", MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), RightsAssertions: []string{"NASA"}, Representation: Representation{Name: "clip.mp4", URL: "https://images-assets.nasa.gov/clip.mp4", MIMEType: "video/mp4", Bytes: 100}}}}
	got, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "mission trailers", AllowedMediaHosts: []string{"images-assets.nasa.gov"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cases[0].CaseID != "images.nasa.gov/nasa-1" || got.Captures[0].RequestsUsed != 3 {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestCommittedPilotLanesPromoteToStrictInventories(t *testing.T) {
	snapshot := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	lanes := map[string]string{"prelinger": "archive.org", "loc": "tile.loc.gov", "nasa": "images-assets.nasa.gov", "cdc": "www.cdc.gov", "commons": "upload.wikimedia.org"}
	for name, host := range lanes {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("corpus/pilot/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var lane Lane
			if err := json.Unmarshal(raw, &lane); err != nil {
				t.Fatal(err)
			}
			if _, err := InventoryFromLane(lane, LaneInventoryOptions{SnapshotAt: snapshot, Collection: "pilot/" + name, AllowedMediaHosts: []string{host}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
