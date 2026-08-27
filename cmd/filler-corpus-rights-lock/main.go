// Command filler-corpus-rights-lock validates a completed spreadsheet review
// against its inert JSON worksheet and emits downloader-compatible JSONL.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-rights-lock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inventoryPath := flags.String("inventory", "", "frozen source inventory JSON")
	worksheetPath := flags.String("worksheet", "", "inert rights worksheet JSON")
	csvPath := flags.String("completed-csv", "", "completed rights review CSV")
	outputPath := flags.String("approvals-out", "", "validated rights decisions JSONL")
	lockedAtText := flags.String("locked-at", "", "decision lock time in RFC3339 format")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *inventoryPath == "" || *worksheetPath == "" || *csvPath == "" || *outputPath == "" || *lockedAtText == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-lock: inventory, worksheet, completed CSV, approvals output, and lock time are required")
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-lock: parse --locked-at:", err)
		return 2
	}
	decisions, err := lockDecisions(*inventoryPath, *worksheetPath, *csvPath, lockedAt)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-lock:", err)
		return 1
	}
	if err := writeJSONL(*outputPath, decisions); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-rights-lock: write approvals:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-rights-lock: locked %d item-level rights decisions for inventory %s\n", len(decisions), decisions[0].InventorySHA256)
	return 0
}

func lockDecisions(inventoryPath, worksheetPath, csvPath string, lockedAt time.Time) ([]fillercorpus.RightsDecision, error) {
	inventoryRaw, err := os.ReadFile(inventoryPath)
	if err != nil {
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	inventoryDigest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	inv, err := fillercorpus.DecodeInventoryBytes(inventoryRaw)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(worksheetPath)
	if err != nil {
		return nil, fmt.Errorf("read worksheet: %w", err)
	}
	var sheet fillercorpus.RightsWorksheet
	if err := json.Unmarshal(raw, &sheet); err != nil {
		return nil, fmt.Errorf("decode worksheet: %w", err)
	}
	if sheet.SchemaVersion != fillercorpus.RightsWorksheetSchemaVersion || len(sheet.Cases) == 0 || sheet.InventorySHA256 != inventoryDigest ||
		!sheet.SnapshotAt.Equal(inv.SnapshotAt) ||
		sheet.PreparedAt.Before(sheet.SnapshotAt) || sheet.MinItems <= 0 || sheet.MaxItems < sheet.MinItems || len(inv.Cases) < sheet.MinItems {
		return nil, fmt.Errorf("worksheet identity is invalid")
	}
	expectedCases := make([]fillercorpus.RightsReviewRow, 0, len(inv.Cases))
	for _, item := range inv.Cases {
		expectedCases = append(expectedCases, fillercorpus.RightsReviewRowFromCase(item))
	}
	sort.Slice(expectedCases, func(i, j int) bool {
		return sha256Hex(inventoryDigest+"/"+expectedCases[i].CaseID) < sha256Hex(inventoryDigest+"/"+expectedCases[j].CaseID)
	})
	if len(expectedCases) > sheet.MaxItems {
		expectedCases = expectedCases[:sheet.MaxItems]
	}
	if len(sheet.Cases) != len(expectedCases) {
		return nil, fmt.Errorf("worksheet selection does not match its frozen inventory and bounds")
	}
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("read completed CSV: %w", err)
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	header := fillercorpus.RightsReviewCSVHeader()
	reader.FieldsPerRecord = len(header)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode completed CSV: %w", err)
	}
	if len(records) == 0 || !reflect.DeepEqual(records[0], header) {
		return nil, fmt.Errorf("completed CSV header does not match the worksheet contract")
	}
	byID := make(map[string]fillercorpus.RightsReviewRow, len(sheet.Cases))
	for index, row := range sheet.Cases {
		if row.InventorySHA256 != sheet.InventorySHA256 || row.CaseID == "" || row.MetadataSHA256 == "" || row.MetadataRetrievedAt.IsZero() {
			return nil, fmt.Errorf("worksheet row %q has incomplete frozen identity", row.CaseID)
		}
		if _, duplicate := byID[row.CaseID]; duplicate {
			return nil, fmt.Errorf("duplicate worksheet row %s", row.CaseID)
		}
		item := expectedCases[index]
		item.Rank = index + 1
		item.InventorySHA256 = inventoryDigest
		if row.Rank != index+1 || !reflect.DeepEqual(fillercorpus.ImmutableRightsReviewRecord(row), fillercorpus.ImmutableRightsReviewRecord(item)) {
			return nil, fmt.Errorf("worksheet row %s changes the deterministic frozen selection", row.CaseID)
		}
		byID[row.CaseID] = row
	}
	seen := make(map[string]struct{}, len(sheet.Cases))
	decisions := make([]fillercorpus.RightsDecision, 0, len(sheet.Cases))
	for index, record := range records[1:] {
		caseID := record[2]
		row, ok := byID[caseID]
		if !ok {
			return nil, fmt.Errorf("CSV row %d item %q is absent from the worksheet", index+2, caseID)
		}
		if _, duplicate := seen[caseID]; duplicate {
			return nil, fmt.Errorf("duplicate completed review row %s", caseID)
		}
		seen[caseID] = struct{}{}
		if expected := fillercorpus.ImmutableRightsReviewRecord(row); !reflect.DeepEqual(record[:len(expected)], expected) {
			return nil, fmt.Errorf("completed review row %s changes immutable worksheet fields", caseID)
		}
		decision, err := parseDecision(row, record[len(fillercorpus.ImmutableRightsReviewRecord(row)):], lockedAt)
		if err != nil {
			return nil, fmt.Errorf("completed review row %s: %w", caseID, err)
		}
		decisions = append(decisions, decision)
	}
	if len(seen) != len(sheet.Cases) {
		return nil, fmt.Errorf("completed CSV covers %d of %d worksheet rows", len(seen), len(sheet.Cases))
	}
	return decisions, nil
}

