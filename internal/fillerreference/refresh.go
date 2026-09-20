package fillerreference

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	legacyReferenceContractVersion = "filler-reference-cohort-2026-08-31-v3"
	legacyPacketEvidenceVersion    = "filler-evidence-v1"
	legacyPacketSchemaVersion      = 1
	refreshReportKind              = "filler_reference_contract_refresh"
	refreshReportSchemaVersion     = 1
	refreshedCorpusSuffix          = "-license-free-v2"
	legacyLicenseFactID            = "source-license"
	legacyLicenseEligible          = "eligible"
)

type RawRefreshInputs struct {
	Manifest      []byte
	Packets       []byte
	Mapping       []byte
	ContentReview []byte
}

type RefreshedArtifacts struct {
	Manifest      []byte
	Packets       []byte
	Mapping       []byte
	ContentReview []byte
	Report        RefreshReport
}

type RefreshArtifactIdentities struct {
	ManifestSHA256      string `json:"manifestSha256"`
	PacketsSHA256       string `json:"packetsSha256"`
	MappingSHA256       string `json:"mappingSha256"`
	ContentReviewSHA256 string `json:"contentReviewSha256"`
}

type RefreshContractIdentity struct {
	ReferenceContract string `json:"referenceContract"`
	PacketSchema      int    `json:"packetSchema"`
	EvidenceVersion   string `json:"evidenceVersion"`
	CorpusVersion     string `json:"corpusVersion"`
}

type RefreshDenominators struct {
	Cases                     int `json:"cases"`
	Packets                   int `json:"packets"`
	RemovedSourceLicenseFacts int `json:"removedSourceLicenseFacts"`
	LabelReviews              int `json:"labelReviews"`
	Adjudications             int `json:"adjudications"`
	ContentFindings           int `json:"contentFindings"`
}

type RefreshReport struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Kind          string                    `json:"kind"`
	RefreshedAt   time.Time                 `json:"refreshedAt"`
	From          RefreshContractIdentity   `json:"from"`
	To            RefreshContractIdentity   `json:"to"`
	Inputs        RefreshArtifactIdentities `json:"inputs"`
	Outputs       RefreshArtifactIdentities `json:"outputs"`
	Denominators  RefreshDenominators       `json:"denominators"`
}

