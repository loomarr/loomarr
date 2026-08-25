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
	inventory := map[string]any{
		"schemaVersion": 1, "source": "archive.org", "collection": "classic_tv_commercials", "snapshotAt": retrievedAt,
		"cases": []any{map[string]any{
			"identifier": "soda-ad", "title": "Mountain Dew", "licenseUrl": "https://creativecommons.org/publicdomain/zero/1.0/",
			"itemUrl": "https://archive.org/details/soda-ad", "metadataUrl": "https://archive.org/metadata/soda-ad", "metadataRetrievedAt": retrievedAt, "metadataSha256": metadataDigest,
			"file": map[string]any{"name": "soda.mp4", "url": "https://archive.org/download/soda-ad/soda.mp4", "format": "MPEG4", "source": "original", "bytes": 1024},
		}},
	}
	inventoryRaw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	if err := os.WriteFile(inventoryPath, inventoryRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	worksheet := map[string]any{
		"schemaVersion": 1, "inventorySha256": digest, "source": "archive.org", "collection": "classic_tv_commercials",
		"snapshotAt": retrievedAt, "preparedAt": "2026-08-25T08:30:00Z", "minItems": 1, "maxItems": 1,
		"cases": []any{map[string]any{
			"rank": 1, "inventorySha256": digest, "identifier": "soda-ad", "title": "Mountain Dew",
			"licenseUrl": "https://creativecommons.org/publicdomain/zero/1.0/", "itemUrl": "https://archive.org/details/soda-ad",
			"metadataUrl": "https://archive.org/metadata/soda-ad", "metadataRetrievedAt": retrievedAt, "metadataSha256": metadataDigest,
			"file":       map[string]any{"name": "soda.mp4", "url": "https://archive.org/download/soda-ad/soda.mp4", "format": "MPEG4", "source": "original", "bytes": 1024},
			"reviewerId": "", "reviewedAt": "", "decision": "", "basis": "", "redistributable": false, "restrictions": []any{},
		}},
	}
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
	header := []string{
		"rank", "inventory_sha256", "identifier", "metadata_sha256", "title", "creator_json", "date", "license_url", "rights_json", "possible_copyright_status_json",
		"item_url", "metadata_url", "metadata_retrieved_at", "file_name", "file_url", "file_format", "file_source", "file_bytes", "file_sha1", "file_md5", "file_duration", "file_width", "file_height",
		"reviewer_id", "reviewed_at", "decision", "basis", "redistributable", "required_credit", "restrictions_json",
	}
	row := []string{
		"1", digest, "soda-ad", metadataDigest, "Mountain Dew", "null", "", "https://creativecommons.org/publicdomain/zero/1.0/", "null", "null",
		"https://archive.org/details/soda-ad", "https://archive.org/metadata/soda-ad", retrievedAt, "soda.mp4", "https://archive.org/download/soda-ad/soda.mp4", "MPEG4", "original", "1024", "", "", "", "", "",
		"rights-reviewer", reviewedAt, "approved", "CC0 dedication and item inspection permit redistribution.", "true", "", "[]",
	}
	if err := writer.WriteAll([][]string{header, row}); err != nil {
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
		Identifier      string    `json:"identifier"`
		MetadataSHA256  string    `json:"metadataSha256"`
		ReviewerID      string    `json:"reviewerId"`
		ReviewedAt      time.Time `json:"reviewedAt"`
		Decision        string    `json:"decision"`
		Redistributable bool      `json:"redistributable"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(approvalRaw), &approval); err != nil {
		t.Fatal(err)
	}
	if approval.InventorySHA256 != digest || approval.Identifier != "soda-ad" || approval.MetadataSHA256 != metadataDigest || approval.ReviewerID != "rights-reviewer" || approval.Decision != "approved" || !approval.Redistributable {
		t.Fatalf("unexpected locked approval: %+v", approval)
	}
}

func TestParseDecisionRejectsIncompleteOrInconsistentAuthority(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	lockedAt := retrievedAt.Add(2 * time.Hour)
	row := reviewRow{MetadataRetrievedAt: retrievedAt, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/"}
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
