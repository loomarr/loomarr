package fillereval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
)

// Labels is the independently reviewed semantic truth for one corpus case.
// It deliberately excludes media identity, provenance, and split assignment.
type Labels struct {
	Truth          Truth               `json:"truth"`
	RejectClass    RejectClass         `json:"rejectClass,omitempty"`
	ContentRole    string              `json:"contentRole"`
	Taxonomy       map[string][]string `json:"taxonomy,omitempty"`
	PolicyFlags    []string            `json:"policyFlags,omitempty"`
	Slices         []string            `json:"slices"`
	Evidence       []Evidence          `json:"evidence"`
	ReviewQuestion string              `json:"reviewQuestion,omitempty"`
}

func LabelsFromCase(c Case) Labels {
	return NormalizeLabels(Labels{
		Truth: c.Truth, RejectClass: c.RejectClass, ContentRole: c.ContentRole,
		Taxonomy: cloneTaxonomy(c.Taxonomy), PolicyFlags: slices.Clone(c.PolicyFlags),
		Slices: slices.Clone(c.Slices), Evidence: slices.Clone(c.Evidence),
		ReviewQuestion: c.ReviewQuestion,
	})
}

func ApplyLabels(c *Case, labels Labels) {
	labels = NormalizeLabels(labels)
	c.Truth = labels.Truth
	c.RejectClass = labels.RejectClass
	c.ContentRole = labels.ContentRole
	c.Taxonomy = cloneTaxonomy(labels.Taxonomy)
	c.PolicyFlags = slices.Clone(labels.PolicyFlags)
	c.Slices = slices.Clone(labels.Slices)
	c.Evidence = slices.Clone(labels.Evidence)
	c.ReviewQuestion = labels.ReviewQuestion
}

func LabelsSHA256(labels Labels) string {
	labels = NormalizeLabels(labels)
	data, err := json.Marshal(labels)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// NormalizeLabels makes set-like label order irrelevant while retaining exact
// human text and evidence locations. Independent reviewers should not disagree
// merely because they selected the same tags in a different order.
func NormalizeLabels(labels Labels) Labels {
	labels.Taxonomy = cloneTaxonomy(labels.Taxonomy)
	for axis, values := range labels.Taxonomy {
		labels.Taxonomy[axis] = sortedUnique(values)
	}
	if len(labels.Taxonomy) == 0 {
		labels.Taxonomy = nil
	}
	labels.PolicyFlags = sortedUnique(labels.PolicyFlags)
	labels.Slices = sortedUnique(labels.Slices)
	labels.Evidence = slices.Clone(labels.Evidence)
	sort.Slice(labels.Evidence, func(i, j int) bool {
		a, b := labels.Evidence[i], labels.Evidence[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Claim != b.Claim {
			return a.Claim < b.Claim
		}
		if a.AtMS != b.AtMS {
			return a.AtMS < b.AtMS
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.Provenance < b.Provenance
	})
	return labels
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
	if input == nil {
		return nil
	}
	output := make(map[string][]string, len(input))
	for axis, values := range input {
		output[axis] = slices.Clone(values)
	}
	return output
}
