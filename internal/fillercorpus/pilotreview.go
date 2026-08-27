package fillercorpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	PilotReviewSchemaVersion = 1
	pilotLaneMinimumYield    = 5
)

// PilotReviewWorksheet is an inert, deterministic presentation of a locked
// rights-yield pilot. It contains no review authority until every mutable CSV
// field has been validated by LockPilotReview.
type PilotReviewWorksheet struct {
	SchemaVersion int               `json:"schemaVersion"`
	PilotSHA256   string            `json:"pilotSha256"`
	SnapshotAt    time.Time         `json:"snapshotAt"`
	PilotLockedAt time.Time         `json:"pilotLockedAt"`
	PreparedAt    time.Time         `json:"preparedAt"`
	Instructions  []string          `json:"instructions"`
	Cases         []PilotReviewCase `json:"cases"`
}

type PilotReviewCase struct {
	Rank                int            `json:"rank"`
	PilotSHA256         string         `json:"pilotSha256"`
	Authority           string         `json:"authority"`
	ItemID              string         `json:"itemId"`
	Title               string         `json:"title"`
	RoleHints           []string       `json:"roleHints"`
	DiscoveryPath       []string       `json:"discoveryPath,omitempty"`
	ItemURL             string         `json:"itemUrl"`
	MetadataURL         string         `json:"metadataUrl"`
	MetadataRetrievedAt time.Time      `json:"metadataRetrievedAt"`
	MetadataSHA256      string         `json:"metadataSha256"`
	RightsAssertions    []string       `json:"rightsAssertions"`
	LicenseURL          string         `json:"licenseUrl,omitempty"`
	Representation      Representation `json:"representation"`
}

type PilotReviewResult struct {
	SchemaVersion          int                   `json:"schemaVersion"`
	Purpose                string                `json:"purpose"`
	PilotSHA256            string                `json:"pilotSha256"`
	WorksheetSHA256        string                `json:"worksheetSha256"`
	LockedAt               time.Time             `json:"lockedAt"`
	ReviewerID             string                `json:"reviewerId"`
	IndependentAttestation bool                  `json:"independentAttestation"`
	DownloadAuthority      bool                  `json:"downloadAuthority"`
	Lanes                  []PilotReviewLane     `json:"lanes"`
	Decisions              []PilotReviewDecision `json:"decisions"`
}

type PilotReviewLane struct {
	Authority           string `json:"authority"`
	CasesReviewed       int    `json:"casesReviewed"`
	RightsApproved      int    `json:"rightsApproved"`
	ProductRelevant     int    `json:"productRelevant"`
	ApprovedAndRelevant int    `json:"approvedAndRelevant"`
	MinimumRequired     int    `json:"minimumRequired"`
	QualifiedForAdapter bool   `json:"qualifiedForAdapter"`
}

type PilotReviewDecision struct {
	Authority         string    `json:"authority"`
	ItemID            string    `json:"itemId"`
	MetadataSHA256    string    `json:"metadataSha256"`
	ReviewedAt        time.Time `json:"reviewedAt"`
	RightsDecision    string    `json:"rightsDecision"`
	ProductRelevant   bool      `json:"productRelevant"`
	Basis             string    `json:"basis"`
	Redistributable   bool      `json:"redistributable"`
	RequiredCredit    string    `json:"requiredCredit,omitempty"`
	Restrictions      []string  `json:"restrictions"`
	DownloadAuthority bool      `json:"downloadAuthority"`
}

var PilotReviewCSVHeader = []string{
	"rank", "pilot_sha256", "authority", "item_id", "metadata_sha256", "title", "role_hints_json", "discovery_path_json",
	"item_url", "metadata_url", "metadata_retrieved_at", "rights_assertions_json", "license_url", "representation_name",
	"representation_url", "representation_mime_type", "representation_bytes", "representation_sha1", "representation_md5",
	"reviewer_id", "reviewed_at", "independent", "rights_decision", "product_relevant", "basis", "redistributable",
	"required_credit", "restrictions_json",
}