// Refresh removes the retired licensing decision fact from the one locked
// development reference corpus. It emits only current artifacts; neither the
// audit nor the application learns how to accept the retired contract.
func Refresh(raw RawRefreshInputs, refreshedAt time.Time) (RefreshedArtifacts, error) {
	if refreshedAt.IsZero() {
		return RefreshedArtifacts{}, fmt.Errorf("refresh time is required")
	}
	raw = RawRefreshInputs{
		Manifest: bytes.Clone(raw.Manifest), Packets: bytes.Clone(raw.Packets),
		Mapping: bytes.Clone(raw.Mapping), ContentReview: bytes.Clone(raw.ContentReview),
	}
	manifest, err := decodeStrictJSON[fillereval.Manifest](raw.Manifest)
	if err != nil {
		return RefreshedArtifacts{}, fmt.Errorf("manifest: %w", err)
	}
	packets, err := decodePackets(raw.Packets)
	if err != nil {
		return RefreshedArtifacts{}, fmt.Errorf("packets: %w", err)
	}
	mapping, err := decodeStrictJSON[MappingArtifact](raw.Mapping)
	if err != nil {
		return RefreshedArtifacts{}, fmt.Errorf("mapping: %w", err)
	}
	review, err := decodeStrictJSON[ContentReviewArtifact](raw.ContentReview)
	if err != nil {
		return RefreshedArtifacts{}, fmt.Errorf("content review: %w", err)
	}
	inputManifestSHA := SHA256(raw.Manifest)
	if err := validateRefreshInputs(manifest, packets, mapping, review, inputManifestSHA, refreshedAt); err != nil {
		return RefreshedArtifacts{}, err
	}

	labelReviews, adjudications := 0, 0
	for index := range manifest.Cases {
		item := &manifest.Cases[index]
		beforeLabel := fillereval.LabelSHA256(*item)
		packet := packets[item.ID]
		packet.SchemaVersion = fillerbakeoff.PacketSchemaVersion
		packet.EvidenceVersion = PacketEvidenceVersion
		packet.Facts = slices.DeleteFunc(packet.Facts, isLegacyLicenseFact)
		packets[item.ID] = packet
		item.EvidenceSHA256 = fillerbakeoff.PacketSHA256(packet)
		if fillereval.LabelSHA256(*item) != beforeLabel {
			return RefreshedArtifacts{}, fmt.Errorf("case %q semantic label changed during refresh", item.ID)
		}
		labelReviews += len(item.LabelReviews)
		if item.Adjudication != nil {
			adjudications++
		}
	}
	fromCorpus := manifest.CorpusVersion
	manifest.CorpusVersion += refreshedCorpusSuffix
	manifest.LockedAt = refreshedAt.UTC()
	if failures := fillereval.ValidateDevelopmentRun(manifest); len(failures) != 0 {
		return RefreshedArtifacts{}, fmt.Errorf("refreshed manifest is invalid: %s", strings.Join(failures, "; "))
	}
	manifestRaw, err := marshalIndented(manifest)
	if err != nil {
		return RefreshedArtifacts{}, err
	}
	packetsRaw, err := marshalPackets(packets)
	if err != nil {
		return RefreshedArtifacts{}, err
	}
	outputManifestSHA := SHA256(manifestRaw)
	mapping.SourceManifestSHA256 = outputManifestSHA
	mappingRaw, err := marshalIndented(mapping)
	if err != nil {
		return RefreshedArtifacts{}, err
	}
	review.ContractVersion = ContractVersion
	review.SourceManifestSHA256 = outputManifestSHA
	reviewRaw, err := marshalIndented(review)
	if err != nil {
		return RefreshedArtifacts{}, err
	}
	if _, err := validateContentReview(manifest, review, outputManifestSHA, refreshedAt); err != nil {
		return RefreshedArtifacts{}, fmt.Errorf("refreshed content review: %w", err)
	}
	for _, item := range manifest.Cases {
		if err := validatePacketReferenceBinding(item, packets[item.ID], PacketEvidenceVersion); err != nil {
			return RefreshedArtifacts{}, fmt.Errorf("refreshed packet: %w", err)
		}
	}
	result := RefreshedArtifacts{
		Manifest: manifestRaw, Packets: packetsRaw, Mapping: mappingRaw, ContentReview: reviewRaw,
		Report: RefreshReport{
			SchemaVersion: refreshReportSchemaVersion, Kind: refreshReportKind, RefreshedAt: refreshedAt.UTC(),
			From:         RefreshContractIdentity{ReferenceContract: legacyReferenceContractVersion, PacketSchema: legacyPacketSchemaVersion, EvidenceVersion: legacyPacketEvidenceVersion, CorpusVersion: fromCorpus},
			To:           RefreshContractIdentity{ReferenceContract: ContractVersion, PacketSchema: fillerbakeoff.PacketSchemaVersion, EvidenceVersion: PacketEvidenceVersion, CorpusVersion: manifest.CorpusVersion},
			Inputs:       RefreshArtifactIdentities{ManifestSHA256: inputManifestSHA, PacketsSHA256: SHA256(raw.Packets), MappingSHA256: SHA256(raw.Mapping), ContentReviewSHA256: SHA256(raw.ContentReview)},
			Outputs:      RefreshArtifactIdentities{ManifestSHA256: outputManifestSHA, PacketsSHA256: SHA256(packetsRaw), MappingSHA256: SHA256(mappingRaw), ContentReviewSHA256: SHA256(reviewRaw)},
			Denominators: RefreshDenominators{Cases: len(manifest.Cases), Packets: len(packets), RemovedSourceLicenseFacts: len(packets), LabelReviews: labelReviews, Adjudications: adjudications, ContentFindings: len(review.Findings)},
		},
	}
	return result, nil
}

