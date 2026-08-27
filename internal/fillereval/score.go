package fillereval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"
)

const oneSided95Z = 1.6448536269514722

// Score evaluates captured predictions against a manifest. It is intentionally
// strict: missing/duplicate results and operational failures cannot certify.
func Score(manifest Manifest, predictions []Prediction, run RunIdentity) Report {
	report := Report{SchemaVersion: SchemaVersion, CorpusVersion: manifest.CorpusVersion, ManifestSHA256: ManifestSHA256(manifest), Run: run}
	report.Failures = append(report.Failures, ValidateManifest(manifest)...)
	if manifest.Kind == CorpusCertification {
		report.Failures = append(report.Failures, ValidateCertificationContract(manifest)...)
	}
	byID := make(map[string]Prediction, len(predictions))
	for _, prediction := range predictions {
		if _, exists := byID[prediction.CaseID]; exists {
			report.Failures = append(report.Failures, "duplicate prediction for "+prediction.CaseID)
			continue
		}
		byID[prediction.CaseID] = prediction
	}

	var eligible, invalid, reviews, answerable, probabilityCount int
	var admittedEligible, admittedRoleCorrect, admittedTaxonomyCorrect int
	var brier float64
	var latencies []int64
	type counts struct {
		total, correct int
		chargedNanoUSD int64
	}
	sliceCounts := map[string]counts{}
	rungCounts := map[string]counts{}
	var deterministicRejects, deterministicRejectsCorrect int
	var semanticRejects, semanticRejectsCorrect int
	var totalAttempts int

	for _, c := range manifest.Cases {
		if c.Split != run.EvaluationSplit {
			continue
		}
		switch c.Truth {
		case TruthEligible:
			eligible++
		case TruthInvalid:
			invalid++
		}
		prediction, ok := byID[c.ID]
		result := CaseResult{CaseID: c.ID, Slice: slices.Clone(c.Slices), Expected: c.Truth}
		if !ok {
			result.Failure = "missing prediction"
			report.Failures = append(report.Failures, c.ID+": missing prediction")
			report.Cases = append(report.Cases, result)
			continue
		}
		delete(byID, c.ID)
		accounting, accountingFailures := summarizePrediction(prediction)
		for _, failure := range accountingFailures {
			report.Failures = append(report.Failures, c.ID+": "+failure)
		}
		result.Actual = prediction.Verdict
		zeroInference := len(prediction.Steps) == 0
		deterministic := zeroInference && prediction.Verdict == VerdictReject && prediction.RejectClass == RejectDeterministic
		result.Role = accounting.Role
		result.Rung = accounting.Rung
		if deterministic {
			result.Rung = "deterministic"
		}
		result.RequestedProvider = accounting.RequestedProvider
		result.RequestedModel = accounting.RequestedModel
		result.ResolvedProvider = accounting.ResolvedProvider
		result.ResolvedModel = accounting.ResolvedModel
		result.UpstreamProvider = accounting.UpstreamProvider
		result.Modalities = slices.Clone(accounting.Modalities)
		result.Derivative = accounting.Derivative
		result.GenerationID = accounting.GenerationID
		result.Attempts = accounting.Attempts
		result.ChargedNanoUSD = accounting.ChargedNanoUSD
		result.LatencyMS = accounting.LatencyMS
		result.Steps = slices.Clone(prediction.Steps)
		if prediction.OperationalFailure == "" && prediction.Verdict != VerdictAdmit && prediction.Verdict != VerdictReject && prediction.Verdict != VerdictReview {
			result.Failure = "invalid verdict"
			report.Failures = append(report.Failures, c.ID+": invalid verdict")
		}
		missingRoute := accounting.Role == "" || accounting.Rung == "" || accounting.RequestedProvider == "" || accounting.RequestedModel == "" || len(accounting.Modalities) == 0
		missingResolution := accounting.ResolvedModel == "" || accounting.ResolvedProvider == ""
		requireAttribution := !deterministic && (prediction.OperationalFailure == "" || !zeroInference)
		if requireAttribution && (missingRoute || (prediction.OperationalFailure == "" && missingResolution)) {
			report.Failures = append(report.Failures, c.ID+": role, rung, model, provider, and modality attribution are required")
		}
		if requireAttribution && accounting.Attempts < 1 {
			report.Failures = append(report.Failures, c.ID+": at least one inference attempt is required")
		}
		if accounting.Attempts > 0 && totalAttempts > math.MaxInt-accounting.Attempts {
			report.Failures = append(report.Failures, c.ID+": inference attempt total overflow")
			totalAttempts = math.MaxInt
		} else {
			totalAttempts += accounting.Attempts
		}
		if prediction.Probability != nil && (*prediction.Probability < 0 || *prediction.Probability > 1) {
			report.Failures = append(report.Failures, c.ID+": probability must be within [0,1]")
		}
		evidenceIDs := make(map[string]struct{}, len(c.Evidence))
		for _, evidence := range c.Evidence {
			evidenceIDs[evidence.ID] = struct{}{}
		}
		for _, ref := range prediction.EvidenceRefs {
			if _, exists := evidenceIDs[ref]; !exists {
				report.Failures = append(report.Failures, c.ID+": unknown evidence reference "+ref)
			}
		}
		for _, conflict := range prediction.Conflicts {
			for _, ref := range conflict.EvidenceRefs {
				if _, exists := evidenceIDs[ref]; !exists {
					report.Failures = append(report.Failures, c.ID+": conflict has unknown evidence reference "+ref)
				}
			}
		}
		if prediction.OperationalFailure != "" {
			result.Failure = "operational failure: " + prediction.OperationalFailure
			report.Failures = append(report.Failures, c.ID+": "+result.Failure)
		} else {
			result.Correct = correct(c, prediction)
			if !result.Correct {
				result.Failure = fmt.Sprintf("expected %s, got %s", c.Truth, prediction.Verdict)
			}
		}
		if prediction.Verdict == VerdictAdmit {
			report.Metrics.AutoAdmit++
			if c.Truth == TruthEligible {
				admittedEligible++
				if prediction.ContentRole == c.ContentRole {
					admittedRoleCorrect++
				}
				if taxonomyContains(prediction.Taxonomy, c.Taxonomy) {
					admittedTaxonomyCorrect++
				}
			}
			if result.Correct {
				report.Metrics.AutoAdmitCorrect++
			}
		}
		if prediction.Verdict == VerdictReject {
			report.Metrics.AutoReject++
			if c.Truth == TruthInvalid && prediction.RejectClass == c.RejectClass {
				report.Metrics.AutoRejectCorrect++
			}
			if prediction.RejectClass == RejectDeterministic {
				deterministicRejects++
				if c.Truth == TruthInvalid && c.RejectClass == RejectDeterministic {
					deterministicRejectsCorrect++
				}
			}
			if prediction.RejectClass == RejectSemantic {
				semanticRejects++
				if c.Truth == TruthInvalid && c.RejectClass == RejectSemantic {
					semanticRejectsCorrect++
				}
			}
		}
		if prediction.Verdict == VerdictReview {
			reviews++
			if strings.EqualFold(strings.TrimSpace(prediction.ReviewQuestion), strings.TrimSpace(c.ReviewQuestion)) && strings.TrimSpace(c.ReviewQuestion) != "" {
				answerable++
			}
		}
		if prediction.Probability != nil {
			probabilityCount++
			y := 0.0
			if c.Truth == TruthEligible {
				y = 1
			}
			d := *prediction.Probability - y
			brier += d * d
		}
		if accounting.ChargedNanoUSD > 0 && report.Metrics.TotalChargedNanoUSD > math.MaxInt64-accounting.ChargedNanoUSD {
			report.Failures = append(report.Failures, c.ID+": charged cost total overflow")
			report.Metrics.TotalChargedNanoUSD = math.MaxInt64
		} else {
			report.Metrics.TotalChargedNanoUSD += accounting.ChargedNanoUSD
		}
		latencies = append(latencies, accounting.LatencyMS)
		for _, slice := range c.Slices {
			n := sliceCounts[slice]
			n.total++
			n.chargedNanoUSD = saturatingCost(n.chargedNanoUSD, accounting.ChargedNanoUSD)
			if result.Correct {
				n.correct++
			}
			sliceCounts[slice] = n
		}
		if len(prediction.Steps) == 0 {
			rungName := accounting.Rung
			if deterministic {
				rungName = "deterministic"
			}
			if rungName != "" {
				rung := rungCounts[rungName]
				rung.total++
				rung.chargedNanoUSD = saturatingCost(rung.chargedNanoUSD, accounting.ChargedNanoUSD)
				if result.Correct {
					rung.correct++
				}
				rungCounts[rungName] = rung
			}
		} else {
			for _, step := range prediction.Steps {
				rung := rungCounts[step.Rung]
				rung.total++
				rung.chargedNanoUSD = saturatingCost(rung.chargedNanoUSD, step.ChargedNanoUSD)
				if result.Correct {
					rung.correct++
				}
				rungCounts[step.Rung] = rung
			}
		}
		report.Cases = append(report.Cases, result)
	}
	for id := range byID {
		report.Failures = append(report.Failures, id+": prediction has no corpus case")
	}

	report.Metrics.Cases = len(report.Cases)
	report.Metrics.AutoAdmitPrecision = ratio(report.Metrics.AutoAdmitCorrect, report.Metrics.AutoAdmit)
	report.Metrics.AutoAdmitPrecisionLower = wilsonLower(report.Metrics.AutoAdmitCorrect, report.Metrics.AutoAdmit)
	report.Metrics.ValidAutomation = ratio(report.Metrics.AutoAdmitCorrect, eligible)
	report.Metrics.ValidAutomationLower = wilsonLower(report.Metrics.AutoAdmitCorrect, eligible)
	report.Metrics.AutoRejectPrecision = ratio(report.Metrics.AutoRejectCorrect, report.Metrics.AutoReject)
	report.Metrics.AutoRejectPrecisionLower = wilsonLower(report.Metrics.AutoRejectCorrect, report.Metrics.AutoReject)
	report.Metrics.InvalidAutomation = ratio(report.Metrics.AutoRejectCorrect, invalid)
	report.Metrics.InvalidAutomationLower = wilsonLower(report.Metrics.AutoRejectCorrect, invalid)
	report.Metrics.DeterministicRejectPrecision = ratio(deterministicRejectsCorrect, deterministicRejects)
	report.Metrics.DeterministicRejectPrecisionLower = wilsonLower(deterministicRejectsCorrect, deterministicRejects)
	report.Metrics.SemanticRejectPrecision = ratio(semanticRejectsCorrect, semanticRejects)
	report.Metrics.SemanticRejectPrecisionLower = wilsonLower(semanticRejectsCorrect, semanticRejects)
	report.Metrics.ReviewRate = ratio(reviews, report.Metrics.Cases)
	report.Metrics.ReviewRateUpper = wilsonUpper(reviews, report.Metrics.Cases)
	report.Metrics.ReviewAnswerable = ratio(answerable, reviews)
	report.Metrics.ReviewAnswerableLower = wilsonLower(answerable, reviews)
	report.Metrics.AdmittedRoleAccuracy = ratio(admittedRoleCorrect, admittedEligible)
	report.Metrics.AdmittedTaxonomyAccuracy = ratio(admittedTaxonomyCorrect, admittedEligible)
	if probabilityCount > 0 {
		report.Metrics.BrierScore = brier / float64(probabilityCount)
	}
	report.Metrics.TotalChargedCostUSD = float64(report.Metrics.TotalChargedNanoUSD) / 1_000_000_000
	report.Metrics.CostPerThousandCasesNanoUSD = perUnit(report.Metrics.TotalChargedNanoUSD, 1000, report.Metrics.Cases)
	report.Metrics.CostPerCorrectAutomationNanoUSD = perUnit(report.Metrics.TotalChargedNanoUSD, 1, report.Metrics.AutoAdmitCorrect+report.Metrics.AutoRejectCorrect)
	report.Metrics.CostPerAdmitNanoUSD = perUnit(report.Metrics.TotalChargedNanoUSD, 1, report.Metrics.AutoAdmit)
	slices.Sort(latencies)
	report.Metrics.P50LatencyMS = percentile(latencies, .50)
	report.Metrics.P95LatencyMS = percentile(latencies, .95)
	for name, n := range sliceCounts {
		report.Slices = append(report.Slices, SliceScore{
			Slice: name, Cases: n.total, Correct: n.correct, Accuracy: ratio(n.correct, n.total), AccuracyLower: wilsonLower(n.correct, n.total),
			ChargedNanoUSD: n.chargedNanoUSD, CostPerCorrectNanoUSD: perUnit(n.chargedNanoUSD, 1, n.correct),
		})
	}
	sort.Slice(report.Slices, func(i, j int) bool { return report.Slices[i].Slice < report.Slices[j].Slice })
	for name, n := range rungCounts {
		report.Metrics.Rungs = append(report.Metrics.Rungs, RungScore{Rung: name, Cases: n.total, Correct: n.correct, ChargedNanoUSD: n.chargedNanoUSD})
	}
	sort.Slice(report.Metrics.Rungs, func(i, j int) bool { return report.Metrics.Rungs[i].Rung < report.Metrics.Rungs[j].Rung })

	if manifest.Kind == CorpusCertification {
		applyGates(&report, manifest.SliceGates, eligible, invalid, deterministicRejects, semanticRejects, totalAttempts)
		report.Certified = len(report.Failures) == 0
	} else {
		applyDevelopmentGates(&report, manifest.SliceGates, totalAttempts)
		report.Certified = false
	}
	return report
}

