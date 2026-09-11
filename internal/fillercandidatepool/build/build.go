// Package build composes the independent corpus, quarantine, review, and
// prior-exposure authorities into one replacement candidate pool.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercandidatepool"
	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerquarantine"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

const maximumAuthorityBytes = int64(16 << 20)

type Config struct {
	InventoryPath                string
	RightsDecisionsPath          string
	RightsProfile                string
	MaterializationLedgerPath    string
	QuarantineDownloadLedgerPath string
	QuarantineInspectionPath     string
	SourceRoot                   string
	Review                       fillerreview.ReplacementCandidateReviewConfig
	PriorAdjudicationPaths       []string
	GeneratedAt                  time.Time
	OutputPath                   string
}

type Result struct {
	Candidates int
	Eligible   int
	Held       int
	SHA256     string
}

// Publish validates every authority before exclusively creating the output.
func Publish(config Config) (Result, error) {
	if strings.TrimSpace(config.OutputPath) == "" {
		return Result{}, errors.New("replacement candidate pool output path is required")
	}
	pool, err := Build(config)
	if err != nil {
		return Result{}, err
	}
	raw, err := json.MarshalIndent(pool, "", "  ")
	if err != nil {
		return Result{}, err
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(config.OutputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Result{}, fmt.Errorf("publish replacement candidate pool: %w", err)
	}
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(config.OutputPath)
		return Result{}, fmt.Errorf("publish replacement candidate pool: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(config.OutputPath)
		return Result{}, fmt.Errorf("publish replacement candidate pool: %w", closeErr)
	}
	result := Result{Candidates: len(pool.Candidates), SHA256: fillercandidatepool.Digest(raw)}
	for _, candidate := range pool.Candidates {
		if candidate.Disposition == fillercandidatepool.DispositionEligible {
			result.Eligible++
		} else {
			result.Held++
		}
	}
	return result, nil
}

// Build performs only local reads and deterministic joins. It does not invoke
// media tools, adapters, providers, downloads, training, or product services.
func Build(config Config) (fillercandidatepool.Pool, error) {
	if err := validateConfig(config); err != nil {
		return fillercandidatepool.Pool{}, err
	}
	inventoryRaw, err := readBounded(config.InventoryPath)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	inventory, err := fillercorpus.DecodeInventoryBytes(inventoryRaw)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	inventorySHA := fillercorpus.InventorySHA256(inventoryRaw)
	if config.GeneratedAt.Before(inventory.SnapshotAt) {
		return fillercandidatepool.Pool{}, errors.New("candidate pool predates its inventory")
	}
	decisionsRaw, err := readBounded(config.RightsDecisionsPath)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	decisions, err := fillercorpus.DecodeRightsDecisions(decisionsRaw)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	inventoryByCase := make(map[string]fillercorpus.InventoryCase, len(inventory.Cases))
	for _, item := range inventory.Cases {
		inventoryByCase[item.CaseID] = item
	}
	decisionByCase := make(map[string]fillercorpus.RightsDecision, len(decisions))
	for _, decision := range decisions {
		item, exists := inventoryByCase[decision.CaseID]
		if !exists {
			return fillercandidatepool.Pool{}, fmt.Errorf("rights decision contains unknown case %q", decision.CaseID)
		}
		if err := fillercorpus.ValidateRightsDecision(item, inventorySHA, decision, config.RightsProfile, config.GeneratedAt); err != nil {
			return fillercandidatepool.Pool{}, err
		}
		decisionByCase[decision.CaseID] = decision
	}

	inputs := []fillercandidatepool.Input{
		{Name: "inventory", SHA256: inventorySHA},
		{Name: "rights_decisions", SHA256: fillercandidatepool.Digest(decisionsRaw)},
	}
	materializedByCase := map[string]fillercorpus.MaterializedCase{}
	if config.MaterializationLedgerPath != "" {
		raw, err := readBounded(config.MaterializationLedgerPath)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		ledger, err := fillercorpus.DecodeMaterializationLedgerBytes(raw)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		if ledger.Profile != config.RightsProfile {
			return fillercandidatepool.Pool{}, errors.New("materialization ledger profile does not match rights decisions")
		}
		if err := fillercorpus.ValidateMaterializationLedger(ledger, inventory, inventorySHA); err != nil {
			return fillercandidatepool.Pool{}, err
		}
		if ledger.GeneratedAt.After(config.GeneratedAt) {
			return fillercandidatepool.Pool{}, errors.New("materialization ledger postdates pool generation")
		}
		for _, item := range ledger.Cases {
			decision, exists := decisionByCase[item.CaseID]
			if !exists || !reflect.DeepEqual(decision, item.Approval) {
				return fillercandidatepool.Pool{}, fmt.Errorf("materialized case %q does not bind the exact rights lock", item.CaseID)
			}
			materializedByCase[item.CaseID] = item
		}
		inputs = append(inputs, fillercandidatepool.Input{Name: "materialization_ledger", SHA256: fillercandidatepool.Digest(raw)})
	}

	hasRemote := false
	for _, item := range inventory.Cases {
		hasRemote = hasRemote || item.Representation.Transport != fillercorpus.TransportLocal
	}
	var inspectionRaw []byte
	if hasRemote {
		if config.MaterializationLedgerPath == "" || config.QuarantineDownloadLedgerPath == "" || config.QuarantineInspectionPath == "" {
			return fillercandidatepool.Pool{}, errors.New("non-local inventory requires exact materialization, quarantine download, and inspection authorities")
		}
		quarantineLedgerRaw, err := readBounded(config.QuarantineDownloadLedgerPath)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		quarantineLedger, err := fillercorpus.DecodeDownloadLedgerBytes(quarantineLedgerRaw)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		if err := fillercorpus.ValidateQuarantineDownloadLedger(inventory, inventorySHA, quarantineLedger); err != nil {
			return fillercandidatepool.Pool{}, err
		}
		inspectionRaw, err = readBounded(config.QuarantineInspectionPath)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		inputs = append(inputs,
			fillercandidatepool.Input{Name: "quarantine_download_ledger", SHA256: fillercandidatepool.Digest(quarantineLedgerRaw)},
			fillercandidatepool.Input{Name: "quarantine_inspection", SHA256: fillercandidatepool.Digest(inspectionRaw)},
		)
	} else if config.QuarantineDownloadLedgerPath != "" || config.QuarantineInspectionPath != "" {
		return fillercandidatepool.Pool{}, errors.New("local-only inventory cannot consume quarantine download or inspection authorities")
	}
	quarantine, err := fillerquarantine.OpenRightsEligibility(inventoryRaw, inspectionRaw)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	if binding := quarantine.InspectionBinding(); binding != nil && binding.DownloadLedgerSHA256 != inputSHA(inputs, "quarantine_download_ledger") {
		return fillercandidatepool.Pool{}, errors.New("quarantine inspection does not bind the exact download ledger")
	}

	config.Review.SourceRoot = config.SourceRoot
	config.Review.OpenedAt = config.GeneratedAt
	review, err := fillerreview.OpenReplacementCandidateReviewAuthority(config.Review)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	for _, input := range review.Inputs {
		inputs = append(inputs, fillercandidatepool.Input{Name: "review:" + input.Name, SHA256: input.SHA256})
	}
	reviewByCase := make(map[string]fillerreview.ReplacementCandidateReviewCase, len(review.Cases))
	for _, item := range review.Cases {
		if _, exists := inventoryByCase[item.CaseID]; !exists {
			return fillercandidatepool.Pool{}, fmt.Errorf("review authority contains unknown inventory case %q", item.CaseID)
		}
		reviewByCase[item.CaseID] = item
	}

	prior, priorInputs, err := fillerreview.OpenTemporalStructurePriorExposure(config.PriorAdjudicationPaths, config.GeneratedAt)
	if err != nil {
		return fillercandidatepool.Pool{}, err
	}
	priorExposure := convertExposure(prior)
	for _, input := range priorInputs {
		inputs = append(inputs, fillercandidatepool.Input{Name: input.Name, SHA256: input.SHA256})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	pool := fillercandidatepool.Pool{
		SchemaVersion: fillercandidatepool.SchemaVersion, ContractVersion: fillercandidatepool.ContractVersion,
		GeneratedAt: config.GeneratedAt.UTC(), Inputs: inputs, PriorExposure: priorExposure,
	}
	for _, item := range inventory.Cases {
		candidate, err := buildCandidate(config, item, decisionByCase[item.CaseID], materializedByCase[item.CaseID], reviewByCase[item.CaseID], quarantine, priorExposure)
		if err != nil {
			return fillercandidatepool.Pool{}, err
		}
		pool.Candidates = append(pool.Candidates, candidate)
	}
	resolveEligibleFamilyCollisions(pool.Candidates)
	sort.Slice(pool.Candidates, func(i, j int) bool { return pool.Candidates[i].CaseID < pool.Candidates[j].CaseID })
	if err := fillercandidatepool.Validate(pool); err != nil {
		return fillercandidatepool.Pool{}, err
	}
	return pool, nil
}

func buildCandidate(config Config, item fillercorpus.InventoryCase, decision fillercorpus.RightsDecision, materialized fillercorpus.MaterializedCase, review fillerreview.ReplacementCandidateReviewCase, quarantine fillerquarantine.RightsEligibility, prior fillercandidatepool.Exposure) (fillercandidatepool.Candidate, error) {
	candidate := fillercandidatepool.Candidate{
		CaseID: item.CaseID, Kind: fillercandidatepool.KindStandaloneAnchor,
		Disposition: fillercandidatepool.DispositionHeld, HoldReasons: []string{},
		FamilyID: unresolvedFamily(item.CaseID),
		Source: fillercandidatepool.Source{
			Transport: item.Representation.Transport, Authority: item.Authority, ItemID: item.ItemID,
			ItemURL: item.ItemURL, MetadataSHA256: item.MetadataSHA256, MetadataRetrievedAt: item.MetadataRetrievedAt,
			SoundtrackStatus:      item.Representation.Soundtrack.Status,
			SoundtrackEvidence:    item.Representation.Soundtrack.EvidenceKind,
			SoundtrackEvidenceSHA: item.Representation.Soundtrack.EvidenceSHA256,
		},
	}
	if item.Representation.Transport != fillercorpus.TransportLocal {
		candidate.Source.MediaURL = item.Representation.URL
	}
	if reason := fillercorpus.TemporalReplacementSoundtrackHoldReason(item.Representation.Soundtrack.Status); reason != "" {
		switch reason {
		case fillercorpus.HoldReasonSoundtrackIntentionallySilent:
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSoundtrackSilent)
		default:
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSoundtrackUnknown)
		}
	}
	if decision.CaseID == "" || decision.Decision == "held" {
		candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldRights)
	}

	var sourcePath, sourceSHA string
	if decision.Decision == "approved" {
		if item.Representation.Transport == fillercorpus.TransportLocal {
			sourcePath, sourceSHA = item.Representation.Path, item.Representation.SHA256
		} else if materialized.CaseID != "" {
			sourcePath, sourceSHA = materialized.LocalFile, materialized.ContentSHA256
		} else {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSourceBytes)
		}
	}
	if sourcePath != "" {
		actualSHA, _, err := verifySourceFile(config.SourceRoot, sourcePath, sourceSHA)
		if err != nil {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSourceBytes)
		} else if err := quarantine.Require(decision, actualSHA); err != nil {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldQuarantine)
		}
	}

	if review.CaseID == "" {
		candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSemanticUnit, fillercandidatepool.HoldTechnical, fillercandidatepool.HoldSuitability, fillercandidatepool.HoldFamily)
	} else {
		candidate.FamilyID = review.FamilyID
		candidate.Source.ID = "candidate-" + fillercandidatepool.Digest([]byte(item.CaseID))[:24]
		candidate.Source.Path = review.ReviewedMediaPath
		candidate.Source.SHA256 = review.ReviewedMediaSHA
		candidate.Source.Bytes = review.ReviewedMediaBytes
		candidate.Source.DurationMS = review.DurationMS
		if sourcePath == "" || filepath.ToSlash(filepath.Clean(review.InspectedSourceFile)) != filepath.ToSlash(filepath.Clean(sourcePath)) || review.InspectedSourceSHA != sourceSHA {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSourceBytes)
		}
		if _, _, err := verifySourceFile(config.SourceRoot, review.ReviewedMediaPath, review.ReviewedMediaSHA); err != nil {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSourceBytes)
		}
		if review.TechnicalFailure != "" || review.TechnicalVerdict != "continue" || !review.HadAudio || !review.FullDecodeMeasured {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldTechnical)
		}
		if !review.HadAudio {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSoundtrackDecodedAudio)
		}
		if review.Suitability != "candidate_no_signal_observed" {
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSuitability)
		}
		switch review.Unit {
		case fillereval.UnitStandalone:
			if review.Role == nil || *review.Role == fillereval.TemporalRoleUnclear {
				candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSemanticRole)
			} else {
				candidate.Role = string(*review.Role)
			}
			if review.Transition.EvidenceAlias == "" {
				candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldTransition)
			} else {
				candidate.Transition = convertTransition(review.Transition)
			}
		case fillereval.UnitProgrammeExcerpt:
			candidate.Kind = fillercandidatepool.KindProgrammeParent
			if review.DurationMS < 120_000 {
				candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldTechnical)
			}
		default:
			candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldSemanticUnit)
		}
	}
	if contains(prior.SourceSHA256, candidate.Source.SHA256) {
		candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldPriorSource)
	}
	if contains(prior.FamilyIDs, candidate.FamilyID) {
		candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldPriorFamily)
	}
	if candidate.Kind == fillercandidatepool.KindProgrammeParent && containsProvenance(prior.ProgrammeProvenance, item.Authority, fillercandidatepool.SourceReference(candidate)) {
		candidate.HoldReasons = append(candidate.HoldReasons, fillercandidatepool.HoldPriorProgrammeProvenance)
	}
	candidate.HoldReasons = sortedUnique(candidate.HoldReasons)
	if len(candidate.HoldReasons) == 0 {
		candidate.Disposition = fillercandidatepool.DispositionEligible
	}
	return candidate, nil
}

