package fillercorpus

import "time"

const (
	DownloadLedgerLegacySchemaVersion = 1
	DownloadLedgerSchemaVersion       = 2
)

// DownloadLedger is the content-addressed acquisition receipt shared by the
// downloader and every later verifier. Profile is absent only in historical
// schema-v1 development receipts; schema v2 always names its authority scope.
type DownloadLedger struct {
	SchemaVersion   int            `json:"schemaVersion"`
	Profile         string         `json:"profile,omitempty"`
	InventorySHA256 string         `json:"inventorySha256"`
	GeneratedAt     time.Time      `json:"generatedAt"`
	MaxRequests     int            `json:"maxRequests"`
	RequestsUsed    int            `json:"requestsUsed"`
	MaxItems        int            `json:"maxItems"`
	MaxBytes        int64          `json:"maxBytes"`
	Bytes           int64          `json:"bytes"`
	Cases           []DownloadCase `json:"cases"`
}

type DownloadCase struct {
	CaseID              string                  `json:"caseId"`
	Authority           string                  `json:"authority"`
	ItemID              string                  `json:"itemId"`
	LicenseURL          string                  `json:"licenseUrl"`
	ItemURL             string                  `json:"itemUrl"`
	MetadataURL         string                  `json:"metadataUrl"`
	MetadataRetrievedAt time.Time               `json:"metadataRetrievedAt"`
	MetadataSHA256      string                  `json:"metadataSha256"`
	Representation      InventoryRepresentation `json:"representation"`
	LocalFile           string                  `json:"localFile"`
	ContentSHA256       string                  `json:"contentSha256"`
	Approval            RightsDecision          `json:"approval"`
	VerifiedAt          time.Time               `json:"verifiedAt"`
}
