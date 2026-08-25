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
	"strconv"
	"strings"
	"time"
)

type worksheet struct {
	SchemaVersion   int         `json:"schemaVersion"`
	InventorySHA256 string      `json:"inventorySha256"`
	Cases           []reviewRow `json:"cases"`
}

type inventory struct {
	SchemaVersion int         `json:"schemaVersion"`
	Source        string      `json:"source"`
	Cases         []reviewRow `json:"cases"`
}

type reviewRow struct {
	Rank                    int        `json:"rank"`
	InventorySHA256         string     `json:"inventorySha256"`
	Identifier              string     `json:"identifier"`
	Title                   string     `json:"title"`
	Creator                 []string   `json:"creator,omitempty"`
	Date                    string     `json:"date,omitempty"`
	LicenseURL              string     `json:"licenseUrl"`
	Rights                  []string   `json:"rights,omitempty"`
	PossibleCopyrightStatus []string   `json:"possibleCopyrightStatus,omitempty"`
	ItemURL                 string     `json:"itemUrl"`
	MetadataURL             string     `json:"metadataUrl"`
	MetadataRetrievedAt     time.Time  `json:"metadataRetrievedAt"`
	MetadataSHA256          string     `json:"metadataSha256"`
	File                    sourceFile `json:"file"`
}

type sourceFile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	Bytes    int64  `json:"bytes"`
	SHA1     string `json:"sha1,omitempty"`
	MD5      string `json:"md5,omitempty"`
	Duration string `json:"duration,omitempty"`
	Width    string `json:"width,omitempty"`
	Height   string `json:"height,omitempty"`
}

type rightsDecision struct {
	InventorySHA256 string    `json:"inventorySha256"`
	Identifier      string    `json:"identifier"`
	MetadataSHA256  string    `json:"metadataSha256"`
	ReviewerID      string    `json:"reviewerId"`
	ReviewedAt      time.Time `json:"reviewedAt"`
	Decision        string    `json:"decision"`
	Basis           string    `json:"basis"`
	Redistributable bool      `json:"redistributable"`
	RequiredCredit  string    `json:"requiredCredit,omitempty"`
	Restrictions    []string  `json:"restrictions,omitempty"`
}

var reviewCSVHeader = []string{
	"rank", "inventory_sha256", "identifier", "metadata_sha256", "title", "creator_json", "date",
	"license_url", "rights_json", "possible_copyright_status_json", "item_url", "metadata_url", "metadata_retrieved_at",
	"file_name", "file_url", "file_format", "file_source", "file_bytes", "file_sha1", "file_md5", "file_duration", "file_width", "file_height",
	"reviewer_id", "reviewed_at", "decision", "basis", "redistributable", "required_credit", "restrictions_json",
}

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

func lockDecisions(inventoryPath, worksheetPath, csvPath string, lockedAt time.Time) ([]rightsDecision, error) {
	inventoryRaw, err := os.ReadFile(inventoryPath)
	if err != nil {
		return nil, fmt.Errorf("read inventory: %w", err)
	}
	inventoryDigest := fmt.Sprintf("%x", sha256.Sum256(inventoryRaw))
	var inv inventory
	if err := json.Unmarshal(inventoryRaw, &inv); err != nil {
		return nil, fmt.Errorf("decode inventory: %w", err)
	}
	if inv.SchemaVersion != 1 || inv.Source != "archive.org" || len(inv.Cases) == 0 {
		return nil, fmt.Errorf("inventory identity is invalid")
	}
	raw, err := os.ReadFile(worksheetPath)
	if err != nil {
		return nil, fmt.Errorf("read worksheet: %w", err)
	}
	var sheet worksheet
	if err := json.Unmarshal(raw, &sheet); err != nil {
		return nil, fmt.Errorf("decode worksheet: %w", err)
	}
	if sheet.SchemaVersion != 1 || len(sheet.Cases) == 0 || sheet.InventorySHA256 != inventoryDigest {
		return nil, fmt.Errorf("worksheet identity is invalid")
	}
	inventoryByID := make(map[string]reviewRow, len(inv.Cases))
	for _, item := range inv.Cases {
		if _, duplicate := inventoryByID[item.Identifier]; duplicate {
			return nil, fmt.Errorf("duplicate inventory item %s", item.Identifier)
		}
		inventoryByID[item.Identifier] = item
	}
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("read completed CSV: %w", err)
	}
	defer func() { _ = file.Close() }()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = len(reviewCSVHeader)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode completed CSV: %w", err)
	}
	if len(records) == 0 || !reflect.DeepEqual(records[0], reviewCSVHeader) {
		return nil, fmt.Errorf("completed CSV header does not match the worksheet contract")
	}
	byID := make(map[string]reviewRow, len(sheet.Cases))
	for _, row := range sheet.Cases {
		if row.InventorySHA256 != sheet.InventorySHA256 || row.Identifier == "" || row.MetadataSHA256 == "" || row.MetadataRetrievedAt.IsZero() {
			return nil, fmt.Errorf("worksheet row %q has incomplete frozen identity", row.Identifier)
		}
		if _, duplicate := byID[row.Identifier]; duplicate {
			return nil, fmt.Errorf("duplicate worksheet row %s", row.Identifier)
		}
		item, ok := inventoryByID[row.Identifier]
		if !ok {
			return nil, fmt.Errorf("worksheet row %s is absent from the inventory", row.Identifier)
		}
		item.Rank = row.Rank
		item.InventorySHA256 = inventoryDigest
		if !reflect.DeepEqual(immutableRecord(row), immutableRecord(item)) {
			return nil, fmt.Errorf("worksheet row %s changes frozen inventory fields", row.Identifier)
		}
		byID[row.Identifier] = row
	}
	seen := make(map[string]struct{}, len(sheet.Cases))
	decisions := make([]rightsDecision, 0, len(sheet.Cases))
	for index, record := range records[1:] {
		identifier := record[2]
		row, ok := byID[identifier]
		if !ok {
			return nil, fmt.Errorf("CSV row %d item %q is absent from the worksheet", index+2, identifier)
		}
		if _, duplicate := seen[identifier]; duplicate {
			return nil, fmt.Errorf("duplicate completed review row %s", identifier)
		}
		seen[identifier] = struct{}{}
		if expected := immutableRecord(row); !reflect.DeepEqual(record[:23], expected) {
			return nil, fmt.Errorf("completed review row %s changes immutable worksheet fields", identifier)
		}
		decision, err := parseDecision(row, record[23:], lockedAt)
		if err != nil {
			return nil, fmt.Errorf("completed review row %s: %w", identifier, err)
		}
		decisions = append(decisions, decision)
	}
	if len(seen) != len(sheet.Cases) {
		return nil, fmt.Errorf("completed CSV covers %d of %d worksheet rows", len(seen), len(sheet.Cases))
	}
	return decisions, nil
}

