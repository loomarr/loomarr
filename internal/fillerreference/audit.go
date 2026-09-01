// Package fillerreference owns the deterministic pre-screen for the production-ready
// filler reference cohort. It never edits media or claims human editorial acceptance.
package fillerreference

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

const (
	AuditSchemaVersion    = 3
	ContractVersion       = "filler-reference-cohort-2026-08-31-v3"
	PacketEvidenceVersion = "filler-evidence-v1"
	ContentReviewKind     = "filler_reference_content_review"
)

type Disposition string

const (
	DispositionCandidate Disposition = "candidate"
	DispositionHold      Disposition = "hold"
	DispositionExclude   Disposition = "exclude"
)

type ReasonCode string

const (
	ReasonSemanticInvalid          ReasonCode = "semantic_invalid"
	ReasonSemanticAmbiguous        ReasonCode = "semantic_ambiguous"
	ReasonRoleNotFiller            ReasonCode = "role_not_filler"
	ReasonRoleMissing              ReasonCode = "role_missing"
	ReasonMediaUnusable            ReasonCode = "media_unusable"
	ReasonDurationTooShort         ReasonCode = "duration_too_short"
	ReasonMediaEvidenceMissing     ReasonCode = "media_evidence_missing"
	ReasonSourceIneligible         ReasonCode = "source_ineligible"
	ReasonSourceEvidenceMissing    ReasonCode = "source_evidence_missing"
	ReasonSourceCoverageIncomplete ReasonCode = "source_coverage_incomplete"
	ReasonCommercialProductMissing ReasonCode = "commercial_product_missing"
	ReasonNonBroadcastMaterial     ReasonCode = "non_broadcast_material"
	ReasonTaxonomyUnresolved       ReasonCode = "taxonomy_unresolved"
)

type ContentReviewArtifact struct {
	SchemaVersion        int              `json:"schemaVersion"`
	Kind                 string           `json:"kind"`
	ContractVersion      string           `json:"contractVersion"`
	ReviewerID           string           `json:"reviewerId"`
	ReviewedAt           time.Time        `json:"reviewedAt"`
	SourceManifestSHA256 string           `json:"sourceManifestSha256"`
	Findings             []ContentFinding `json:"findings"`
}

type ContentFinding struct {
	ContentSHA256 string     `json:"contentSha256"`
	Disposition   string     `json:"disposition"`
	ReasonCode    ReasonCode `json:"reasonCode"`
	Detail        string     `json:"detail"`
	EvidenceRefs  []string   `json:"evidenceRefs"`
}

type AppliedContentFinding struct {
	ReviewerID   string     `json:"reviewerId"`
	ReviewedAt   time.Time  `json:"reviewedAt"`
	Disposition  string     `json:"disposition"`
	ReasonCode   ReasonCode `json:"reasonCode"`
	Detail       string     `json:"detail"`
	EvidenceRefs []string   `json:"evidenceRefs"`
}

type MappingArtifact struct {
	SchemaVersion            int              `json:"schemaVersion"`
	MappingVersion           string           `json:"mappingVersion"`
	SourceManifestSHA256     string           `json:"sourceManifestSha256"`
	SourceProductAssignments int              `json:"sourceProductAssignments"`
	UniqueReviewerLabels     int              `json:"uniqueReviewerLabels"`
	MappedAssignments        int              `json:"mappedAssignments"`
	UnmappedAssignments      int              `json:"unmappedAssignments"`
	ProductionCategories     []string         `json:"productionCategories"`
	Mappings                 []ProductMapping `json:"mappings"`
}

type ProductMapping struct {
	ReviewerLabel        string   `json:"reviewerLabel"`
	Occurrences          int      `json:"occurrences"`
	ProductionCategories []string `json:"productionCategories"`
	Basis                string   `json:"basis"`
}

type InputIdentity struct {
	ManifestSHA256       string `json:"manifestSha256"`
	PacketsSHA256        string `json:"packetsSha256"`
	MappingSHA256        string `json:"mappingSha256"`
	DownloadLedgerSHA256 string `json:"downloadLedgerSha256"`
	ContentReviewSHA256  string `json:"contentReviewSha256"`
}

// RawAuditInputs are the exact artifact bytes bound into an audit. BuildAudit
// clones, strictly decodes, and hashes these bytes itself; callers cannot pair
// typed values from one artifact with asserted identities from another.
type RawAuditInputs struct {
	Manifest       []byte
	Packets        []byte
	Mapping        []byte
	DownloadLedger []byte
	ContentReview  []byte
}