func summarizePrediction(prediction Prediction) (InferenceStep, []string) {
	if len(prediction.Steps) == 0 {
		return InferenceStep{}, nil
	}
	var total InferenceStep
	seen := make(map[string]struct{}, len(prediction.Steps))
	var failures []string
	for i, step := range prediction.Steps {
		prefix := fmt.Sprintf("step[%d]", i)
		if step.EvaluationID == "" {
			failures = append(failures, prefix+": evaluation id is required")
		} else if _, exists := seen[step.EvaluationID]; exists {
			failures = append(failures, prefix+": duplicate evaluation id "+step.EvaluationID)
		} else {
			seen[step.EvaluationID] = struct{}{}
		}
		missingRoute := step.Role == "" || step.Rung == "" || step.RequestedProvider == "" || step.RequestedModel == "" || len(step.Modalities) == 0
		missingResolution := step.ResolvedModel == "" || step.ResolvedProvider == ""
		if missingRoute || (step.OperationalFailure == "" && missingResolution) {
			failures = append(failures, prefix+": role, rung, model, provider, and modality attribution are required")
		}
		if step.Attempts < 1 {
			failures = append(failures, prefix+": at least one inference attempt is required")
		}
		if step.Abstained != (strings.TrimSpace(step.AbstentionReason) != "") || (step.Abstained && step.OperationalFailure != "") {
			failures = append(failures, prefix+": semantic abstention requires one reason and cannot be an operational failure")
		}
		if err := validateAccounting(step); err != nil {
			failures = append(failures, prefix+": "+err.Error())
		}
		addAccountingValue(&total.Derivative.Bytes, step.Derivative.Bytes, prefix, "derivative bytes", &failures)
		addAccountingValue(&total.Derivative.DurationMS, step.Derivative.DurationMS, prefix, "derivative duration", &failures)
		addAccountingValue(&total.Derivative.Pixels, step.Derivative.Pixels, prefix, "derivative pixels", &failures)
		addAccountingValue(&total.Tokens.Prompt, step.Tokens.Prompt, prefix, "prompt tokens", &failures)
		addAccountingValue(&total.Tokens.Completion, step.Tokens.Completion, prefix, "completion tokens", &failures)
		addAccountingValue(&total.Tokens.Reasoning, step.Tokens.Reasoning, prefix, "reasoning tokens", &failures)
		addAccountingValue(&total.Tokens.Cached, step.Tokens.Cached, prefix, "cached tokens", &failures)
		addAccountingValue(&total.Tokens.CacheWrite, step.Tokens.CacheWrite, prefix, "cache-write tokens", &failures)
		addAccountingValue(&total.Tokens.Image, step.Tokens.Image, prefix, "image tokens", &failures)
		addAccountingValue(&total.Tokens.Audio, step.Tokens.Audio, prefix, "audio tokens", &failures)
		addAccountingValue(&total.Tokens.Video, step.Tokens.Video, prefix, "video tokens", &failures)
		addAccountingValue(&total.ChargedNanoUSD, step.ChargedNanoUSD, prefix, "charged cost", &failures)
		addAccountingValue(&total.EstimatedNanoUSD, step.EstimatedNanoUSD, prefix, "estimated cost", &failures)
		if step.Attempts > 0 && total.Attempts > math.MaxInt-step.Attempts {
			failures = append(failures, prefix+": inference attempt total overflow")
			total.Attempts = math.MaxInt
		} else {
			total.Attempts += step.Attempts
		}
		addAccountingValue(&total.LatencyMS, step.LatencyMS, prefix, "latency", &failures)
	}
	terminal := prediction.Steps[len(prediction.Steps)-1]
	terminal.Derivative = total.Derivative
	terminal.Tokens = total.Tokens
	terminal.ChargedAmount = ""
	terminal.ChargedCurrency = ""
	terminal.ChargedNanoUSD = total.ChargedNanoUSD
	terminal.EstimatedNanoUSD = total.EstimatedNanoUSD
	terminal.Attempts = total.Attempts
	terminal.LatencyMS = total.LatencyMS
	return terminal, failures
}

