package fillereval

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

const oneSided95Z = 1.6448536269514722

// Score evaluates captured predictions against a manifest. It is intentionally
// strict: missing/duplicate results and operational failures cannot certify.
func Score(manifest Manifest, predictions []Prediction, run RunIdentity) Report {
	if run.GeneratedAt.IsZero() {
		run.GeneratedAt = time.Now().UTC()
	}
	report := Report{SchemaVersion: SchemaVersion, CorpusVersion: manifest.CorpusVersion, Run: run}
	report.Failures = append(report.Failures, ValidateManifest(manifest)...)
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
	type counts struct{ total, correct int }
	sliceCounts := map[string]counts{}
	var deterministicRejects, deterministicRejectsCorrect int
	var semanticRejects, semanticRejectsCorrect int

	for _, c := range manifest.Cases {
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
		result.Actual = prediction.Verdict
		if prediction.Verdict != VerdictAdmit && prediction.Verdict != VerdictReject && prediction.Verdict != VerdictReview {
			result.Failure = "invalid verdict"
			report.Failures = append(report.Failures, c.ID+": invalid verdict")
		}
		if prediction.RequestedModel == "" || prediction.ResolvedModel == "" || prediction.ResolvedProvider == "" || len(prediction.Modalities) == 0 {
			report.Failures = append(report.Failures, c.ID+": model, provider, and modality attribution are required")
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
		if prediction.ChargedCostUSD < 0 {
			report.Failures = append(report.Failures, c.ID+": charged cost cannot be negative")
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
		report.Metrics.TotalChargedCostUSD += prediction.ChargedCostUSD
		latencies = append(latencies, prediction.LatencyMS)
		for _, slice := range c.Slices {
			n := sliceCounts[slice]
			n.total++
			if result.Correct {
				n.correct++
			}
			sliceCounts[slice] = n
		}
		report.Cases = append(report.Cases, result)
	}
	for id := range byID {
		report.Failures = append(report.Failures, id+": prediction has no corpus case")
	}

	report.Metrics.Cases = len(manifest.Cases)
	report.Metrics.AutoAdmitPrecision = ratio(report.Metrics.AutoAdmitCorrect, report.Metrics.AutoAdmit)
	report.Metrics.AutoAdmitPrecisionLower = wilsonLower(report.Metrics.AutoAdmitCorrect, report.Metrics.AutoAdmit)
	report.Metrics.ValidAutomation = ratio(report.Metrics.AutoAdmitCorrect, eligible)
	report.Metrics.AutoRejectPrecision = ratio(report.Metrics.AutoRejectCorrect, report.Metrics.AutoReject)
	report.Metrics.InvalidAutomation = ratio(report.Metrics.AutoRejectCorrect, invalid)
	report.Metrics.DeterministicRejectPrecision = ratio(deterministicRejectsCorrect, deterministicRejects)
	report.Metrics.SemanticRejectPrecision = ratio(semanticRejectsCorrect, semanticRejects)
	report.Metrics.ReviewRate = ratio(reviews, len(manifest.Cases))
	report.Metrics.ReviewAnswerable = ratio(answerable, reviews)
	report.Metrics.AdmittedRoleAccuracy = ratio(admittedRoleCorrect, admittedEligible)
	report.Metrics.AdmittedTaxonomyAccuracy = ratio(admittedTaxonomyCorrect, admittedEligible)
	if probabilityCount > 0 {
		report.Metrics.BrierScore = brier / float64(probabilityCount)
	}
	if len(manifest.Cases) > 0 {
		report.Metrics.CostPerThousandCasesUSD = report.Metrics.TotalChargedCostUSD * 1000 / float64(len(manifest.Cases))
	}
	slices.Sort(latencies)
	report.Metrics.P50LatencyMS = percentile(latencies, .50)
	report.Metrics.P95LatencyMS = percentile(latencies, .95)
	for name, n := range sliceCounts {
		report.Slices = append(report.Slices, SliceScore{Slice: name, Cases: n.total, Correct: n.correct, Accuracy: ratio(n.correct, n.total)})
	}
	sort.Slice(report.Slices, func(i, j int) bool { return report.Slices[i].Slice < report.Slices[j].Slice })

	applyGates(&report, manifest.SliceGates, eligible, invalid, deterministicRejects, semanticRejects)
	report.Certified = len(report.Failures) == 0
	return report
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
	ids := map[string]struct{}{}
	clusters := map[string]Split{}
	if len(manifest.SliceGates) == 0 {
		failures = append(failures, "at least one safety-critical slice gate is required")
	}
	gateNames := map[string]struct{}{}
	for _, gate := range manifest.SliceGates {
		if gate.Slice == "" || gate.MinCases <= 0 || gate.MinAccuracy <= 0 || gate.MinAccuracy > 1 {
			failures = append(failures, "slice gate requires a name, positive case count, and accuracy within (0,1]")
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
		}
		if c.Truth != TruthEligible && c.Truth != TruthInvalid && c.Truth != TruthAmbiguous {
			failures = append(failures, prefix+": invalid truth")
		}
		if c.Truth == TruthInvalid && c.RejectClass != RejectDeterministic && c.RejectClass != RejectSemantic {
			failures = append(failures, prefix+": invalid truth requires a reject class")
		}
		if c.Truth == TruthAmbiguous && strings.TrimSpace(c.ReviewQuestion) == "" {
			failures = append(failures, prefix+": ambiguous truth requires a review question")
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
		if len(c.Slices) == 0 {
			failures = append(failures, prefix+": at least one slice is required")
		}
		evidenceIDs := map[string]struct{}{}
		for _, evidence := range c.Evidence {
			if evidence.ID == "" || evidence.Kind == "" || evidence.Claim == "" || evidence.Provenance == "" {
				failures = append(failures, prefix+": evidence requires id, kind, claim, and provenance")
			}
			if _, exists := evidenceIDs[evidence.ID]; exists {
				failures = append(failures, prefix+": duplicate evidence id "+evidence.ID)
			}
			evidenceIDs[evidence.ID] = struct{}{}
		}
	}
	return failures
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

func applyGates(r *Report, gates []SliceGate, eligible, invalid, deterministicRejects, semanticRejects int) {
	if r.Run.Profile == "" || r.Run.EvidenceVersion == "" || r.Run.PromptVersion == "" || r.Run.TaxonomyVersion == "" || r.Run.PolicyVersion == "" || r.Run.CapabilitySnapshot == "" || r.Run.PriceSnapshot == "" {
		r.Failures = append(r.Failures, "run identity requires profile, evidence, prompt, taxonomy, policy, capability, and price versions")
	}
	if r.Metrics.Cases < 300 {
		r.Failures = append(r.Failures, fmt.Sprintf("corpus has %d cases; certification requires at least 300", r.Metrics.Cases))
	}
	if r.Metrics.AutoAdmit == 0 || r.Metrics.AutoAdmitPrecision < .99 || r.Metrics.AutoAdmitPrecisionLower < .99 {
		r.Failures = append(r.Failures, fmt.Sprintf("auto-admit precision %.4f (one-sided 95%% lower %.4f), require both >= 0.99", r.Metrics.AutoAdmitPrecision, r.Metrics.AutoAdmitPrecisionLower))
	}
	if deterministicRejects == 0 || r.Metrics.DeterministicRejectPrecision < .99 {
		r.Failures = append(r.Failures, fmt.Sprintf("deterministic reject precision %.4f, require >= 0.99", r.Metrics.DeterministicRejectPrecision))
	}
	if semanticRejects == 0 || r.Metrics.SemanticRejectPrecision < .97 {
		r.Failures = append(r.Failures, fmt.Sprintf("semantic reject precision %.4f, require >= 0.97", r.Metrics.SemanticRejectPrecision))
	}
	if eligible == 0 || r.Metrics.ValidAutomation < .90 {
		r.Failures = append(r.Failures, fmt.Sprintf("valid filler automation %.4f, require >= 0.90", r.Metrics.ValidAutomation))
	}
	if invalid == 0 || r.Metrics.InvalidAutomation < .95 {
		r.Failures = append(r.Failures, fmt.Sprintf("invalid input automation %.4f, require >= 0.95", r.Metrics.InvalidAutomation))
	}
	if r.Metrics.ReviewRate > .10 {
		r.Failures = append(r.Failures, fmt.Sprintf("review rate %.4f, require <= 0.10", r.Metrics.ReviewRate))
	}
	if r.Metrics.ReviewRate > 0 && r.Metrics.ReviewAnswerable < .95 {
		r.Failures = append(r.Failures, fmt.Sprintf("answerable review %.4f, require >= 0.95", r.Metrics.ReviewAnswerable))
	}
	scores := make(map[string]SliceScore, len(r.Slices))
	for _, score := range r.Slices {
		scores[score.Slice] = score
	}
	for _, gate := range gates {
		score := scores[gate.Slice]
		if score.Cases < gate.MinCases || score.Accuracy < gate.MinAccuracy {
			r.Failures = append(r.Failures, fmt.Sprintf("slice %s has %d cases at %.4f accuracy; require %d at %.4f", gate.Slice, score.Cases, score.Accuracy, gate.MinCases, gate.MinAccuracy))
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