type DownloadLedger struct {
	SchemaVersion   int            `json:"schemaVersion"`
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
	CaseID              string                               `json:"caseId"`
	Authority           string                               `json:"authority"`
	ItemID              string                               `json:"itemId"`
	LicenseURL          string                               `json:"licenseUrl"`
	ItemURL             string                               `json:"itemUrl"`
	MetadataURL         string                               `json:"metadataUrl"`
	MetadataRetrievedAt time.Time                            `json:"metadataRetrievedAt"`
	MetadataSHA256      string                               `json:"metadataSha256"`
	Representation      fillercorpus.InventoryRepresentation `json:"representation"`
	LocalFile           string                               `json:"localFile"`
	ContentSHA256       string                               `json:"contentSha256"`
	Approval            fillercorpus.RightsDecision          `json:"approval"`
	VerifiedAt          time.Time                            `json:"verifiedAt"`
}

type Summary struct {
	Cases      int            `json:"cases"`
	Candidates int            `json:"candidates"`
	Holds      int            `json:"holds"`
	Excluded   int            `json:"excluded"`
	ByRole     map[string]int `json:"byRole"`
	ByReason   map[string]int `json:"byReason"`
	BySource   map[string]int `json:"candidateBySource"`
	Unresolved map[string]int `json:"unresolvedTaxonomyValues"`
	Mapping    string         `json:"mappingVersion"`
	Contract   string         `json:"contractVersion"`
}

type Audit struct {
	SchemaVersion int           `json:"schemaVersion"`
	Contract      string        `json:"contractVersion"`
	GeneratedAt   time.Time     `json:"generatedAt"`
	Inputs        InputIdentity `json:"inputs"`
	Summary       Summary       `json:"summary"`
	Cases         []Case        `json:"cases"`
}

type Case struct {
	CaseID                   string                 `json:"caseId"`
	ContentSHA256            string                 `json:"contentSha256"`
	Source                   string                 `json:"source"`
	SourceItemID             string                 `json:"sourceItemId"`
	SourceFilename           string                 `json:"sourceFilename"`
	SourceLocalFile          string                 `json:"sourceLocalFile"`
	SemanticTruth            fillereval.Truth       `json:"semanticTruth"`
	ContentRole              string                 `json:"contentRole,omitempty"`
	Disposition              Disposition            `json:"disposition"`
	ReasonCodes              []ReasonCode           `json:"reasonCodes,omitempty"`
	ReadinessGrade           string                 `json:"readinessGrade"`
	NeedsFullVideoInspection bool                   `json:"needsFullVideoInspection"`
	Media                    MediaSummary           `json:"media"`
	ProposedProductionTags   []string               `json:"proposedProductionTags,omitempty"`
	UnresolvedTaxonomyValues []string               `json:"unresolvedTaxonomyValues,omitempty"`
	ProposedBrandValues      []string               `json:"proposedBrandValues,omitempty"`
	ReviewerTaxonomy         map[string][]string    `json:"reviewerTaxonomy,omitempty"`
	ContentFinding           *AppliedContentFinding `json:"contentFinding,omitempty"`
}

type MediaSummary struct {
	SourceDurationMS        int64  `json:"sourceDurationMs"`
	SegmentStartMS          int64  `json:"segmentStartMs"`
	SegmentDurationMS       int64  `json:"segmentDurationMs"`
	EvidenceVideoDurationMS int64  `json:"evidenceVideoDurationMs,omitempty"`
	Width                   int    `json:"width,omitempty"`
	Height                  int    `json:"height,omitempty"`
	BlackPercent            int    `json:"blackPercent"`
	SilencePercent          int    `json:"silencePercent"`
	NoVideo                 bool   `json:"noVideo"`
	NoAudio                 bool   `json:"noAudio"`
	EvidenceVideoPath       string `json:"evidenceVideoPath,omitempty"`
	EvidenceVideoSHA256     string `json:"evidenceVideoSha256,omitempty"`
	EvidenceVideoBytes      int64  `json:"evidenceVideoBytes,omitempty"`
	evidenceVideoValid      bool
}