func saturatingCost(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func addAccountingValue(dst *int64, value int64, prefix, label string, failures *[]string) {
	if value > 0 && *dst > math.MaxInt64-value {
		*failures = append(*failures, prefix+": "+label+" total overflow")
		*dst = math.MaxInt64
		return
	}
	*dst += value
}

func validateAccounting(step InferenceStep) error {
	values := []int64{
		step.Derivative.Bytes, step.Derivative.DurationMS, step.Derivative.Pixels,
		step.Tokens.Prompt, step.Tokens.Completion, step.Tokens.Reasoning,
		step.Tokens.Cached, step.Tokens.CacheWrite, step.Tokens.Image,
		step.Tokens.Audio, step.Tokens.Video, step.ChargedNanoUSD,
		step.EstimatedNanoUSD, step.LatencyMS,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("accounting values cannot be negative")
		}
	}
	if step.ChargedAmount == "" {
		if step.ChargedCurrency != "" || step.ChargedNanoUSD != 0 {
			return fmt.Errorf("charged amount, currency, and nanodollars must be present together")
		}
		return nil
	}
	if step.ChargedCurrency == "" {
		return fmt.Errorf("charged amount requires its provider-reported currency")
	}
	if step.ChargedCurrency != "USD" {
		if step.ChargedNanoUSD != 0 {
			return fmt.Errorf("non-USD provider charge cannot be projected without an exchange-rate snapshot")
		}
		return nil
	}
	nano, err := USDToNanoCeil(step.ChargedAmount)
	if err != nil {
		return fmt.Errorf("invalid charged amount: %w", err)
	}
	if nano != step.ChargedNanoUSD {
		return fmt.Errorf("charged amount projects to %d nanodollars, got %d", nano, step.ChargedNanoUSD)
	}
	return nil
}

