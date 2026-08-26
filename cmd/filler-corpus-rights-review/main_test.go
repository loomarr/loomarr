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
)

func TestRunWritesSpreadsheetSafeInertCSV(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{SchemaVersion: 1, Source: "archive.org", Collection: "classic_tv_commercials", SnapshotAt: snapshot, Cases: []candidate{{
		Identifier: "formula-title", Title: `=HYPERLINK("https://attacker.invalid")`, MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: snapshot,
		File: sourceFile{Name: "clip.mp4", URL: "https://archive.org/download/formula-title/clip.mp4", Format: "MPEG4", Source: "original", Bytes: 1024},
	}}}
	raw, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "inventory.json")
	worksheetPath := filepath.Join(dir, "worksheet.json")
	csvPath := filepath.Join(dir, "worksheet.csv")
	if err := os.WriteFile(inventoryPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--inventory", inventoryPath,
		"--out", worksheetPath,
		"--csv-out", csvPath,
		"--prepared-at", snapshot.Add(time.Minute).Format(time.RFC3339),
		"--min-items", "1",
		"--max-items", "1",
	}, &stdout, &stderr)
	if code != 0 {
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
	if len(records) != 2 {
		t.Fatalf("CSV rows = %d, want header plus one review row", len(records))
	}
	columns := map[string]string{}
	for i, name := range records[0] {
		columns[name] = records[1][i]
	}
	if columns["title"] != `'`+inv.Cases[0].Title {
		t.Fatalf("title = %q, want spreadsheet-safe literal", columns["title"])
	}
	for _, field := range []string{"reviewer_id", "reviewed_at", "decision", "basis", "redistributable", "required_credit", "restrictions_json"} {
		if columns[field] != "" {
			t.Fatalf("%s unexpectedly grants authority: %q", field, columns[field])
		}
	}
}

func TestPrepareWorksheetIsDeterministicAndInert(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{SchemaVersion: 1, Source: "archive.org", Collection: "classic_tv_commercials", SnapshotAt: snapshot}
	for _, id := range []string{"third", "first", "second"} {
		inv.Cases = append(inv.Cases, candidate{
			Identifier: id, Title: id, MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: snapshot,
			File: sourceFile{Name: id + ".mp4", Bytes: 1024},
		})
	}
	first, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot.Add(time.Minute), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical input produced different worksheets")
	}
	if len(first.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(first.Cases))
	}
	for _, row := range first.Cases {
		if row.Decision != "" || row.ReviewerID != "" || row.Redistributable {
			t.Fatalf("row %s unexpectedly grants authority: %+v", row.Identifier, row)
		}
	}
}

func TestPrepareWorksheetFailsBelowMinimum(t *testing.T) {
	snapshot := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	inv := inventory{SchemaVersion: 1, Source: "archive.org", Collection: "small", SnapshotAt: snapshot, Cases: []candidate{{
		Identifier: "one", MetadataSHA256: strings.Repeat("a", 64), MetadataRetrievedAt: snapshot,
		File: sourceFile{Name: "one.mp4", Bytes: 1},
	}}}
	if _, err := prepareWorksheet(inv, strings.Repeat("f", 64), snapshot, 2, 10); err == nil {
		t.Fatal("undersized inventory was accepted")
	}
}