// BuildAudit creates a stable, read-only screen. Candidate means only that no deterministic
// exclusion or hold was found; every candidate still needs inspection of the complete video.
func BuildAudit(raw RawAuditInputs, generatedAt time.Time) (Audit, error) {
	raw = RawAuditInputs{
		Manifest: bytes.Clone(raw.Manifest), Packets: bytes.Clone(raw.Packets),
		Mapping: bytes.Clone(raw.Mapping), DownloadLedger: bytes.Clone(raw.DownloadLedger),
		ContentReview: bytes.Clone(raw.ContentReview),
	}
	manifest, err := decodeStrictJSON[fillereval.Manifest](raw.Manifest)
	if err != nil {
		return Audit{}, fmt.Errorf("manifest: %w", err)
	}
	packets, err := decodePackets(raw.Packets)
	if err != nil {
		return Audit{}, fmt.Errorf("packets: %w", err)
	}
	mapping, err := decodeStrictJSON[MappingArtifact](raw.Mapping)
	if err != nil {
		return Audit{}, fmt.Errorf("mapping: %w", err)
	}
	downloads, err := DecodeDownloadLedger(raw.DownloadLedger)
	if err != nil {
		return Audit{}, fmt.Errorf("download ledger: %w", err)
	}
	contentReview, err := decodeStrictJSON[ContentReviewArtifact](raw.ContentReview)
	if err != nil {
		return Audit{}, fmt.Errorf("content review: %w", err)
	}
	return buildAudit(manifest, packets, mapping, downloads, contentReview, InputIdentity{
		ManifestSHA256: SHA256(raw.Manifest), PacketsSHA256: SHA256(raw.Packets),
		MappingSHA256: SHA256(raw.Mapping), DownloadLedgerSHA256: SHA256(raw.DownloadLedger),
		ContentReviewSHA256: SHA256(raw.ContentReview),
	}, generatedAt)
}

func buildAudit(manifest fillereval.Manifest, packets map[string]fillerbakeoff.Packet, mapping MappingArtifact, downloads DownloadLedger, contentReview ContentReviewArtifact, inputs InputIdentity, generatedAt time.Time) (Audit, error) {
	if len(manifest.Cases) != 300 {
		return Audit{}, fmt.Errorf("reference audit requires one locked 300-case development seed")
	}
	if failures := fillereval.ValidateDevelopmentRun(manifest); len(failures) != 0 {
		return Audit{}, fmt.Errorf("reference audit development manifest is invalid: %s", strings.Join(failures, "; "))
	}
	if !validSHA256(inputs.ManifestSHA256) || !validSHA256(inputs.PacketsSHA256) || !validSHA256(inputs.MappingSHA256) || !validSHA256(inputs.DownloadLedgerSHA256) || !validSHA256(inputs.ContentReviewSHA256) || mapping.SchemaVersion != 1 || strings.TrimSpace(mapping.MappingVersion) == "" || mapping.SourceManifestSHA256 != inputs.ManifestSHA256 || mapping.UnmappedAssignments != 0 || mapping.SourceProductAssignments != mapping.MappedAssignments {
		return Audit{}, fmt.Errorf("reference audit input identities or mapping coverage are incomplete")
	}
	contentFindings, err := validateContentReview(manifest, contentReview, inputs.ManifestSHA256, generatedAt)
	if err != nil {
		return Audit{}, err
	}
	if err := validatePacketCaseSet(manifest, packets); err != nil {
		return Audit{}, err
	}
	downloadByID, err := validateDownloads(manifest, downloads)
	if err != nil {
		return Audit{}, err
	}
	mapped, err := validateMapping(manifest, mapping)
	if err != nil {
		return Audit{}, err
	}
	forest := taxonomy.New(taxonomy.SeedForest())
	cases := slices.Clone(manifest.Cases)
	slices.SortFunc(cases, func(a, b fillereval.Case) int { return strings.Compare(a.ID, b.ID) })
	audit := Audit{
		SchemaVersion: AuditSchemaVersion,
		Contract:      ContractVersion,
		GeneratedAt:   generatedAt.UTC(),
		Inputs:        inputs,
		Summary: Summary{
			Cases: len(cases), ByRole: map[string]int{}, ByReason: map[string]int{}, BySource: map[string]int{},
			Unresolved: map[string]int{}, Mapping: mapping.MappingVersion, Contract: ContractVersion,
		},
		Cases: make([]Case, 0, len(cases)),
	}
	for _, item := range cases {
		packet, ok := packets[item.ID]
		if !ok {
			return Audit{}, fmt.Errorf("case %q has no content-bound packet", item.ID)
		}
		if err := fillerbakeoff.ValidatePacketReferenceBinding(item, packet, PacketEvidenceVersion); err != nil {
			return Audit{}, err
		}
		download, ok := downloadByID[item.ID]
		if !ok {
			return Audit{}, fmt.Errorf("case %q has no content-bound acquired source", item.ID)
		}
		row, err := screen(item, packet, download, mapped, forest, contentFindings[item.ContentSHA256])
		if err != nil {
			return Audit{}, fmt.Errorf("case %q: %w", item.ID, err)
		}
		audit.Cases = append(audit.Cases, row)
		audit.Summary.ByRole[valueOr(row.ContentRole, "unresolved")]++
		for _, reason := range row.ReasonCodes {
			audit.Summary.ByReason[string(reason)]++
		}
		for _, unresolved := range row.UnresolvedTaxonomyValues {
			audit.Summary.Unresolved[unresolved]++
		}
		switch row.Disposition {
		case DispositionCandidate:
			audit.Summary.Candidates++
			audit.Summary.BySource[row.Source]++
		case DispositionHold:
			audit.Summary.Holds++
		case DispositionExclude:
			audit.Summary.Excluded++
		}
	}
	return audit, nil
}