// USDToNanoCeil converts exact provider decimal text to an integer budget unit.
// Sub-nanodollar charges round up so a budget reservation never understates spend.
func USDToNanoCeil(amount string) (int64, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(amount))
	if !ok || r.Sign() < 0 {
		return 0, fmt.Errorf("invalid non-negative USD decimal %q", amount)
	}
	scaled := new(big.Rat).Mul(r, big.NewRat(1_000_000_000, 1))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(scaled.Num(), scaled.Denom(), rem)
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("USD decimal %q exceeds nanodollar range", amount)
	}
	return q.Int64(), nil
}

func perUnit(nano int64, multiplier, denominator int) int64 {
	if nano <= 0 || multiplier <= 0 || denominator <= 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(nano), big.NewInt(int64(multiplier)))
	d := big.NewInt(int64(denominator))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n, d, rem)
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return math.MaxInt64
	}
	return q.Int64()
}

// ValidateManifest rejects corpus drift that would make a score look stronger
// than its evidence, especially duplicate leakage across development and holdout.
func ValidateManifest(manifest Manifest) []string {
	var failures []string
	if manifest.SchemaVersion != SchemaVersion {
		failures = append(failures, fmt.Sprintf("manifest schema %d, want %d", manifest.SchemaVersion, SchemaVersion))
	}
	if strings.TrimSpace(manifest.CorpusVersion) == "" {
		failures = append(failures, "corpus version is required")
	}
	if manifest.Kind != CorpusDevelopmentSeed && manifest.Kind != CorpusCertification {
		failures = append(failures, "manifest kind must be development_seed or certification")
	}
	if manifest.Kind == CorpusCertification && manifest.LockedAt.IsZero() {
		failures = append(failures, "certification corpus requires a lock time")
	}
	ids := map[string]struct{}{}
	clusters := map[string]Split{}
	contentClusters := map[string]string{}
	campaignSplits := map[string]Split{}
	familySplits := map[string]Split{}
	splitCounts := map[Split]int{}
	if len(manifest.SliceGates) == 0 {
		failures = append(failures, "at least one safety-critical slice gate is required")
	}
	gateNames := map[string]struct{}{}
	for _, gate := range manifest.SliceGates {
		if gate.Slice == "" || gate.MinCases <= 0 || gate.MinAccuracy <= 0 || gate.MinAccuracy > 1 || gate.MinAccuracyLower < 0 || gate.MinAccuracyLower > 1 {
			failures = append(failures, "slice gate requires a name, positive case count, and accuracy within (0,1]")
		}
		if manifest.Kind == CorpusCertification && gate.MinAccuracyLower <= 0 {
			failures = append(failures, "certification slice gate "+gate.Slice+" requires a positive confidence lower bound")
		}
		if _, exists := gateNames[gate.Slice]; exists {
			failures = append(failures, "duplicate slice gate "+gate.Slice)
		}
		gateNames[gate.Slice] = struct{}{}
	}
	for i, c := range manifest.Cases {
		prefix := fmt.Sprintf("case[%d]", i)
		if c.ID == "" {
			failures = append(failures, prefix+": id is required")
		} else if _, exists := ids[c.ID]; exists {
			failures = append(failures, c.ID+": duplicate case id")
		} else {
			ids[c.ID] = struct{}{}
			prefix = c.ID
		}
		if c.Split != SplitDevelopment && c.Split != SplitHoldout {
			failures = append(failures, prefix+": invalid split")
		} else {
			splitCounts[c.Split]++
		}
		if strings.TrimSpace(c.Cluster) == "" {
			failures = append(failures, prefix+": similarity cluster is required")
		} else if split, exists := clusters[c.Cluster]; exists && split != c.Split {
			failures = append(failures, prefix+": similarity cluster crosses development and holdout")
		} else {
			clusters[c.Cluster] = c.Split
		}
		if strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.License) == "" {
			failures = append(failures, prefix+": source and license are required")
		}
		failures = append(failures, validateLabels(prefix, LabelsFromCase(c))...)
		if manifest.Kind == CorpusCertification {
			failures = append(failures, validateCertificationCase(prefix, c, manifest.LockedAt)...)
			failures = append(failures, trackCertificationSplit(prefix, "campaign", strings.TrimSpace(c.Provenance.Campaign), c.Split, campaignSplits)...)
			failures = append(failures, trackCertificationSplit(prefix, "source family", strings.TrimSpace(c.Provenance.SourceFamily), c.Split, familySplits)...)
			if previousCluster, exists := contentClusters[c.ContentSHA256]; exists && previousCluster != c.Cluster {
				failures = append(failures, prefix+": identical content hash appears in a different similarity cluster")
			} else if c.ContentSHA256 != "" {
				contentClusters[c.ContentSHA256] = c.Cluster
			}
		}
	}
	if manifest.Kind == CorpusCertification && (splitCounts[SplitDevelopment] == 0 || splitCounts[SplitHoldout] == 0) {
		failures = append(failures, "certification corpus requires non-empty development and holdout splits")
	}
	return failures
}