func PreparePilotReview(pilotRaw []byte, preparedAt time.Time) (PilotReviewWorksheet, error) {
	pilot, err := decodePilot(pilotRaw)
	if err != nil {
		return PilotReviewWorksheet{}, err
	}
	if failures := ValidatePilot(pilot); len(failures) > 0 {
		return PilotReviewWorksheet{}, fmt.Errorf("invalid pilot: %s", strings.Join(failures, "; "))
	}
	if preparedAt.Before(pilot.LockedAt) {
		return PilotReviewWorksheet{}, fmt.Errorf("preparation time predates the pilot lock")
	}
	digest := sha256Hex(pilotRaw)
	result := PilotReviewWorksheet{
		SchemaVersion: PilotReviewSchemaVersion,
		PilotSHA256:   digest,
		SnapshotAt:    pilot.SnapshotAt.UTC(),
		PilotLockedAt: pilot.LockedAt.UTC(),
		PreparedAt:    preparedAt.UTC(),
		Instructions: []string{
			"This worksheet qualifies source yield only; it is not media-download authority.",
			"Review every exact item, frozen metadata assertion, selected representation, embedded material, attribution duty, non-copyright restriction, and product relevance.",
			"Set independent=true only when the named reviewer is independent of candidate selection; blank or incomplete rows fail closed.",
		},
	}
	for _, lane := range pilot.Lanes {
		for _, candidate := range lane.Cases {
			result.Cases = append(result.Cases, PilotReviewCase{
				Rank: len(result.Cases) + 1, PilotSHA256: digest, Authority: lane.Authority,
				ItemID: candidate.ItemID, Title: candidate.Title, RoleHints: candidate.RoleHints,
				DiscoveryPath: candidate.DiscoveryPath, ItemURL: candidate.ItemURL, MetadataURL: candidate.MetadataURL,
				MetadataRetrievedAt: candidate.MetadataRetrievedAt, MetadataSHA256: candidate.MetadataSHA256,
				RightsAssertions: candidate.RightsAssertions, LicenseURL: candidate.LicenseURL, Representation: candidate.Representation,
			})
		}
	}
	return result, nil
}

func PilotReviewCSV(sheet PilotReviewWorksheet) ([]byte, error) {
	if err := validateWorksheetIdentity(sheet); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write(PilotReviewCSVHeader); err != nil {
		return nil, err
	}
	for _, row := range sheet.Cases {
		record := append(immutablePilotReviewRecord(row), "", "", "", "", "", "", "", "", "")
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func LockPilotReview(pilotRaw, worksheetRaw, completedCSV []byte, lockedAt time.Time) (PilotReviewResult, error) {
	var sheet PilotReviewWorksheet
	if err := decodeStrict(worksheetRaw, &sheet); err != nil {
		return PilotReviewResult{}, fmt.Errorf("decode worksheet: %w", err)
	}
	expected, err := PreparePilotReview(pilotRaw, sheet.PreparedAt)
	if err != nil {
		return PilotReviewResult{}, err
	}
	if !reflect.DeepEqual(sheet, expected) {
		return PilotReviewResult{}, fmt.Errorf("worksheet does not match the exact pilot and preparation contract")
	}
	if lockedAt.Before(sheet.PreparedAt) {
		return PilotReviewResult{}, fmt.Errorf("lock time predates worksheet preparation")
	}
	reader := csv.NewReader(bytes.NewReader(completedCSV))
	reader.FieldsPerRecord = len(PilotReviewCSVHeader)
	records, err := reader.ReadAll()
	if err != nil {
		return PilotReviewResult{}, fmt.Errorf("decode completed CSV: %w", err)
	}
	if len(records) != len(sheet.Cases)+1 || !reflect.DeepEqual(records[0], PilotReviewCSVHeader) {
		return PilotReviewResult{}, fmt.Errorf("completed CSV shape does not match the worksheet")
	}
	result := PilotReviewResult{
		SchemaVersion: PilotReviewSchemaVersion, Purpose: "pilot-source-qualification",
		PilotSHA256: sheet.PilotSHA256, WorksheetSHA256: sha256Hex(worksheetRaw), LockedAt: lockedAt.UTC(),
		IndependentAttestation: true, DownloadAuthority: false,
	}
	result.Lanes = make([]PilotReviewLane, len(PilotAuthorities))
	lanes := make(map[string]*PilotReviewLane, len(PilotAuthorities))
	for index, authority := range PilotAuthorities {
		result.Lanes[index] = PilotReviewLane{Authority: authority, MinimumRequired: pilotLaneMinimumYield}
		lanes[authority] = &result.Lanes[index]
	}
	seen := make(map[string]struct{}, len(sheet.Cases))
	for index, record := range records[1:] {
		row := sheet.Cases[index]
		key := row.Authority + "/" + row.ItemID
		if _, duplicate := seen[key]; duplicate {
			return PilotReviewResult{}, fmt.Errorf("duplicate completed review row %s", key)
		}
		seen[key] = struct{}{}
		if !reflect.DeepEqual(record[:19], immutablePilotReviewRecord(row)) {
			return PilotReviewResult{}, fmt.Errorf("completed review row %s changes immutable worksheet fields", key)
		}
		decision, reviewerID, err := parsePilotReviewDecision(row, record[19:], sheet.PreparedAt, lockedAt)
		if err != nil {
			return PilotReviewResult{}, fmt.Errorf("completed review row %s: %w", key, err)
		}
		if result.ReviewerID == "" {
			result.ReviewerID = reviewerID
		} else if result.ReviewerID != reviewerID {
			return PilotReviewResult{}, fmt.Errorf("completed review uses multiple reviewer identities")
		}
		result.Decisions = append(result.Decisions, decision)
		lane := lanes[row.Authority]
		lane.CasesReviewed++
		if decision.RightsDecision == "approved" {
			lane.RightsApproved++
		}
		if decision.ProductRelevant {
			lane.ProductRelevant++
		}
		if decision.RightsDecision == "approved" && decision.ProductRelevant {
			lane.ApprovedAndRelevant++
		}
	}
	for index := range result.Lanes {
		lane := &result.Lanes[index]
		lane.QualifiedForAdapter = lane.CasesReviewed == 10 && lane.ApprovedAndRelevant >= lane.MinimumRequired
	}
	return result, nil
}

func parsePilotReviewDecision(row PilotReviewCase, fields []string, preparedAt, lockedAt time.Time) (PilotReviewDecision, string, error) {
	reviewerID := strings.TrimSpace(fields[0])
	reviewedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[1]))
	if reviewerID == "" || err != nil || reviewedAt.Before(preparedAt) || reviewedAt.Before(row.MetadataRetrievedAt) || reviewedAt.After(lockedAt) {
		return PilotReviewDecision{}, "", fmt.Errorf("reviewer and review time must bind the completed review")
	}
	independent, err := strconv.ParseBool(strings.TrimSpace(fields[2]))
	if err != nil || !independent {
		return PilotReviewDecision{}, "", fmt.Errorf("independent must be explicitly true")
	}
	rightsDecision := strings.TrimSpace(fields[3])
	if rightsDecision != "approved" && rightsDecision != "held" {
		return PilotReviewDecision{}, "", fmt.Errorf("rights_decision must be approved or held")
	}
	productRelevant, err := strconv.ParseBool(strings.TrimSpace(fields[4]))
	if err != nil {
		return PilotReviewDecision{}, "", fmt.Errorf("product_relevant must be true or false")
	}
	basis := strings.TrimSpace(fields[5])
	if basis == "" {
		return PilotReviewDecision{}, "", fmt.Errorf("a reasoned basis is required")
	}
	redistributable, err := strconv.ParseBool(strings.TrimSpace(fields[6]))
	if err != nil {
		return PilotReviewDecision{}, "", fmt.Errorf("redistributable must be true or false")
	}
	if (rightsDecision == "approved") != redistributable {
		return PilotReviewDecision{}, "", fmt.Errorf("rights decision and redistribution assessment disagree")
	}
	requiredCredit := strings.TrimSpace(fields[7])
	if rightsDecision == "approved" && requiredCredit == "" {
		return PilotReviewDecision{}, "", fmt.Errorf("approved rows require an explicit credit or public-domain attribution statement")
	}
	var restrictions []string
	if err := json.Unmarshal([]byte(fields[8]), &restrictions); err != nil || restrictions == nil {
		return PilotReviewDecision{}, "", fmt.Errorf("restrictions_json must be a JSON string array")
	}
	return PilotReviewDecision{
		Authority: row.Authority, ItemID: row.ItemID, MetadataSHA256: row.MetadataSHA256,
		ReviewedAt: reviewedAt.UTC(), RightsDecision: rightsDecision, ProductRelevant: productRelevant,
		Basis: basis, Redistributable: redistributable, RequiredCredit: requiredCredit,
		Restrictions: restrictions, DownloadAuthority: false,
	}, reviewerID, nil
}