// DecodeDownloadLedger applies the same hostile-input decoder BuildAudit uses,
// allowing the CLI to verify source files against the exact raw ledger bytes.
func DecodeDownloadLedger(data []byte) (DownloadLedger, error) {
	return decodeStrictJSON[DownloadLedger](data)
}

func decodeStrictJSON[T any](data []byte) (T, error) {
	var value T
	if err := rejectDuplicateObjectKeys(data); err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("trailing JSON value")
		}
		return value, err
	}
	return value, nil
}

func decodePackets(data []byte) (map[string]fillerbakeoff.Packet, error) {
	packets := make(map[string]fillerbakeoff.Packet)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		if err := rejectDuplicateObjectKeys(raw); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		packet, err := fillerbakeoff.DecodePacket(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if _, exists := packets[packet.CaseID]; exists {
			return nil, fmt.Errorf("line %d: duplicate case %q", line, packet.CaseID)
		}
		packets[packet.CaseID] = packet
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return packets, nil
}

func rejectDuplicateObjectKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object member name is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim(map[json.Delim]json.Delim{'{': '}', '[': ']'}[delim]) {
		return fmt.Errorf("mismatched JSON delimiter")
	}
	return nil
}

func validatePacketCaseSet(manifest fillereval.Manifest, packets map[string]fillerbakeoff.Packet) error {
	if len(packets) != len(manifest.Cases) {
		return fmt.Errorf("reference audit has %d packets, want %d", len(packets), len(manifest.Cases))
	}
	for _, item := range manifest.Cases {
		if _, ok := packets[item.ID]; !ok {
			return fmt.Errorf("reference audit packet set is missing case %q", item.ID)
		}
	}
	return nil
}

