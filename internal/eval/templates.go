//go:build eval

package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mantonx/loomarr/internal/suggest"
)

// canonicalTemplate is the Go adapter for the product data shipped by @loomarr/core.
// Semantic expectations deliberately do not live here: they are evaluation policy,
// while this file is the exact user-facing request the product promises to submit.
type canonicalTemplate struct {
	ID     string         `json:"id"`
	Label  string         `json:"label"`
	Intent suggest.Intent `json:"intent"`
}

var canonicalTemplates = mustLoadCanonicalTemplates()

func mustLoadCanonicalTemplates() []canonicalTemplate {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("eval: locate canonical channel-template loader")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "web", "packages", "core", "src", "templates", "channel-templates.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("eval: read canonical channel templates: %v", err))
	}

	var templates []canonicalTemplate
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&templates); err != nil {
		panic(fmt.Sprintf("eval: decode canonical channel templates: %v", err))
	}
	if err := requireJSONEOF(dec); err != nil {
		panic(fmt.Sprintf("eval: decode canonical channel templates: %v", err))
	}
	if len(templates) == 0 {
		panic("eval: canonical channel templates are empty")
	}

	seen := make(map[string]struct{}, len(templates))
	for i, template := range templates {
		if strings.TrimSpace(template.ID) == "" {
			panic(fmt.Sprintf("eval: canonical channel template %d has no id", i))
		}
		if _, exists := seen[template.ID]; exists {
			panic(fmt.Sprintf("eval: duplicate canonical channel template id %q", template.ID))
		}
		seen[template.ID] = struct{}{}
		if strings.TrimSpace(template.Label) == "" {
			panic(fmt.Sprintf("eval: canonical channel template %q has no label", template.ID))
		}
		if strings.TrimSpace(template.Intent.Description) == "" {
			panic(fmt.Sprintf("eval: canonical channel template %q has no intent description", template.ID))
		}
	}
	return templates
}

func requireJSONEOF(dec *json.Decoder) error {
	var trailing any
	err := dec.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func canonicalIntent(id string) suggest.Intent {
	for _, template := range canonicalTemplates {
		if template.ID == id {
			return template.Intent
		}
	}
	panic(fmt.Sprintf("eval: no canonical channel template %q", id))
}