// ValidateDevelopmentRun proves that a development seed is ready for the
// non-certifying model-selection run. Draft and partially reviewed corpora are
// rejected before a provider can see their packet set.
func ValidateDevelopmentRun(manifest Manifest) []string {
	var failures []string
	if manifest.Kind != CorpusDevelopmentSeed {
		return []string{"development run requires a development_seed manifest"}
	}
	failures = append(failures, ValidateManifest(manifest)...)
	if manifest.LockedAt.IsZero() {
		failures = append(failures, "development run requires a lock time")
	}
	if len(manifest.Cases) < CertificationMinDevelopment {
		failures = append(failures, fmt.Sprintf("development run has %d cases; require at least %d", len(manifest.Cases), CertificationMinDevelopment))
	}
	for _, c := range manifest.Cases {
		if c.Split != SplitDevelopment {
			failures = append(failures, c.ID+": development run may contain only development cases")
		}
		failures = append(failures, validateLabelLock(c.ID, c, manifest.LockedAt)...)
	}
	return failures
}

func trackCertificationSplit(prefix, name, value string, split Split, seen map[string]Split) []string {
	if value == "" {
		return nil
	}
	if prior, exists := seen[value]; exists && prior != split {
		return []string{prefix + ": " + name + " crosses development and holdout"}
	}
	seen[value] = split
	return nil
}

