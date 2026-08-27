package fillereval

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	CertificationMinDevelopment = 300
	CertificationMinHoldout     = 1126
)

// certificationContract is the one maintained definition of a statistically
// useful, independently sampled holdout. Preparation owns the split shape;
// final manifest validation additionally owns the reviewed label composition.
type certificationContract struct {
	MinDevelopment           int
	MinHoldout               int
	MinEligible              int
	MinDeterministicInvalid  int
	MinSemanticInvalid       int
	MinAmbiguous             int
	MaxCreatorSharePerRole   float64
	MaxSourceShareOfEligible float64
	MinEligibleRoles         map[string]int
}

var currentCertificationContract = certificationContract{
	MinDevelopment:           CertificationMinDevelopment,
	MinHoldout:               CertificationMinHoldout,
	MinEligible:              446,
	MinDeterministicInvalid:  446,
	MinSemanticInvalid:       147,
	MinAmbiguous:             87,
	MaxCreatorSharePerRole:   .10,
	MaxSourceShareOfEligible: .25,
	MinEligibleRoles: map[string]int{
		"commercial": 82,
		"promo":      82,
		"bumper":     59,
		"station_id": 59,
		"trailer":    82,
		"psa":        82,
	},
}

// ValidateCertificationContract applies the release-scale sampling and
// diversity contract. The label lock and scorer are the certification
// authorities; provider capture remains non-authorizing and may be exercised
// on smaller, structurally valid development fixtures under its spend limits.
func ValidateCertificationContract(manifest Manifest) []string {
	failures := ValidateCertificationDraft(manifest)
	return append(failures, ValidateCertificationComposition(manifest)...)
}

// ValidateCertificationDraft checks what can be proven before semantic labels
// exist. A holdout case is one independent source/similarity unit; repeated
// derivatives or segments therefore cannot inflate a denominator.
func ValidateCertificationDraft(manifest Manifest) []string {
	contract := currentCertificationContract
	counts := map[Split]int{}
	holdoutClusters := map[string]string{}
	holdoutCampaigns := map[string]string{}
	holdoutFamilies := map[string]string{}
	var failures []string
	for _, c := range manifest.Cases {
		counts[c.Split]++
		if c.Split != SplitHoldout || strings.TrimSpace(c.Cluster) == "" {
			continue
		}
		if prior := holdoutClusters[c.Cluster]; prior != "" {
			failures = append(failures, fmt.Sprintf("%s: holdout similarity cluster %q is already represented by %s", c.ID, c.Cluster, prior))
		} else {
			holdoutClusters[c.Cluster] = c.ID
		}
		campaign := strings.TrimSpace(c.Provenance.Campaign)
		if prior := holdoutCampaigns[campaign]; campaign != "" && prior != "" {
			failures = append(failures, fmt.Sprintf("%s: holdout campaign %q is already represented by %s", c.ID, campaign, prior))
		} else if campaign != "" {
			holdoutCampaigns[campaign] = c.ID
		}
		family := strings.TrimSpace(c.Provenance.SourceFamily)
		if prior := holdoutFamilies[family]; family != "" && prior != "" {
			failures = append(failures, fmt.Sprintf("%s: holdout source family %q is already represented by %s", c.ID, family, prior))
		} else if family != "" {
			holdoutFamilies[family] = c.ID
		}
	}
	if counts[SplitDevelopment] < contract.MinDevelopment {
		failures = append(failures, fmt.Sprintf("certification development split has %d cases; require at least %d", counts[SplitDevelopment], contract.MinDevelopment))
	}
	if counts[SplitHoldout] < contract.MinHoldout {
		failures = append(failures, fmt.Sprintf("certification holdout has %d independent cases; require at least %d", counts[SplitHoldout], contract.MinHoldout))
	}
	return failures
}

// ValidateCertificationComposition checks the independently reviewed answer
// key. Counts are intentionally tied to the Wilson gates used by Score: this
// cohort tolerates one observed error in each precision/answerability slice.
func ValidateCertificationComposition(manifest Manifest) []string {
	contract := currentCertificationContract
	var failures []string
	truthCounts := map[Truth]int{}
	invalidCounts := map[RejectClass]int{}
	roleCounts := map[string]int{}
	creatorByRole := map[string]map[string]int{}
	sourceCounts := map[string]int{}
	for _, c := range manifest.Cases {
		if c.Split != SplitHoldout {
			continue
		}
		truthCounts[c.Truth]++
		if c.Truth == TruthInvalid {
			invalidCounts[c.RejectClass]++
		}
		if c.Truth != TruthEligible {
			continue
		}
		role := strings.TrimSpace(c.ContentRole)
		creator := strings.TrimSpace(c.Provenance.Creator)
		roleCounts[role]++
		incrementNested(creatorByRole, role, creator)
		sourceCounts[strings.TrimSpace(c.Source)]++
		if creator == "" {
			failures = append(failures, c.ID+": eligible holdout case requires creator provenance")
		}
	}
	if truthCounts[TruthEligible] < contract.MinEligible {
		failures = append(failures, fmt.Sprintf("eligible holdout has %d cases; require at least %d", truthCounts[TruthEligible], contract.MinEligible))
	}
	if invalidCounts[RejectDeterministic] < contract.MinDeterministicInvalid {
		failures = append(failures, fmt.Sprintf("deterministic-invalid holdout has %d cases; require at least %d", invalidCounts[RejectDeterministic], contract.MinDeterministicInvalid))
	}
	if invalidCounts[RejectSemantic] < contract.MinSemanticInvalid {
		failures = append(failures, fmt.Sprintf("semantic-invalid holdout has %d cases; require at least %d", invalidCounts[RejectSemantic], contract.MinSemanticInvalid))
	}
	if truthCounts[TruthAmbiguous] < contract.MinAmbiguous {
		failures = append(failures, fmt.Sprintf("ambiguous holdout has %d cases; require at least %d", truthCounts[TruthAmbiguous], contract.MinAmbiguous))
	}
	for _, role := range slices.Sorted(maps.Keys(contract.MinEligibleRoles)) {
		minimum := contract.MinEligibleRoles[role]
		if roleCounts[role] < minimum {
			failures = append(failures, fmt.Sprintf("eligible holdout role %s has %d cases; require at least %d", role, roleCounts[role], minimum))
		}
	}
	for _, role := range slices.Sorted(maps.Keys(roleCounts)) {
		total := roleCounts[role]
		failures = append(failures, capFailures("creator", role, total, creatorByRole[role], contract.MaxCreatorSharePerRole)...)
	}
	eligible := truthCounts[TruthEligible]
	for _, source := range slices.Sorted(maps.Keys(sourceCounts)) {
		count := sourceCounts[source]
		if float64(count) > float64(eligible)*contract.MaxSourceShareOfEligible {
			failures = append(failures, fmt.Sprintf("eligible source %q supplies %d/%d cases; maximum share is %.0f%%", source, count, eligible, 100*contract.MaxSourceShareOfEligible))
		}
	}
	return failures
}

func incrementNested(counts map[string]map[string]int, outer, inner string) {
	if counts[outer] == nil {
		counts[outer] = map[string]int{}
	}
	counts[outer][inner]++
}

func capFailures(kind, role string, total int, counts map[string]int, share float64) []string {
	var failures []string
	for _, value := range slices.Sorted(maps.Keys(counts)) {
		count := counts[value]
		if value != "" && float64(count) > float64(total)*share {
			failures = append(failures, fmt.Sprintf("eligible role %s %s %q supplies %d/%d cases; maximum share is %.0f%%", role, kind, value, count, total, 100*share))
		}
	}
	return failures
}