func validateConfig(config Config) error {
	if config.InventoryPath == "" || config.RightsDecisionsPath == "" || config.SourceRoot == "" ||
		(config.RightsProfile != fillercorpus.RightsProfileDevelopment && config.RightsProfile != fillercorpus.RightsProfileCertification) ||
		len(config.PriorAdjudicationPaths) == 0 || config.GeneratedAt.IsZero() || config.GeneratedAt.Location() != time.UTC {
		return errors.New("replacement candidate pool requires inventory, rights decisions/profile, source root, prior adjudication, and fixed UTC time")
	}
	return nil
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read authority %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maximumAuthorityBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximumAuthorityBytes {
		return nil, fmt.Errorf("authority %q exceeds %d bytes", path, maximumAuthorityBytes)
	}
	return raw, nil
}

func verifySourceFile(root, relative, expectedSHA string) (string, int64, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) == ".." || strings.HasPrefix(filepath.Clean(relative), ".."+string(filepath.Separator)) {
		return "", 0, errors.New("source path is unsafe")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", 0, err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filepath.FromSlash(relative)))
	if err != nil || resolved == resolvedRoot || !strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)) {
		return "", 0, errors.New("source path escapes its root")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", 0, errors.New("source is not a non-empty regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if digest != expectedSHA {
		return "", 0, errors.New("source content digest does not match authority")
	}
	return digest, info.Size(), nil
}

