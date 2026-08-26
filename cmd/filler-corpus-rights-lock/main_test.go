package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestRunLocksCompleteSpreadsheetReviewToDownloaderJSONL(t *testing.T) {
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "inventory.json")
	worksheetPath := filepath.Join(dir, "worksheet.json")
	csvPath := filepath.Join(dir, "completed.csv")
	approvalsPath := filepath.Join(dir, "approvals.jsonl")
	metadataDigest := strings.Repeat("a", 64)
	retrievedAt := "2026-08-25T08:00:00Z"
	reviewedAt := "2026-08-25T09:00:00Z"
	retrievedTime, _ := time.Parse(time.RFC3339, retrievedAt)
	authority := "archive.org/prelinger"
	captureID := fillercorpus.NewCaptureID(authority, "prelinger", "commercial")
	item := fillercorpus.InventoryCase{CaseID: fillercorpus.CaseID(authority, "soda-ad"), CaptureID: captureID, Authority: authority, ItemID: "soda-ad", Title: "Mountain Dew", RoleHints: []string{"commercial"}, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/", RightsAssertions: []string{"CC0"}, ItemURL: "https://archive.org/details/soda-ad", MetadataURL: "https://archive.org/metadata/soda-ad", MetadataRetrievedAt: retrievedTime, MetadataSHA256: metadataDigest, AllowedMediaHosts: []string{"archive.org", ".archive.org"}, Representation: fillercorpus.InventoryRepresentation{Name: "soda.mp4", URL: "https://archive.org/download/soda-ad/soda.mp4", MIMEType: "video/mp4", Origin: "original", Bytes: 1024}}
	inventory := fillercorpus.Inventory{SchemaVersion: fillercorpus.InventorySchemaVersion, SnapshotAt: retrievedTime, Captures: []fillercorpus.Capture{{CaptureID: captureID, Authority: authority, Collection: "prelinger", RoleHint: "commercial", SnapshotAt: retrievedTime, MaxRequests: 2, RequestsUsed: 1, MaxResponseBytes: 2048, ResponseBytes: 100, MaxPredictedMediaBytes: 2048, PredictedMediaBytes: 1024, MaxWallTimeMS: 1000, WallTimeMS: 10}}, Cases: []fillercorpus.InventoryCase{item}}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	if err := os.WriteFile(inventoryPath, inventoryRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	rowValue := fillercorpus.RightsReviewRowFromCase(item)
	rowValue.Rank, rowValue.InventorySHA256 = 1, digest
	worksheet := fillercorpus.RightsWorksheet{SchemaVersion: fillercorpus.RightsWorksheetSchemaVersion, InventorySHA256: digest, SnapshotAt: retrievedTime, PreparedAt: retrievedTime.Add(30 * time.Minute), MinItems: 1, MaxItems: 1, Cases: []fillercorpus.RightsReviewRow{rowValue}}
	raw, err := json.Marshal(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worksheetPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	completed := append(fillercorpus.ImmutableRightsReviewRecord(rowValue), "rights-reviewer", reviewedAt, "approved", "CC0 dedication and item inspection permit redistribution.", "true", "", "[]")
	if err := writer.WriteAll([][]string{fillercorpus.RightsReviewCSVHeader(), completed}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--inventory", inventoryPath,
		"--worksheet", worksheetPath,
		"--completed-csv", csvPath,
		"--approvals-out", approvalsPath,
		"--locked-at", "2026-08-25T10:00:00Z",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}
	approvalRaw, err := os.ReadFile(approvalsPath)
	if err != nil {
		t.Fatal(err)
	}
	var approval struct {
		InventorySHA256 string    `json:"inventorySha256"`
		CaseID          string    `json:"caseId"`
		MetadataSHA256  string    `json:"metadataSha256"`
		ReviewerID      string    `json:"reviewerId"`
		ReviewedAt      time.Time `json:"reviewedAt"`
		Decision        string    `json:"decision"`
		Redistributable bool      `json:"redistributable"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(approvalRaw), &approval); err != nil {
		t.Fatal(err)
	}
	if approval.InventorySHA256 != digest || approval.CaseID != item.CaseID || approval.MetadataSHA256 != metadataDigest || approval.ReviewerID != "rights-reviewer" || approval.Decision != "approved" || !approval.Redistributable {
		t.Fatalf("unexpected locked approval: %+v", approval)
	}
}

func TestParseDecisionRejectsIncompleteOrInconsistentAuthority(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	lockedAt := retrievedAt.Add(2 * time.Hour)
	row := fillercorpus.RightsReviewRow{MetadataRetrievedAt: retrievedAt, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/"}
	valid := []string{"rights-reviewer", retrievedAt.Add(time.Hour).Format(time.RFC3339), "approved", "Exact item inspection confirms CC0 redistribution.", "true", "", "[]"}
	tests := map[string]func([]string){
		"missing reviewer":           func(fields []string) { fields[0] = "" },
		"review before metadata":     func(fields []string) { fields[1] = retrievedAt.Add(-time.Minute).Format(time.RFC3339) },
		"review after lock":          func(fields []string) { fields[1] = lockedAt.Add(time.Minute).Format(time.RFC3339) },
		"unknown decision":           func(fields []string) { fields[2] = "maybe" },
		"missing basis":              func(fields []string) { fields[3] = "" },
		"invalid redistribution":     func(fields []string) { fields[4] = "yes" },
		"approval without authority": func(fields []string) { fields[4] = "false" },
		"malformed restrictions":     func(fields []string) { fields[6] = "none" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fields := append([]string(nil), valid...)
			mutate(fields)
			if _, err := parseDecision(row, fields, lockedAt); err == nil {
				t.Fatal("invalid rights authority was accepted")
			}
		})
	}
	t.Run("held row grants redistribution", func(t *testing.T) {
		fields := append([]string(nil), valid...)
		fields[2] = "held"
		if _, err := parseDecision(row, fields, lockedAt); err == nil {
			t.Fatal("held row granted redistribution authority")
		}
	})
	t.Run("attribution license lacks credit", func(t *testing.T) {
		byRow := row
		byRow.LicenseURL = "https://creativecommons.org/licenses/by/4.0/"
		if _, err := parseDecision(byRow, valid, lockedAt); err == nil {
			t.Fatal("attribution-bearing approval without credit was accepted")
		}
	})
}
