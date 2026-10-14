package fillerreview

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalTruthHistoryLabels        = "labels"
	TemporalTruthHistoryAdjudications = "adjudications"
)

type TemporalTruthHistoryConfig struct {
	DraftPath string
	Artifacts []TemporalTruthHistoryArtifact
}

type TemporalTruthHistoryArtifact struct {
	Name           string
	PackagePath    string
	AliasMapPath   string
	SubmissionPath string
	Kind           string
}

// LoadTemporalTruthCandidateHistory strictly joins the three immutable legacy
// artifacts to the exact 300-case draft. Recovered answers are normalized only
// for sampling and are never returned as final truth.
func LoadTemporalTruthCandidateHistory(config TemporalTruthHistoryConfig) ([]fillereval.TemporalTruthInputDigest, []fillereval.TemporalTruthCandidate, error) {
	if strings.TrimSpace(config.DraftPath) == "" || len(config.Artifacts) != 3 {
		return nil, nil, fmt.Errorf("temporal truth history requires one draft and exactly three artifacts")
	}
	draft, err := readStrictJSON[fillereval.Manifest](config.DraftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read temporal truth draft: %w", err)
	}
	if failures := fillereval.ValidateReviewDraft(draft); len(failures) > 0 || len(draft.Cases) != fillereval.CertificationMinDevelopment {
		return nil, nil, fmt.Errorf("temporal truth draft must be the exact unlabeled 300-case development seed: %s", strings.Join(failures, "; "))
	}
	draftDigest := fillereval.ManifestSHA256(draft)
	inputs := []fillereval.TemporalTruthInputDigest{{Name: "draft", SHA256: mustHistoryFileSHA256(config.DraftPath)}}
	if inputs[0].SHA256 == "" {
		return nil, nil, fmt.Errorf("hash temporal truth draft")
	}
	candidates := make(map[string]*fillereval.TemporalTruthCandidate, len(draft.Cases))
	for _, item := range draft.Cases {
		if _, duplicate := candidates[item.ID]; duplicate {
			return nil, nil, fmt.Errorf("temporal truth draft repeats case %q", item.ID)
		}
		candidates[item.ID] = &fillereval.TemporalTruthCandidate{
			CaseID: item.ID, ContentSHA256: item.ContentSHA256, SourceLane: item.Source,
			DurationMS: item.Provenance.SegmentDurationMS,
		}
	}

	artifactNames := map[string]struct{}{}
	labelArtifacts, adjudicationArtifacts := 0, 0
	for _, artifact := range config.Artifacts {
		name := strings.TrimSpace(artifact.Name)
		if name == "" || strings.TrimSpace(artifact.PackagePath) == "" || strings.TrimSpace(artifact.AliasMapPath) == "" || strings.TrimSpace(artifact.SubmissionPath) == "" {
			return nil, nil, fmt.Errorf("temporal truth history artifacts require name, package, map, and submission")
		}
		if _, duplicate := artifactNames[name]; duplicate {
			return nil, nil, fmt.Errorf("temporal truth history repeats artifact %q", name)
		}
		artifactNames[name] = struct{}{}
		switch artifact.Kind {
		case TemporalTruthHistoryLabels:
			labelArtifacts++
		case TemporalTruthHistoryAdjudications:
			adjudicationArtifacts++
		default:
			return nil, nil, fmt.Errorf("temporal truth history artifact %q has invalid kind", name)
		}

		pack, mapping, err := loadTemporalTruthHistoryPackage(artifact, draft, draftDigest)
		if err != nil {
			return nil, nil, fmt.Errorf("temporal truth history %q: %w", name, err)
		}
		inputs = append(inputs,
			fillereval.TemporalTruthInputDigest{Name: name + ".package", SHA256: mustHistoryFileSHA256(artifact.PackagePath)},
			fillereval.TemporalTruthInputDigest{Name: name + ".map", SHA256: mustHistoryFileSHA256(artifact.AliasMapPath)},
			fillereval.TemporalTruthInputDigest{Name: name + ".submissions", SHA256: mustHistoryFileSHA256(artifact.SubmissionPath)},
		)
		var normalized map[string]fillereval.TemporalTruthCandidateAssessment
		if artifact.Kind == TemporalTruthHistoryLabels {
			normalized, err = loadTemporalTruthLabels(name, artifact.SubmissionPath, pack.BatchID, mapping, candidates)
		} else {
			normalized, err = loadTemporalTruthAdjudications(name, artifact.SubmissionPath, candidates)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("temporal truth history %q: %w", name, err)
		}
		for caseID, assessment := range normalized {
			candidates[caseID].Assessments = append(candidates[caseID].Assessments, assessment)
		}
	}
	if labelArtifacts != 2 || adjudicationArtifacts != 1 {
		return nil, nil, fmt.Errorf("temporal truth history requires exactly two label artifacts and one adjudication artifact")
	}

	result := make([]fillereval.TemporalTruthCandidate, 0, len(draft.Cases))
	for _, item := range draft.Cases {
		candidate := *candidates[item.ID]
		if len(candidate.Assessments) != 3 {
			return nil, nil, fmt.Errorf("temporal truth case %q has %d/3 recovered assessments", item.ID, len(candidate.Assessments))
		}
		sort.Slice(candidate.Assessments, func(i, j int) bool { return candidate.Assessments[i].Assessor < candidate.Assessments[j].Assessor })
		result = append(result, candidate)
	}
	for _, input := range inputs {
		if input.SHA256 == "" {
			return nil, nil, fmt.Errorf("hash temporal truth input %q", input.Name)
		}
	}
	return inputs, result, nil
}

