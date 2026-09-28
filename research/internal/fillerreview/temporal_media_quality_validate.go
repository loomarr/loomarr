package fillerreview

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type temporalMediaQualityLoaded struct {
	pack               TemporalHumanReviewPackage
	packageSHA         string
	mapSHA             string
	assessmentSHA      string
	attestationFileSHA string
}

func loadTemporalMediaQuality(config TemporalMediaQualityConfig) (temporalMediaQualityLoaded, []temporalMediaQualityInput, error) {
	if strings.TrimSpace(config.HumanPackagePath) == "" || strings.TrimSpace(config.HumanPrivateMapPath) == "" || strings.TrimSpace(config.HumanAssessmentPath) == "" || strings.TrimSpace(config.HumanAttestationPath) == "" || config.MeasuredAt.IsZero() || config.ExpectedCases <= 0 {
		return temporalMediaQualityLoaded{}, nil, fmt.Errorf("temporal media quality requires human package, map, lock, fixed measurement time, and expected cases")
	}
	pack, packageSHA, err := LoadTemporalHumanReviewPackage(config.HumanPackagePath)
	if err != nil {
		return temporalMediaQualityLoaded{}, nil, err
	}
	mapping, err := readStrictJSON[TemporalHumanReviewMap](config.HumanPrivateMapPath)
	if err != nil {
		return temporalMediaQualityLoaded{}, nil, fmt.Errorf("read temporal human review map: %w", err)
	}
	mapSHA, err := hashFile(config.HumanPrivateMapPath)
	if err != nil {
		return temporalMediaQualityLoaded{}, nil, err
	}
	aliasMap, err := validateTemporalHumanReviewMap(filepath.Dir(config.HumanPackagePath), pack, packageSHA, mapping)
	if err != nil {
		return temporalMediaQualityLoaded{}, nil, err
	}
	set, attestation, assessmentSHA, attestationFileSHA, err := loadTemporalHumanLockAuthority(config.HumanAssessmentPath, config.HumanAttestationPath)
	if err != nil {
		return temporalMediaQualityLoaded{}, nil, err
	}
	if len(pack.Cases) != config.ExpectedCases || len(set.Assessments) != config.ExpectedCases || set.PackageSHA256 != packageSHA || attestation.MapSHA256 != mapSHA || set.EvidenceManifestSHA256 != pack.EvidenceManifestSHA256 || set.SelectionSHA256 != pack.SelectionSHA256 || config.MeasuredAt.Before(attestation.LockedAt) {
		return temporalMediaQualityLoaded{}, nil, fmt.Errorf("temporal media quality inputs do not bind the same locked panel")
	}
	assessmentByAlias := make(map[string]TemporalHumanReviewAssessment, len(set.Assessments))
	for _, assessment := range set.Assessments {
		if _, duplicate := assessmentByAlias[assessment.EvidenceAlias]; duplicate {
			return temporalMediaQualityLoaded{}, nil, fmt.Errorf("locked assessment repeats evidence alias %q", assessment.EvidenceAlias)
		}
		assessmentByAlias[assessment.EvidenceAlias] = assessment
	}
	root := filepath.Dir(config.HumanPackagePath)
	inputs := make([]temporalMediaQualityInput, 0, len(pack.Cases))
	for _, item := range pack.Cases {
		evidenceAlias := aliasMap[item.Alias]
		assessment, exists := assessmentByAlias[evidenceAlias]
		if !exists {
			return temporalMediaQualityLoaded{}, nil, fmt.Errorf("locked assessment is missing evidence alias %q", evidenceAlias)
		}
		if assessment.DecisiveAtMS < 0 || assessment.DecisiveAtMS >= item.DurationMS || !validHumanUnit(assessment.Unit) {
			return temporalMediaQualityLoaded{}, nil, fmt.Errorf("locked assessment %q has an invalid unit or decisive time", evidenceAlias)
		}
		if assessment.Unit == fillereval.UnitStandalone {
			if assessment.Role == nil || !validHumanRole(*assessment.Role) {
				return temporalMediaQualityLoaded{}, nil, fmt.Errorf("locked standalone assessment %q lacks a closed role", evidenceAlias)
			}
		} else if assessment.Role != nil {
			return temporalMediaQualityLoaded{}, nil, fmt.Errorf("locked non-standalone assessment %q carries a role", evidenceAlias)
		}
		inputs = append(inputs, temporalMediaQualityInput{
			EvidenceAlias: evidenceAlias, SourceMediaSHA256: item.Video.SHA256, HumanUnit: assessment.Unit, DurationMS: item.Video.DurationMS,
			Path: filepath.Join(root, filepath.FromSlash(item.Video.Path)),
		})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].EvidenceAlias < inputs[j].EvidenceAlias })
	return temporalMediaQualityLoaded{pack: pack, packageSHA: packageSHA, mapSHA: mapSHA, assessmentSHA: assessmentSHA, attestationFileSHA: attestationFileSHA}, inputs, nil
}
