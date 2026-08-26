package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestRunCombinesStrictInventories(t *testing.T) {
	dir := t.TempDir()
	first := validCommandInventory(time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), "archive.org/prelinger", "archive.org")
	second := validCommandInventory(first.SnapshotAt.Add(time.Minute), "loc.gov/national-screening-room", "tile.loc.gov")
	var args []string
	for index, value := range []fillercorpus.Inventory{first, second} {
		path := filepath.Join(dir, string(rune('a'+index))+".json")
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--inventory", path)
	}
	output := filepath.Join(dir, "merged.json")
	args = append(args, "--out", output)
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := fillercorpus.DecodeInventoryBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Captures) != 2 || len(merged.Cases) != 2 {
		t.Fatalf("merged = %+v", merged)
	}
}

func validCommandInventory(snapshot time.Time, authority, host string) fillercorpus.Inventory {
	role := "commercial"
	captureID := fillercorpus.NewCaptureID(authority, "", role)
	return fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: snapshot, Captures: []fillercorpus.Capture{{CaptureID: captureID, Authority: authority, RoleHint: role, SnapshotAt: snapshot, MaxRequests: 1, RequestsUsed: 1, MaxResponseBytes: 100, ResponseBytes: 50, MaxPredictedMediaBytes: 100, PredictedMediaBytes: 100, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []fillercorpus.InventoryCase{{CaseID: fillercorpus.CaseID(authority, "clip"), CaptureID: captureID, Authority: authority, ItemID: "clip", Title: "Clip", RoleHints: []string{role}, RightsAssertions: []string{"review"}, ItemURL: "https://example.org/item", MetadataURL: "https://example.org/metadata", MetadataRetrievedAt: snapshot, MetadataSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AllowedMediaHosts: []string{host}, Representation: fillercorpus.InventoryRepresentation{Name: "clip.mp4", URL: "https://" + host + "/clip.mp4", MIMEType: "video/mp4", Bytes: 100}}}}
}