func loadTemporalTruthHistoryPackage(artifact TemporalTruthHistoryArtifact, draft fillereval.Manifest, draftDigest string) (Package, fillereval.BlindReviewMap, error) {
	pack, err := readStrictJSON[Package](artifact.PackagePath)
	if err != nil {
		return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("read package: %w", err)
	}
	if _, err := validateReviewPackageStructure(pack, len(draft.Cases)); err != nil {
		return Package{}, fillereval.BlindReviewMap{}, err
	}
	mapping, err := readStrictJSON[fillereval.BlindReviewMap](artifact.AliasMapPath)
	if err != nil {
		return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("read private alias map: %w", err)
	}
	if pack.DraftSHA256 != draftDigest || mapping.SchemaVersion != fillereval.BlindReviewSchemaVersion || mapping.BatchID != pack.BatchID || mapping.DraftSHA256 != draftDigest || len(mapping.Entries) != len(draft.Cases) {
		return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("package and private map do not bind the exact draft and batch")
	}
	draftByID := make(map[string]fillereval.Case, len(draft.Cases))
	for _, item := range draft.Cases {
		draftByID[item.ID] = item
	}
	caseByAlias := make(map[string]string, len(mapping.Entries))
	seenCases := make(map[string]struct{}, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		if strings.TrimSpace(entry.Alias) == "" || strings.TrimSpace(entry.CaseID) == "" {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("private alias map contains an empty entry")
		}
		if _, duplicate := caseByAlias[entry.Alias]; duplicate {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("private alias map repeats alias %q", entry.Alias)
		}
		if _, duplicate := seenCases[entry.CaseID]; duplicate {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("private alias map repeats case %q", entry.CaseID)
		}
		if _, exists := draftByID[entry.CaseID]; !exists {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("private alias map names unknown case %q", entry.CaseID)
		}
		caseByAlias[entry.Alias] = entry.CaseID
		seenCases[entry.CaseID] = struct{}{}
	}
	seenAliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		caseID, exists := caseByAlias[item.Alias]
		if !exists {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("package alias %q is absent from private map", item.Alias)
		}
		if _, duplicate := seenAliases[item.Alias]; duplicate {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("package repeats alias %q", item.Alias)
		}
		seenAliases[item.Alias] = struct{}{}
		source := draftByID[caseID]
		if item.ContentSHA256 != source.ContentSHA256 || item.EvidenceSHA256 != source.EvidenceSHA256 || item.SegmentStartMS != source.Provenance.SegmentStartMS || item.SegmentDurationMS != source.Provenance.SegmentDurationMS {
			return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("package alias %q does not content-bind mapped case %q", item.Alias, caseID)
		}
	}
	if len(seenAliases) != len(caseByAlias) {
		return Package{}, fillereval.BlindReviewMap{}, fmt.Errorf("package covers %d/%d private aliases", len(seenAliases), len(caseByAlias))
	}
	return pack, mapping, nil
}

func loadTemporalTruthLabels(name, path, batchID string, mapping fillereval.BlindReviewMap, candidates map[string]*fillereval.TemporalTruthCandidate) (map[string]fillereval.TemporalTruthCandidateAssessment, error) {
	rows, err := readStrictHistoryJSONL[fillereval.LabelSubmission](path)
	if err != nil {
		return nil, err
	}
	aliasToCase := make(map[string]string, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		aliasToCase[entry.Alias] = entry.CaseID
	}
	result := make(map[string]fillereval.TemporalTruthCandidateAssessment, len(rows))
	var reviewerID string
	for index, row := range rows {
		caseID, exists := aliasToCase[row.Alias]
		if !exists || row.BatchID != batchID || strings.TrimSpace(row.ReviewerID) == "" || row.ReviewedAt.IsZero() {
			return nil, fmt.Errorf("label row %d does not bind the package alias, batch, reviewer, and time", index+1)
		}
		if reviewerID == "" {
			reviewerID = row.ReviewerID
		} else if row.ReviewerID != reviewerID {
			return nil, fmt.Errorf("label artifact contains more than one reviewer")
		}
		if _, duplicate := result[caseID]; duplicate {
			return nil, fmt.Errorf("label artifact repeats mapped case %q", caseID)
		}
		if _, exists := candidates[caseID]; !exists {
			return nil, fmt.Errorf("label artifact names unknown case %q", caseID)
		}
		assessment, err := normalizeTemporalTruthLabels(name, row.Labels)
		if err != nil {
			return nil, fmt.Errorf("label row %d: %w", index+1, err)
		}
		result[caseID] = assessment
	}
	if len(result) != len(candidates) {
		return nil, fmt.Errorf("label artifact covers %d/%d cases", len(result), len(candidates))
	}
	return result, nil
}

