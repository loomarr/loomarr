package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func reviewInventory(snapshot time.Time, ids ...string) fillercorpus.Inventory {
	captureID := fillercorpus.NewCaptureID("archive.org/prelinger", "prelinger", "commercial")
	inv := fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: snapshot, Captures: []fillercorpus.Capture{{CaptureID: captureID, Authority: "archive.org/prelinger", Collection: "prelinger", RoleHint: "commercial", SnapshotAt: snapshot, MaxRequests: 10, RequestsUsed: 1, MaxResponseBytes: 10_000, ResponseBytes: 100, MaxPredictedMediaBytes: 10_000, PredictedMediaBytes: int64(len(ids)) * 1024, MaxWallTimeMS: 1000, WallTimeMS: 10}}}
	for _, id := range ids {
		inv.Cases = append(inv.Cases, fillercorpus.InventoryCase{CaseID: fillercorpus.CaseID("archive.org/prelinger", id), CaptureID: captureID, Authority: "archive.org/prelinger", ItemID: id, Title: id, RoleHints: []string{"commercial"}, RightsAssertions: []string{"CC0"}, ItemURL: "https://archive.org/details/" + id, MetadataURL: "https://archive.org/metadata/" + id, MetadataRetrievedAt: snapshot, MetadataSHA256: strings.Repeat("a", 64), AllowedMediaHosts: []string{"archive.org", ".archive.org"}, Representation: fillercorpus.InventoryRepresentation{Name: id + ".mp4", URL: "https://archive.org/download/" + id + "/" + id + ".mp4", MIMEType: "video/mp4", Bytes: 1024}})
	}
	return inv
}

func TestRunWritesSpreadsheetSafeInertCSV(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := reviewInventory(snapshot, "formula-title")
	inv.Cases[0].Title = `=HYPERLINK("https://attacker.invalid")`
	raw, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inventoryPath, worksheetPath, csvPath := filepath.Join(dir, "inventory.json"), filepath.Join(dir, "worksheet.json"), filepath.Join(dir, "worksheet.csv")
	if err := os.WriteFile(inventoryPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--inventory", inventoryPath, "--out", worksheetPath, "--csv-out", csvPath, "--prepared-at", snapshot.Add(time.Minute).Format(time.RFC3339), "--min-items", "1", "--max-items", "1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}
	csvRaw, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvRaw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]string{}
	for i, name := range records[0] {
		columns[name] = records[1][i]
	}
	if columns["title"] != `'`+inv.Cases[0].Title {
		t.Fatalf("title = %q", columns["title"])
	}
	for _, field := range []string{"reviewer_id", "reviewed_at", "decision", "basis", "redistributable", "required_credit", "restrictions_json"} {
		if columns[field] != "" {
			t.Fatalf("%s unexpectedly grants authority", field)
		}
	}
}

func TestPrepareWorksheetIsDeterministicAndInert(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := reviewInventory(snapshot, "third", "first", "second")
	first, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Cases) != 2 {
		t.Fatalf("non-deterministic worksheet")
	}
	for _, row := range first.Cases {
		if row.Decision != "" || row.ReviewerID != "" || row.Redistributable {
			t.Fatalf("row %s grants authority", row.CaseID)
		}
	}
}

func TestPrepareWorksheetFailsBelowMinimum(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	if _, err := prepareWorksheet(reviewInventory(snapshot, "one"), strings.Repeat("f", 64), snapshot, 2, 10); err == nil {
		t.Fatal("undersized inventory was accepted")
	}
}