func validateCertificationCase(prefix string, c Case, lockedAt time.Time) []string {
	var failures []string
	if !isSHA256(c.ContentSHA256) || !isSHA256(c.EvidenceSHA256) {
		failures = append(failures, prefix+": certification requires lowercase SHA-256 media and evidence hashes")
	}
	p := c.Provenance
	if strings.TrimSpace(p.Authority) == "" || strings.TrimSpace(p.ItemID) == "" || strings.TrimSpace(p.ItemRef) == "" || p.MetadataRetrievedAt.IsZero() || !isSHA256(p.MetadataSHA256) || strings.TrimSpace(p.EvidenceRef) == "" {
		failures = append(failures, prefix+": source authority, item identity, metadata retrieval, metadata hash, and evidence reference are required")
	}
	if strings.TrimSpace(p.RightsStatement) == "" || strings.TrimSpace(p.RightsDecision) == "" || strings.TrimSpace(p.RightsReviewerID) == "" || p.RightsReviewedAt.IsZero() {
		failures = append(failures, prefix+": item-level rights evidence and adjudication are required")
	}
	if p.MetadataRetrievedAt.After(lockedAt) || p.RightsReviewedAt.After(lockedAt) {
		failures = append(failures, prefix+": source metadata and rights review cannot postdate the manifest lock")
	}
	if strings.TrimSpace(p.SourceFilename) == "" || strings.TrimSpace(p.SourceRef) == "" || p.SourceBytes <= 0 || p.SegmentStartMS < 0 || p.SegmentDurationMS <= 0 {
		failures = append(failures, prefix+": source file identity, positive size, and bounded segment are required")
	}
	if strings.TrimSpace(p.Campaign) == "" || strings.TrimSpace(p.SourceFamily) == "" {
		failures = append(failures, prefix+": certification requires campaign and source-family identity")
	}
	failures = append(failures, validateLabelLock(prefix, c, lockedAt)...)
	return failures
}