func resolveEligibleFamilyCollisions(candidates []fillercandidatepool.Candidate) {
	byFamily := map[string][]int{}
	for index := range candidates {
		if candidates[index].Disposition == fillercandidatepool.DispositionEligible {
			byFamily[candidates[index].FamilyID] = append(byFamily[candidates[index].FamilyID], index)
		}
	}
	for _, indices := range byFamily {
		if len(indices) < 2 {
			continue
		}
		sort.Slice(indices, func(i, j int) bool { return candidates[indices[i]].CaseID < candidates[indices[j]].CaseID })
		for _, index := range indices[1:] {
			candidates[index].Disposition = fillercandidatepool.DispositionHeld
			candidates[index].HoldReasons = []string{fillercandidatepool.HoldFamily}
			candidates[index].Role = ""
			candidates[index].Transition = nil
		}
	}
}

func convertExposure(value fillerreview.TemporalStructureHoldoutTrainingExclusion) fillercandidatepool.Exposure {
	result := fillercandidatepool.Exposure{SourceSHA256: append([]string(nil), value.SourceSHA256...), FamilyIDs: append([]string(nil), value.FamilyIDs...), ProgrammeProvenance: []fillercandidatepool.ProgrammeProvenance{}}
	for _, item := range value.ProgrammeProvenance {
		result.ProgrammeProvenance = append(result.ProgrammeProvenance, fillercandidatepool.ProgrammeProvenance{Authority: item.Authority, Reference: item.Reference})
	}
	return result
}

