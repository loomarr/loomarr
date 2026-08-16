//go:build eval

package eval

import (
	"reflect"
	"testing"

	"github.com/mantonx/loomarr/internal/suggest"
)

// This test is intentionally network-free. It proves that every shipped template has
// exactly one semantic case and that the case evaluates the exact object the UI submits,
// not a separately rephrased approximation.
func TestCanonicalTemplateCoverage(t *testing.T) {
	casesByTemplate := make(map[string][]suggest.Intent, len(canonicalTemplates))
	for _, evalCase := range Corpus {
		if evalCase.TemplateID != "" {
			casesByTemplate[evalCase.TemplateID] = append(casesByTemplate[evalCase.TemplateID], evalCase.Intent)
		}
	}

	if len(casesByTemplate) != len(canonicalTemplates) {
		t.Errorf("semantic template ids = %d, canonical template ids = %d", len(casesByTemplate), len(canonicalTemplates))
	}
	canonicalIDs := make(map[string]bool, len(canonicalTemplates))
	for _, template := range canonicalTemplates {
		canonicalIDs[template.ID] = true
		intents := casesByTemplate[template.ID]
		if len(intents) != 1 {
			t.Errorf("canonical template %q has %d semantic cases, want exactly 1", template.ID, len(intents))
			continue
		}
		if !reflect.DeepEqual(intents[0], template.Intent) {
			t.Errorf("canonical template %q semantic intent = %#v, want exact shipped intent %#v", template.ID, intents[0], template.Intent)
		}
	}
	for templateID := range casesByTemplate {
		if !canonicalIDs[templateID] {
			t.Errorf("semantic case names unknown canonical template %q", templateID)
		}
	}
}
