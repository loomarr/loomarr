package fillerreference

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

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
		if err := validatePacketReferenceBinding(item, packet, PacketEvidenceVersion); err != nil {
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