func immutableRecord(row reviewRow) []string {
	return []string{
		strconv.Itoa(row.Rank), row.InventorySHA256, row.Identifier, row.MetadataSHA256, spreadsheetSafe(row.Title), jsonCell(row.Creator), spreadsheetSafe(row.Date),
		row.LicenseURL, jsonCell(row.Rights), jsonCell(row.PossibleCopyrightStatus), row.ItemURL, row.MetadataURL, row.MetadataRetrievedAt.UTC().Format(time.RFC3339),
		spreadsheetSafe(row.File.Name), row.File.URL, spreadsheetSafe(row.File.Format), spreadsheetSafe(row.File.Source), strconv.FormatInt(row.File.Bytes, 10), row.File.SHA1, row.File.MD5,
		spreadsheetSafe(row.File.Duration), spreadsheetSafe(row.File.Width), spreadsheetSafe(row.File.Height),
	}
}

func parseDecision(row reviewRow, fields []string, lockedAt time.Time) (rightsDecision, error) {
	reviewerID := strings.TrimSpace(fields[0])
	reviewedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[1]))
	if reviewerID == "" || err != nil || reviewedAt.Before(row.MetadataRetrievedAt) || reviewedAt.After(lockedAt) {
		return rightsDecision{}, fmt.Errorf("reviewer and review time must bind a completed review to the frozen metadata")
	}
	decision := strings.TrimSpace(fields[2])
	if decision != "approved" && decision != "held" {
		return rightsDecision{}, fmt.Errorf("decision must be approved or held")
	}
	basis := strings.TrimSpace(fields[3])
	if basis == "" {
		return rightsDecision{}, fmt.Errorf("a reasoned basis is required")
	}
	redistributable, err := strconv.ParseBool(strings.TrimSpace(fields[4]))
	if err != nil {
		return rightsDecision{}, fmt.Errorf("redistributable must be true or false")
	}
	requiredCredit := strings.TrimSpace(fields[5])
	var restrictions []string
	if err := json.Unmarshal([]byte(fields[6]), &restrictions); err != nil {
		return rightsDecision{}, fmt.Errorf("restrictions_json must be a JSON string array")
	}
	if decision == "approved" && !redistributable {
		return rightsDecision{}, fmt.Errorf("approved rows must explicitly be redistributable")
	}
	if decision == "held" && redistributable {
		return rightsDecision{}, fmt.Errorf("held rows cannot grant redistribution authority")
	}
	if decision == "approved" && requiresCredit(row.LicenseURL) && requiredCredit == "" {
		return rightsDecision{}, fmt.Errorf("the asserted license requires attribution")
	}
	return rightsDecision{
		InventorySHA256: row.InventorySHA256, Identifier: row.Identifier, MetadataSHA256: row.MetadataSHA256,
		ReviewerID: reviewerID, ReviewedAt: reviewedAt.UTC(), Decision: decision, Basis: basis,
		Redistributable: redistributable, RequiredCredit: requiredCredit, Restrictions: restrictions,
	}, nil
}

func requiresCredit(licenseURL string) bool {
	value := strings.ToLower(licenseURL)
	return strings.Contains(value, "/licenses/by/") || strings.Contains(value, "/licenses/by-sa/")
}

func jsonCell(value any) string {
	if values, ok := value.([]string); ok && len(values) == 0 {
		return "null"
	}
	data, _ := json.Marshal(value)
	return spreadsheetSafe(string(data))
}

func spreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func writeJSONL(path string, decisions []rightsDecision) error {
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