func validateLabelLock(prefix string, c Case, lockedAt time.Time) []string {
	var failures []string
	wantLabelHash := LabelSHA256(c)
	if len(c.Evidence) == 0 {
		failures = append(failures, prefix+": independently reviewed evidence labels are required")
	}
	reviewers := map[string]struct{}{}
	batches := map[string]struct{}{}
	needsAdjudication := false
	for _, review := range c.LabelReviews {
		if strings.TrimSpace(review.ReviewerID) == "" || strings.TrimSpace(review.BatchID) == "" || review.ReviewedAt.IsZero() || !review.Independent || !isSHA256(review.SubmissionSHA256) {
			continue
		}
		if review.ReviewedAt.After(lockedAt) {
			failures = append(failures, prefix+": label review cannot postdate the manifest lock")
			continue
		}
		if review.SubmissionSHA256 != wantLabelHash {
			needsAdjudication = true
		}
		reviewers[review.ReviewerID] = struct{}{}
		batches[review.BatchID] = struct{}{}
	}
	if len(reviewers) < 2 || len(batches) < 2 {
		failures = append(failures, prefix+": two independent label submissions are required")
	}
	if needsAdjudication {
		a := c.Adjudication
		if a == nil || strings.TrimSpace(a.AdjudicatorID) == "" || a.AdjudicatedAt.IsZero() || a.LabelSHA256 != wantLabelHash || strings.TrimSpace(a.Reason) == "" {
			failures = append(failures, prefix+": divergent independent labels require final adjudication")
		} else if _, reviewerWasAdjudicator := reviewers[a.AdjudicatorID]; reviewerWasAdjudicator {
			failures = append(failures, prefix+": adjudicator must be independent from the two label reviewers")
		} else if a.AdjudicatedAt.After(lockedAt) {
			failures = append(failures, prefix+": adjudication cannot postdate the manifest lock")
		}
	}
	return failures
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// ManifestSHA256 gives reports an exact corpus identity, not merely a mutable
// human-readable version string.
func ManifestSHA256(manifest Manifest) string {
	data, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// LabelSHA256 is the attestation target for independent corpus reviewers.
func LabelSHA256(c Case) string {
	return LabelsSHA256(LabelsFromCase(c))
}

func correct(c Case, prediction Prediction) bool {
	switch c.Truth {
	case TruthEligible:
		return prediction.Verdict == VerdictAdmit && prediction.ContentRole == c.ContentRole && taxonomyContains(prediction.Taxonomy, c.Taxonomy) && containsAll(prediction.PolicyFlags, c.PolicyFlags)
	case TruthInvalid:
		return prediction.Verdict == VerdictReject && prediction.RejectClass == c.RejectClass
	case TruthAmbiguous:
		return prediction.Verdict == VerdictReview
	default:
		return false
	}
}

func taxonomyContains(actual, expected map[string][]string) bool {
	for axis, tags := range expected {
		if !containsAll(actual[axis], tags) {
			return false
		}
	}
	return true
}

func containsAll(actual, expected []string) bool {
	set := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		set[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func applyGates(r *Report, gates []SliceGate, eligible, invalid, deterministicRejects, semanticRejects, totalAttempts int) {
	applyRunBounds(r, totalAttempts)
	if r.Metrics.Cases < CertificationMinHoldout {
		r.Failures = append(r.Failures, fmt.Sprintf("corpus has %d cases; certification requires at least %d independent holdout cases", r.Metrics.Cases, CertificationMinHoldout))
	}
	if r.Metrics.AutoAdmit == 0 || r.Metrics.AutoAdmitPrecision < .99 || r.Metrics.AutoAdmitPrecisionLower < .99 {
		r.Failures = append(r.Failures, fmt.Sprintf("auto-admit precision %.4f (one-sided 95%% lower %.4f), require both >= 0.99", r.Metrics.AutoAdmitPrecision, r.Metrics.AutoAdmitPrecisionLower))
	}
	if deterministicRejects == 0 || r.Metrics.DeterministicRejectPrecision < .99 || r.Metrics.DeterministicRejectPrecisionLower < .99 {
		r.Failures = append(r.Failures, fmt.Sprintf("deterministic reject precision %.4f (one-sided 95%% lower %.4f), require both >= 0.99", r.Metrics.DeterministicRejectPrecision, r.Metrics.DeterministicRejectPrecisionLower))
	}
	if semanticRejects == 0 || r.Metrics.SemanticRejectPrecision < .97 || r.Metrics.SemanticRejectPrecisionLower < .97 {
		r.Failures = append(r.Failures, fmt.Sprintf("semantic reject precision %.4f (one-sided 95%% lower %.4f), require both >= 0.97", r.Metrics.SemanticRejectPrecision, r.Metrics.SemanticRejectPrecisionLower))
	}
	if eligible == 0 || r.Metrics.ValidAutomation < .90 || r.Metrics.ValidAutomationLower < .90 {
		r.Failures = append(r.Failures, fmt.Sprintf("valid filler automation %.4f (one-sided 95%% lower %.4f), require both >= 0.90", r.Metrics.ValidAutomation, r.Metrics.ValidAutomationLower))
	}
	if invalid == 0 || r.Metrics.InvalidAutomation < .95 || r.Metrics.InvalidAutomationLower < .95 {
		r.Failures = append(r.Failures, fmt.Sprintf("invalid input automation %.4f (one-sided 95%% lower %.4f), require both >= 0.95", r.Metrics.InvalidAutomation, r.Metrics.InvalidAutomationLower))
	}
	if r.Metrics.ReviewRateUpper > .10 {
		r.Failures = append(r.Failures, fmt.Sprintf("review rate %.4f (one-sided 95%% upper %.4f), require upper <= 0.10", r.Metrics.ReviewRate, r.Metrics.ReviewRateUpper))
	}
	if r.Metrics.ReviewRate > 0 && (r.Metrics.ReviewAnswerable < .95 || r.Metrics.ReviewAnswerableLower < .95) {
		r.Failures = append(r.Failures, fmt.Sprintf("answerable review %.4f (one-sided 95%% lower %.4f), require both >= 0.95", r.Metrics.ReviewAnswerable, r.Metrics.ReviewAnswerableLower))
	}
	applySliceGates(r, gates)
}

func applyDevelopmentGates(r *Report, gates []SliceGate, totalAttempts int) {
	applyRunBounds(r, totalAttempts)
	applySliceGates(r, gates)
}

func applyRunBounds(r *Report, totalAttempts int) {
	if r.Run.Profile == "" || (r.Run.EvaluationSplit != SplitDevelopment && r.Run.EvaluationSplit != SplitHoldout) || r.Run.EvidenceVersion == "" || r.Run.PromptVersion == "" || r.Run.TaxonomyVersion == "" || r.Run.PolicyVersion == "" || r.Run.RolePolicyVersion == "" || r.Run.CapabilitySnapshot == "" || r.Run.PriceSnapshot == "" || r.Run.GeneratedAt.IsZero() {
		r.Failures = append(r.Failures, "run identity requires profile, evidence, prompt, taxonomy, admission policy, role policy, capability, and price versions")
	}
	if r.Run.MaxRequests <= 0 || r.Run.MaxSpendNanoUSD <= 0 || r.Run.MaxConcurrency <= 0 {
		r.Failures = append(r.Failures, "run identity requires positive request, spend, and concurrency ceilings")
	}
	if totalAttempts > r.Run.MaxRequests {
		r.Failures = append(r.Failures, fmt.Sprintf("run used %d attempts; request ceiling is %d", totalAttempts, r.Run.MaxRequests))
	}
	if r.Metrics.TotalChargedNanoUSD > r.Run.MaxSpendNanoUSD {
		r.Failures = append(r.Failures, fmt.Sprintf("run charged %d nanodollars; spend ceiling is %d", r.Metrics.TotalChargedNanoUSD, r.Run.MaxSpendNanoUSD))
	}
}

func applySliceGates(r *Report, gates []SliceGate) {
	scores := make(map[string]SliceScore, len(r.Slices))
	for _, score := range r.Slices {
		scores[score.Slice] = score
	}
	for _, gate := range gates {
		score := scores[gate.Slice]
		if score.Cases < gate.MinCases || score.Accuracy < gate.MinAccuracy || score.AccuracyLower < gate.MinAccuracyLower {
			r.Failures = append(r.Failures, fmt.Sprintf("slice %s has %d cases at %.4f accuracy (one-sided 95%% lower %.4f); require %d at %.4f with lower %.4f", gate.Slice, score.Cases, score.Accuracy, score.AccuracyLower, gate.MinCases, gate.MinAccuracy, gate.MinAccuracyLower))
		}
	}
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func wilsonLower(successes, trials int) float64 {
	if trials == 0 {
		return 0
	}
	n := float64(trials)
	p := float64(successes) / n
	z2 := oneSided95Z * oneSided95Z
	center := p + z2/(2*n)
	margin := oneSided95Z * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (center - margin) / (1 + z2/n)
}

func wilsonUpper(successes, trials int) float64 {
	if trials == 0 {
		return 0
	}
	n := float64(trials)
	p := float64(successes) / n
	z2 := oneSided95Z * oneSided95Z
	center := p + z2/(2*n)
	margin := oneSided95Z * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (center + margin) / (1 + z2/n)
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
