package fillercorpus

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const RightsWorksheetSchemaVersion = 2

type RightsWorksheet struct {
	SchemaVersion   int               `json:"schemaVersion"`
	InventorySHA256 string            `json:"inventorySha256"`
	SnapshotAt      time.Time         `json:"snapshotAt"`
	PreparedAt      time.Time         `json:"preparedAt"`
	MinItems        int               `json:"minItems"`
	MaxItems        int               `json:"maxItems"`
	Instructions    []string          `json:"instructions"`
	Cases           []RightsReviewRow `json:"cases"`
}

type RightsReviewRow struct {
	Rank                    int                     `json:"rank"`
	InventorySHA256         string                  `json:"inventorySha256"`
	CaseID                  string                  `json:"caseId"`
	CaptureID               string                  `json:"captureId"`
	Authority               string                  `json:"authority"`
	ItemID                  string                  `json:"itemId"`
	Title                   string                  `json:"title"`
	RoleHints               []string                `json:"roleHints"`
	Collection              []string                `json:"collection,omitempty"`
	Creator                 []string                `json:"creator,omitempty"`
	Date                    string                  `json:"date,omitempty"`
	LicenseURL              string                  `json:"licenseUrl,omitempty"`
	RightsAssertions        []string                `json:"rightsAssertions"`
	PossibleCopyrightStatus []string                `json:"possibleCopyrightStatus,omitempty"`
	ItemURL                 string                  `json:"itemUrl"`
	MetadataURL             string                  `json:"metadataUrl"`
	MetadataRetrievedAt     time.Time               `json:"metadataRetrievedAt"`
	MetadataSHA256          string                  `json:"metadataSha256"`
	AllowedMediaHosts       []string                `json:"allowedMediaHosts"`
	Representation          InventoryRepresentation `json:"representation"`
	ReviewerID              string                  `json:"reviewerId"`
	ReviewedAt              string                  `json:"reviewedAt"`
	Decision                string                  `json:"decision"`
	Basis                   string                  `json:"basis"`
	Redistributable         bool                    `json:"redistributable"`
	RequiredCredit          string                  `json:"requiredCredit,omitempty"`
	Restrictions            []string                `json:"restrictions"`
}

var rightsReviewCSVHeader = []string{
	"rank", "inventory_sha256", "case_id", "capture_id", "authority", "item_id", "metadata_sha256", "title", "role_hints_json", "collection_json", "creator_json", "date",
	"license_url", "rights_assertions_json", "possible_copyright_status_json", "item_url", "metadata_url", "metadata_retrieved_at",
	"representation_name", "representation_url", "representation_mime_type", "representation_origin", "representation_bytes", "representation_sha256", "representation_sha1", "representation_md5", "allowed_media_hosts_json",
	"reviewer_id", "reviewed_at", "decision", "basis", "redistributable", "required_credit", "restrictions_json",
}

func RightsReviewCSVHeader() []string { return append([]string(nil), rightsReviewCSVHeader...) }

func RightsReviewRowFromCase(item InventoryCase) RightsReviewRow {
	return RightsReviewRow{
		CaseID: item.CaseID, CaptureID: item.CaptureID, Authority: item.Authority, ItemID: item.ItemID, Title: item.Title,
		RoleHints: item.RoleHints, Collection: item.Collection, Creator: item.Creator, Date: item.Date,
		LicenseURL: item.LicenseURL, RightsAssertions: item.RightsAssertions,
		PossibleCopyrightStatus: item.PossibleCopyrightStatus, ItemURL: item.ItemURL, MetadataURL: item.MetadataURL,
		MetadataRetrievedAt: item.MetadataRetrievedAt, MetadataSHA256: item.MetadataSHA256,
		AllowedMediaHosts: item.AllowedMediaHosts, Representation: item.Representation,
		Restrictions: []string{},
	}
}

func ImmutableRightsReviewRecord(row RightsReviewRow) []string {
	return []string{
		strconv.Itoa(row.Rank), row.InventorySHA256, row.CaseID, row.CaptureID, row.Authority, row.ItemID, row.MetadataSHA256, SpreadsheetSafe(row.Title), JSONCell(row.RoleHints), JSONCell(row.Collection), JSONCell(row.Creator), SpreadsheetSafe(row.Date),
		row.LicenseURL, JSONCell(row.RightsAssertions), JSONCell(row.PossibleCopyrightStatus), row.ItemURL, row.MetadataURL, row.MetadataRetrievedAt.UTC().Format(time.RFC3339),
		SpreadsheetSafe(row.Representation.Name), row.Representation.URL, SpreadsheetSafe(row.Representation.MIMEType), SpreadsheetSafe(row.Representation.Origin), strconv.FormatInt(row.Representation.Bytes, 10), row.Representation.SHA256, row.Representation.SHA1, row.Representation.MD5, JSONCell(row.AllowedMediaHosts),
	}
}

func JSONCell(value any) string {
	if values, ok := value.([]string); ok && len(values) == 0 {
		return "null"
	}
	data, _ := json.Marshal(value)
	return SpreadsheetSafe(string(data))
}

func SpreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