func convertTransition(value fillerreview.TemporalTransitionAuthorityCase) *fillercandidatepool.Transition {
	return &fillercandidatepool.Transition{EvidenceAlias: value.EvidenceAlias, Head: convertEdge(value.Head), Tail: convertEdge(value.Tail)}
}

func convertEdge(value fillerreview.TemporalTransitionEdge) fillercandidatepool.Edge {
	result := fillercandidatepool.Edge{StartMS: value.StartMS, EndMS: value.EndMS, RMSMilliDBFS: value.RMSMilliDBFS, PeakMilliDBFS: value.PeakMilliDBFS}
	for _, interval := range value.Black {
		result.Black = append(result.Black, fillercandidatepool.Interval{StartMS: interval.StartMs, EndMS: interval.EndMs})
	}
	for _, interval := range value.Silence {
		result.Silence = append(result.Silence, fillercandidatepool.Interval{StartMS: interval.StartMs, EndMS: interval.EndMs})
	}
	return result
}

func inputSHA(inputs []fillercandidatepool.Input, name string) string {
	for _, input := range inputs {
		if input.Name == name {
			return input.SHA256
		}
	}
	return ""
}

func unresolvedFamily(caseID string) string {
	return "unresolved-" + fillercandidatepool.Digest([]byte(caseID))[:24]
}

func sortedUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsProvenance(values []fillercandidatepool.ProgrammeProvenance, authority, reference string) bool {
	for _, value := range values {
		if value.Authority == authority && value.Reference == reference {
			return true
		}
	}
	return false
}