func sha256Hex(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func parseDecision(row fillercorpus.RightsReviewRow, fields []string, lockedAt time.Time) (fillercorpus.RightsDecision, error) {
	reviewerID := strings.TrimSpace(fields[0])
	reviewedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[1]))
	if reviewerID == "" || err != nil || reviewedAt.Before(row.MetadataRetrievedAt) || reviewedAt.After(lockedAt) {
		return fillercorpus.RightsDecision{}, fmt.Errorf("reviewer and review time must bind a completed review to the frozen metadata")
	}
	decision := strings.TrimSpace(fields[2])
	if decision != "approved" && decision != "held" {
		return fillercorpus.RightsDecision{}, fmt.Errorf("decision must be approved or held")
	}
	basis := strings.TrimSpace(fields[3])
	if basis == "" {
		return fillercorpus.RightsDecision{}, fmt.Errorf("a reasoned basis is required")
	}
	redistributable, err := strconv.ParseBool(strings.TrimSpace(fields[4]))
	if err != nil {
		return fillercorpus.RightsDecision{}, fmt.Errorf("redistributable must be true or false")
	}
	requiredCredit := strings.TrimSpace(fields[5])
	var restrictions []string
	if err := json.Unmarshal([]byte(fields[6]), &restrictions); err != nil {
		return fillercorpus.RightsDecision{}, fmt.Errorf("restrictions_json must be a JSON string array")
	}
	if decision == "approved" && !redistributable {
		return fillercorpus.RightsDecision{}, fmt.Errorf("approved rows must explicitly be redistributable")
	}
	if decision == "held" && redistributable {
		return fillercorpus.RightsDecision{}, fmt.Errorf("held rows cannot grant redistribution authority")
	}
	if decision == "approved" && requiresCredit(row.LicenseURL) && requiredCredit == "" {
		return fillercorpus.RightsDecision{}, fmt.Errorf("the asserted license requires attribution")
	}
	return fillercorpus.RightsDecision{
		InventorySHA256: row.InventorySHA256, CaseID: row.CaseID, CaptureID: row.CaptureID, Authority: row.Authority, ItemID: row.ItemID, MetadataSHA256: row.MetadataSHA256,
		ReviewerID: reviewerID, ReviewedAt: reviewedAt.UTC(), Decision: decision, Basis: basis,
		Redistributable: redistributable, RequiredCredit: requiredCredit, Restrictions: restrictions,
	}, nil
}

func requiresCredit(licenseURL string) bool {
	value := strings.ToLower(licenseURL)
	return strings.Contains(value, "/licenses/by/") || strings.Contains(value, "/licenses/by-sa/")
}

func writeJSONL(path string, decisions []fillercorpus.RightsDecision) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-rights-lock-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	for _, decision := range decisions {
		if err := encoder.Encode(decision); err != nil {
			return err
		}
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