func validateContentReview(manifest fillereval.Manifest, review ContentReviewArtifact, manifestSHA256 string, generatedAt time.Time) (map[string]*AppliedContentFinding, error) {
	if review.SchemaVersion != 1 || review.Kind != ContentReviewKind || review.ContractVersion != ContractVersion || strings.TrimSpace(review.ReviewerID) == "" || review.ReviewedAt.IsZero() || review.ReviewedAt.After(generatedAt) || review.SourceManifestSHA256 != manifestSHA256 || len(review.Findings) == 0 {
		return nil, fmt.Errorf("content review identity, authority, or time is invalid")
	}
	byContent := make(map[string]fillereval.Case, len(manifest.Cases))
	for _, item := range manifest.Cases {
		if _, duplicate := byContent[item.ContentSHA256]; duplicate {
			return nil, fmt.Errorf("manifest repeats content identity %q", item.ContentSHA256)
		}
		byContent[item.ContentSHA256] = item
	}
	result := make(map[string]*AppliedContentFinding, len(review.Findings))
	for _, finding := range review.Findings {
		item, ok := byContent[finding.ContentSHA256]
		if !ok || !validSHA256(finding.ContentSHA256) {
			return nil, fmt.Errorf("content review names unknown content identity %q", finding.ContentSHA256)
		}
		if _, duplicate := result[finding.ContentSHA256]; duplicate {
			return nil, fmt.Errorf("content review repeats content identity %q", finding.ContentSHA256)
		}
		if finding.Disposition != string(DispositionExclude) || finding.ReasonCode != ReasonNonBroadcastMaterial || strings.TrimSpace(finding.Detail) == "" || len(finding.Detail) > 2000 || len(finding.EvidenceRefs) < 2 || !slices.IsSorted(finding.EvidenceRefs) || len(slices.Compact(slices.Clone(finding.EvidenceRefs))) != len(finding.EvidenceRefs) {
			return nil, fmt.Errorf("content review finding %q has unsupported authority or evidence", finding.ContentSHA256)
		}
		evidenceByID := make(map[string]fillereval.Evidence, len(item.Evidence))
		for _, evidence := range item.Evidence {
			if _, duplicate := evidenceByID[evidence.ID]; duplicate {
				return nil, fmt.Errorf("content review case %q has ambiguous evidence id %q", finding.ContentSHA256, evidence.ID)
			}
			evidenceByID[evidence.ID] = evidence
		}
		for _, ref := range finding.EvidenceRefs {
			evidence, exists := evidenceByID[ref]
			if !exists || !contentEvidenceKind(evidence.Kind) {
				return nil, fmt.Errorf("content review finding %q cites missing or unsupported evidence %q", finding.ContentSHA256, ref)
			}
		}
		result[finding.ContentSHA256] = &AppliedContentFinding{
			ReviewerID: review.ReviewerID, ReviewedAt: review.ReviewedAt.UTC(), Disposition: finding.Disposition,
			ReasonCode: finding.ReasonCode, Detail: finding.Detail, EvidenceRefs: slices.Clone(finding.EvidenceRefs),
		}
	}
	return result, nil
}

func contentEvidenceKind(kind string) bool {
	switch kind {
	case string(filleradmission.KindFrame), string(filleradmission.KindTranscript), string(filleradmission.KindOCR), string(filleradmission.KindAudio), string(filleradmission.KindVideo):
		return true
	default:
		return false
	}
}

func validateDownloads(manifest fillereval.Manifest, ledger DownloadLedger) (map[string]DownloadCase, error) {
	if ledger.SchemaVersion != 1 || !validSHA256(ledger.InventorySHA256) || ledger.GeneratedAt.IsZero() || ledger.MaxRequests <= 0 || ledger.RequestsUsed < 0 || ledger.RequestsUsed > ledger.MaxRequests || ledger.MaxItems <= 0 || ledger.MaxBytes <= 0 || ledger.Bytes < 0 || ledger.Bytes > ledger.MaxBytes || len(ledger.Cases) != len(manifest.Cases) {
		return nil, fmt.Errorf("download ledger has invalid identity, ceilings, or case count")
	}
	manifestByID := make(map[string]fillereval.Case, len(manifest.Cases))
	for _, item := range manifest.Cases {
		manifestByID[item.ID] = item
	}
	byID := make(map[string]DownloadCase, len(ledger.Cases))
	localFiles := make(map[string]struct{}, len(ledger.Cases))
	contentHashes := make(map[string]struct{}, len(ledger.Cases))
	var bytes int64
	for _, item := range ledger.Cases {
		caseItem, exists := manifestByID[item.CaseID]
		if !exists {
			return nil, fmt.Errorf("download ledger contains extra case %q", item.CaseID)
		}
		if err := validateDownloadCase(caseItem, ledger.InventorySHA256, item); err != nil {
			return nil, err
		}
		if bytes > ledger.MaxBytes-item.Representation.Bytes {
			return nil, fmt.Errorf("download ledger bytes overflow")
		}
		bytes += item.Representation.Bytes
		if item.CaseID == "" || item.LocalFile == "" || !validSHA256(item.ContentSHA256) || item.Representation.Bytes <= 0 {
			return nil, fmt.Errorf("download ledger contains an incomplete case")
		}
		if _, duplicate := byID[item.CaseID]; duplicate {
			return nil, fmt.Errorf("download ledger repeats case %q", item.CaseID)
		}
		if _, duplicate := localFiles[item.LocalFile]; duplicate {
			return nil, fmt.Errorf("download ledger repeats local file %q", item.LocalFile)
		}
		if _, duplicate := contentHashes[item.ContentSHA256]; duplicate {
			return nil, fmt.Errorf("download ledger repeats content identity %q", item.ContentSHA256)
		}
		byID[item.CaseID] = item
		localFiles[item.LocalFile] = struct{}{}
		contentHashes[item.ContentSHA256] = struct{}{}
	}
	if bytes != ledger.Bytes {
		return nil, fmt.Errorf("download ledger byte total differs from its acquired source rows")
	}
	for _, item := range manifest.Cases {
		if _, ok := byID[item.ID]; !ok {
			return nil, fmt.Errorf("download ledger is missing case %q", item.ID)
		}
	}
	return byID, nil
}