func loadTemporalTruthAdjudications(name, path string, candidates map[string]*fillereval.TemporalTruthCandidate) (map[string]fillereval.TemporalTruthCandidateAssessment, error) {
	rows, err := readStrictHistoryJSONL[fillereval.AdjudicationSubmission](path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]fillereval.TemporalTruthCandidateAssessment, len(rows))
	for index, row := range rows {
		if strings.TrimSpace(row.CaseID) == "" || strings.TrimSpace(row.AdjudicatorID) == "" || row.AdjudicatedAt.IsZero() || strings.TrimSpace(row.Reason) == "" {
			return nil, fmt.Errorf("adjudication row %d lacks case, adjudicator, time, or reason", index+1)
		}
		if _, exists := candidates[row.CaseID]; !exists {
			return nil, fmt.Errorf("adjudication artifact names unknown case %q", row.CaseID)
		}
		if _, duplicate := result[row.CaseID]; duplicate {
			return nil, fmt.Errorf("adjudication artifact repeats case %q", row.CaseID)
		}
		assessment, err := normalizeTemporalTruthLabels(name, row.Labels)
		if err != nil {
			return nil, fmt.Errorf("adjudication row %d: %w", index+1, err)
		}
		result[row.CaseID] = assessment
	}
	if len(result) != len(candidates) {
		return nil, fmt.Errorf("adjudication artifact covers %d/%d cases", len(result), len(candidates))
	}
	return result, nil
}

func normalizeTemporalTruthLabels(assessor string, labels fillereval.Labels) (fillereval.TemporalTruthCandidateAssessment, error) {
	if failures := fillereval.ValidateLabels(labels); len(failures) > 0 {
		return fillereval.TemporalTruthCandidateAssessment{}, fmt.Errorf("invalid recovered labels: %s", strings.Join(failures, "; "))
	}
	result := fillereval.TemporalTruthCandidateAssessment{Assessor: assessor}
	switch {
	case labels.Truth == fillereval.TruthInvalid && labels.RejectClass == fillereval.RejectDeterministic:
		result.Unit = fillereval.UnitUnusable
	case labels.Truth == fillereval.TruthAmbiguous:
		result.Unit = fillereval.UnitUnclear
	case labels.ContentRole == "programme_excerpt":
		result.Unit = fillereval.UnitProgrammeExcerpt
	case labels.ContentRole == "compilation":
		result.Unit = fillereval.UnitCompilation
	case labels.Truth == fillereval.TruthEligible:
		result.Unit = fillereval.UnitStandalone
		result.Role = temporalTruthRole(labels.ContentRole)
	default:
		// The old semantic-invalid schema asserted that a filler-looking role was
		// invalid without expressing why. It cannot safely become standalone.
		result.Unit = fillereval.UnitUnclear
	}
	if result.Unit == fillereval.UnitStandalone && result.Role == "" {
		return fillereval.TemporalTruthCandidateAssessment{}, fmt.Errorf("eligible recovered labels have no factored standalone role")
	}
	return result, nil
}

func temporalTruthRole(role string) fillereval.TemporalRole {
	switch role {
	case "commercial":
		return fillereval.TemporalRoleCommercial
	case "promo":
		return fillereval.TemporalRolePromo
	case "bumper":
		return fillereval.TemporalRoleBumper
	case "psa":
		return fillereval.TemporalRolePSA
	case "station_id":
		return fillereval.TemporalRoleStationID
	case "trailer":
		return fillereval.TemporalRoleTrailer
	case "interstitial":
		return fillereval.TemporalRoleInterstitial
	default:
		return ""
	}
}

func readStrictHistoryJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var result []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLine)
	for line := 1; scanner.Scan(); line++ {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var value T
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("line %d: trailing JSON value", line)
			}
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		result = append(result, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func mustHistoryFileSHA256(path string) string {
	digest, err := hashFile(path)
	if err != nil {
		return ""
	}
	return digest
}