func validateRefreshInputs(manifest fillereval.Manifest, packets map[string]fillerbakeoff.Packet, mapping MappingArtifact, review ContentReviewArtifact, manifestSHA string, refreshedAt time.Time) error {
	if len(manifest.Cases) != 300 || strings.HasSuffix(manifest.CorpusVersion, refreshedCorpusSuffix) {
		return fmt.Errorf("refresh requires one unrefreshed locked 300-case development manifest")
	}
	if failures := fillereval.ValidateDevelopmentRun(manifest); len(failures) != 0 {
		return fmt.Errorf("legacy manifest is invalid: %s", strings.Join(failures, "; "))
	}
	if refreshedAt.Before(manifest.LockedAt) {
		return fmt.Errorf("refresh time predates the manifest lock")
	}
	if mapping.SchemaVersion != 1 || strings.TrimSpace(mapping.MappingVersion) == "" || mapping.SourceManifestSHA256 != manifestSHA || mapping.UnmappedAssignments != 0 || mapping.SourceProductAssignments != mapping.MappedAssignments {
		return fmt.Errorf("legacy mapping identity or coverage is invalid")
	}
	if review.SchemaVersion != 1 || review.Kind != ContentReviewKind || review.ContractVersion != legacyReferenceContractVersion || strings.TrimSpace(review.ReviewerID) == "" || review.ReviewedAt.IsZero() || review.ReviewedAt.After(refreshedAt) || review.SourceManifestSHA256 != manifestSHA || len(review.Findings) == 0 {
		return fmt.Errorf("legacy content review identity, authority, or time is invalid")
	}
	if len(packets) != len(manifest.Cases) {
		return fmt.Errorf("legacy packet set covers %d/%d cases", len(packets), len(manifest.Cases))
	}
	seen := make(map[string]struct{}, len(manifest.Cases))
	for _, item := range manifest.Cases {
		packet, ok := packets[item.ID]
		if !ok {
			return fmt.Errorf("case %q has no legacy packet", item.ID)
		}
		seen[item.ID] = struct{}{}
		if packet.SchemaVersion != legacyPacketSchemaVersion || packet.CaseID != item.ID || packet.EvidenceVersion != legacyPacketEvidenceVersion || packet.ContentSHA256 != item.ContentSHA256 || item.EvidenceSHA256 != fillerbakeoff.PacketSHA256(packet) {
			return fmt.Errorf("case %q legacy packet identity is invalid", item.ID)
		}
		mediaFacts, licenseFacts := 0, 0
		for _, fact := range packet.Facts {
			switch {
			case fact.Claim == filleradmission.ClaimMediaUsability && fact.Kind == filleradmission.KindDecoder && strings.TrimSpace(fact.ID) != "" && strings.TrimSpace(fact.Source) != "" && fact.EvaluationID == "":
				mediaFacts++
			case isLegacyLicenseFact(fact):
				licenseFacts++
			default:
				return fmt.Errorf("case %q contains an undeclared legacy fact", item.ID)
			}
		}
		if len(packet.Facts) != 2 || mediaFacts != 1 || licenseFacts != 1 {
			return fmt.Errorf("case %q requires exactly one decoder and one retired licensing fact", item.ID)
		}
	}
	for id := range packets {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("legacy packet %q is outside the manifest", id)
		}
	}
	return nil
}

func isLegacyLicenseFact(fact filleradmission.Evidence) bool {
	return fact.ID == legacyLicenseFactID && strings.TrimSpace(string(fact.Claim)) != "" && fact.Claim != filleradmission.ClaimMediaUsability && fact.Value == legacyLicenseEligible && fact.Kind == filleradmission.KindSourcePolicy && strings.TrimSpace(fact.Source) != "" && fact.Derivative == "" && fact.Location == "" && fact.AtMS == 0 && fact.EvaluationID == ""
}

func marshalIndented(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func marshalPackets(packets map[string]fillerbakeoff.Packet) ([]byte, error) {
	ids := make([]string, 0, len(packets))
	for id := range packets {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, id := range ids {
		if err := encoder.Encode(packets[id]); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}