func validateDownloadCase(item fillereval.Case, inventorySHA256 string, download DownloadCase) error {
	p := item.Provenance
	if download.ContentSHA256 != item.ContentSHA256 || download.Authority != p.Authority || download.ItemID != p.ItemID || download.LicenseURL != p.LicenseURL || download.ItemURL != p.ItemRef || download.MetadataRetrievedAt != p.MetadataRetrievedAt || download.MetadataSHA256 != p.MetadataSHA256 || download.Representation.Name != p.SourceFilename || download.Representation.URL != p.SourceRef || download.Representation.Bytes != p.SourceBytes {
		return fmt.Errorf("download case %q does not match manifest provenance or acquired source bytes", item.ID)
	}
	approval := download.Approval
	if approval.InventorySHA256 != inventorySHA256 || approval.CaseID != item.ID || approval.Authority != p.Authority || approval.ItemID != p.ItemID || approval.MetadataSHA256 != p.MetadataSHA256 || approval.Decision != "approved" || !approval.Redistributable || approval.ReviewerID != p.RightsReviewerID || approval.ReviewedAt != p.RightsReviewedAt || approval.RequiredCredit != p.RequiredCredit || !slices.Equal(approval.Restrictions, p.Restrictions) || strings.TrimSpace(approval.Basis) == "" || p.RightsDecision != "approved" || !p.Redistributable {
		return fmt.Errorf("download case %q has no matching approved rights decision", item.ID)
	}
	return nil
}

func validateMapping(manifest fillereval.Manifest, mapping MappingArtifact) (map[string]ProductMapping, error) {
	byLabel := make(map[string]ProductMapping, len(mapping.Mappings))
	for _, entry := range mapping.Mappings {
		if strings.TrimSpace(entry.ReviewerLabel) == "" || entry.Occurrences <= 0 || len(entry.ProductionCategories) == 0 || strings.TrimSpace(entry.Basis) == "" || !slices.IsSorted(entry.ProductionCategories) || len(slices.Compact(slices.Clone(entry.ProductionCategories))) != len(entry.ProductionCategories) {
			return nil, fmt.Errorf("invalid product mapping for %q", entry.ReviewerLabel)
		}
		if _, duplicate := byLabel[entry.ReviewerLabel]; duplicate {
			return nil, fmt.Errorf("duplicate product mapping for %q", entry.ReviewerLabel)
		}
		byLabel[entry.ReviewerLabel] = entry
	}
	counts := map[string]int{}
	assignments := 0
	for _, item := range manifest.Cases {
		for _, product := range item.Taxonomy["product"] {
			counts[product]++
			assignments++
		}
	}
	if assignments != mapping.SourceProductAssignments || assignments != mapping.MappedAssignments || len(counts) != mapping.UniqueReviewerLabels || len(counts) != len(byLabel) {
		return nil, fmt.Errorf("product mapping does not cover the manifest assignment set")
	}
	for label, count := range counts {
		entry, ok := byLabel[label]
		if !ok || entry.Occurrences != count {
			return nil, fmt.Errorf("product mapping occurrence mismatch for %q", label)
		}
	}
	return byLabel, nil
}

