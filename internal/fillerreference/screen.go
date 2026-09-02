package fillerreference

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

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