func immutablePilotReviewRecord(row PilotReviewCase) []string {
	return []string{
		strconv.Itoa(row.Rank), row.PilotSHA256, row.Authority, row.ItemID, row.MetadataSHA256,
		spreadsheetSafe(row.Title), jsonCell(row.RoleHints), jsonCell(row.DiscoveryPath), row.ItemURL, row.MetadataURL,
		row.MetadataRetrievedAt.UTC().Format(time.RFC3339), jsonCell(row.RightsAssertions), row.LicenseURL,
		spreadsheetSafe(row.Representation.Name), row.Representation.URL, spreadsheetSafe(row.Representation.MIMEType),
		strconv.FormatInt(row.Representation.Bytes, 10), row.Representation.SHA1, row.Representation.MD5,
	}
}

func validateWorksheetIdentity(sheet PilotReviewWorksheet) error {
	if sheet.SchemaVersion != PilotReviewSchemaVersion || !digest(sheet.PilotSHA256, 64) || sheet.SnapshotAt.IsZero() || sheet.PilotLockedAt.Before(sheet.SnapshotAt) || sheet.PreparedAt.Before(sheet.PilotLockedAt) || len(sheet.Cases) != 50 || len(sheet.Instructions) == 0 {
		return fmt.Errorf("pilot review worksheet identity is invalid")
	}
	return nil
}

func decodePilot(raw []byte) (Pilot, error) {
	var pilot Pilot
	if err := decodeStrict(raw, &pilot); err != nil {
		return Pilot{}, fmt.Errorf("decode pilot: %w", err)
	}
	return pilot, nil
}

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func sha256Hex(value []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(value))
}

func jsonCell(value any) string {
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