func screen(item fillereval.Case, packet fillerbakeoff.Packet, download DownloadCase, mappings map[string]ProductMapping, forest *taxonomy.Forest, contentFinding *AppliedContentFinding) (Case, error) {
	media, usability, license, err := mediaSummary(packet)
	if err != nil {
		return Case{}, err
	}
	row := Case{
		CaseID: item.ID, ContentSHA256: item.ContentSHA256, Source: item.Source,
		SourceItemID: item.Provenance.ItemID, SourceFilename: item.Provenance.SourceFilename,
		SourceLocalFile: download.LocalFile,
		SemanticTruth:   item.Truth, ContentRole: item.ContentRole, ReadinessGrade: "ungraded",
		NeedsFullVideoInspection: true, Media: media, ReviewerTaxonomy: cloneTaxonomy(item.Taxonomy),
		ProposedBrandValues: sortedUnique(item.Taxonomy["brand"]), ContentFinding: cloneContentFinding(contentFinding),
	}
	var exclude, hold []ReasonCode
	switch item.Truth {
	case fillereval.TruthInvalid:
		exclude = append(exclude, ReasonSemanticInvalid)
	case fillereval.TruthAmbiguous:
		hold = append(hold, ReasonSemanticAmbiguous)
	case fillereval.TruthEligible:
	default:
		hold = append(hold, ReasonSemanticAmbiguous)
	}
	switch item.ContentRole {
	case filleradmission.RoleProgrammeExcerpt, filleradmission.RoleCompilation:
		exclude = append(exclude, ReasonRoleNotFiller)
	case filleradmission.RoleCommercial, filleradmission.RolePromo, filleradmission.RoleBumper,
		filleradmission.RolePSA, filleradmission.RoleStationID, filleradmission.RoleTrailer,
		filleradmission.RoleInterstitial:
	case "":
		hold = append(hold, ReasonRoleMissing)
	default:
		return Case{}, fmt.Errorf("unknown content role %q", item.ContentRole)
	}
	switch usability {
	case filleradmission.UsabilityUnusable:
		exclude = append(exclude, ReasonMediaUnusable)
	case filleradmission.UsabilityUsable:
	default:
		hold = append(hold, ReasonMediaEvidenceMissing)
	}
	if media.NoVideo || media.NoAudio || !media.evidenceVideoValid {
		exclude = append(exclude, ReasonMediaUnusable)
	}
	if media.SourceDurationMS <= 0 || media.SegmentDurationMS <= 0 {
		exclude = append(exclude, ReasonMediaUnusable)
	}
	if media.SegmentDurationMS > 0 && media.SegmentDurationMS < 10_000 {
		exclude = append(exclude, ReasonDurationTooShort)
	}
	switch license {
	case filleradmission.EligibilityIneligible:
		exclude = append(exclude, ReasonSourceIneligible)
	case filleradmission.EligibilityEligible:
	default:
		hold = append(hold, ReasonSourceEvidenceMissing)
	}
	// The recovery preparer capped inspection at two minutes. A source extending materially
	// beyond the bound has not established that the observed prefix is one complete filler unit.
	if media.SourceDurationMS > media.SegmentStartMS+media.SegmentDurationMS+1000 {
		hold = append(hold, ReasonSourceCoverageIncomplete)
	}
	production, unresolved := productionTaxonomy(item, mappings, forest)
	row.ProposedProductionTags = production
	row.UnresolvedTaxonomyValues = unresolved
	if len(unresolved) > 0 {
		hold = append(hold, ReasonTaxonomyUnresolved)
	}
	if item.ContentRole == filleradmission.RoleCommercial && len(productTags(production)) == 0 {
		hold = append(hold, ReasonCommercialProductMissing)
	}
	if contentFinding != nil {
		exclude = append(exclude, contentFinding.ReasonCode)
	}
	exclude = sortedReasons(exclude)
	hold = sortedReasons(hold)
	switch {
	case len(exclude) > 0:
		row.Disposition = DispositionExclude
		row.ReasonCodes = append(exclude, hold...)
	case len(hold) > 0:
		row.Disposition = DispositionHold
		row.ReasonCodes = hold
	default:
		row.Disposition = DispositionCandidate
	}
	row.ReasonCodes = sortedReasons(row.ReasonCodes)
	return row, nil
}

func cloneContentFinding(finding *AppliedContentFinding) *AppliedContentFinding {
	if finding == nil {
		return nil
	}
	result := *finding
	result.EvidenceRefs = slices.Clone(finding.EvidenceRefs)
	return &result
}

func mediaSummary(packet fillerbakeoff.Packet) (MediaSummary, string, string, error) {
	var summary MediaSummary
	var usability, license string
	mediaFacts, rightsFacts := 0, 0
	for _, fact := range packet.Facts {
		switch fact.Claim {
		case filleradmission.ClaimMediaUsability:
			mediaFacts++
			usability = fact.Value
			if strings.TrimSpace(fact.Source) == "" || fact.Location == "" || (fact.Value != filleradmission.UsabilityUsable && fact.Value != filleradmission.UsabilityUnusable) {
				return MediaSummary{}, "", "", fmt.Errorf("incomplete media-usability fact")
			}
			count, err := fmt.Sscanf(fact.Location,
				"source_duration_ms=%d;segment_start_ms=%d;segment_duration_ms=%d;no_video=%t;no_audio=%t;black_percent=%d;silence_percent=%d",
				&summary.SourceDurationMS, &summary.SegmentStartMS, &summary.SegmentDurationMS,
				&summary.NoVideo, &summary.NoAudio, &summary.BlackPercent, &summary.SilencePercent)
			if err != nil || count != 7 || summary.SourceDurationMS < 0 || summary.SegmentStartMS < 0 || summary.SegmentDurationMS < 0 || summary.BlackPercent < 0 || summary.BlackPercent > 100 || summary.SilencePercent < 0 || summary.SilencePercent > 100 {
				return MediaSummary{}, "", "", fmt.Errorf("invalid media-usability measurement")
			}
			if summary.SegmentStartMS > summary.SourceDurationMS || summary.SegmentDurationMS > summary.SourceDurationMS-summary.SegmentStartMS {
				return MediaSummary{}, "", "", fmt.Errorf("invalid media-usability segment bounds")
			}
		case filleradmission.ClaimSourceLicense:
			rightsFacts++
			license = fact.Value
			if strings.TrimSpace(fact.Source) == "" || (fact.Value != filleradmission.EligibilityEligible && fact.Value != filleradmission.EligibilityIneligible) {
				return MediaSummary{}, "", "", fmt.Errorf("incomplete source-license fact")
			}
		}
	}
	if mediaFacts != 1 || rightsFacts != 1 {
		return MediaSummary{}, "", "", fmt.Errorf("packet requires exactly one media-usability fact and one source-license fact")
	}
	videoSignals := 0
	for _, signal := range packet.Signals {
		if signal.Kind != "video" {
			continue
		}
		videoSignals++
		summary.EvidenceVideoDurationMS = signal.DurationMS
		summary.Width = signal.Width
		summary.Height = signal.Height
		summary.EvidenceVideoPath = signal.Path
		summary.EvidenceVideoSHA256 = signal.SHA256
		summary.EvidenceVideoBytes = signal.Bytes
		summary.evidenceVideoValid = signal.Path != "" && validSHA256(signal.SHA256) && signal.Bytes > 0 && signal.DurationMS > 0 && signal.Width > 0 && signal.Height > 0 && slices.Equal(signal.ContentTypes, []string{"video/mp4"})
	}
	if videoSignals > 1 {
		return MediaSummary{}, "", "", fmt.Errorf("packet contains multiple evidence-video presentations")
	}
	return summary, usability, license, nil
}

func productionTaxonomy(item fillereval.Case, mappings map[string]ProductMapping, forest *taxonomy.Forest) ([]string, []string) {
	var proposed, unresolved []string
	for _, raw := range item.Taxonomy["product"] {
		entry, ok := mappings[raw]
		if !ok {
			unresolved = append(unresolved, raw)
			continue
		}
		for _, target := range entry.ProductionCategories {
			if slug, ok := forest.Resolve(target); ok {
				proposed = append(proposed, slug)
			} else {
				unresolved = append(unresolved, target)
			}
		}
	}
	format := map[string]string{
		filleradmission.RoleCommercial: "commercial", filleradmission.RolePromo: "promo",
		filleradmission.RoleBumper: "bumper", filleradmission.RolePSA: "psa",
		filleradmission.RoleStationID: "ident", filleradmission.RoleInterstitial: "interstitial",
	}[item.ContentRole]
	if format != "" {
		proposed = append(proposed, format)
	}
	if item.ContentRole == filleradmission.RoleTrailer {
		proposed = append(proposed, "movie_trailer")
	}
	return sortedUnique(proposed), sortedUnique(unresolved)
}

func productTags(values []string) []string {
	products := make(map[string]struct{})
	for _, taxon := range taxonomy.SeedForest() {
		if taxon.Axis == taxonomy.AxisProduct {
			products[taxon.Slug] = struct{}{}
		}
	}
	var result []string
	for _, value := range values {
		if _, ok := products[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func sortedReasons(values []ReasonCode) []ReasonCode {
	if len(values) == 0 {
		return nil
	}
	slices.Sort(values)
	return slices.Compact(values)
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := slices.Clone(values)
	slices.Sort(result)
	return slices.Compact(result)
}

func cloneTaxonomy(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string][]string, len(input))
	for axis, values := range input {
		output[axis] = sortedUnique(values)
	}
	return output
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
