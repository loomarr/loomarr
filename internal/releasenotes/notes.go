// Package releasenotes categorizes GitHub-generated release notes without allowing
// a language model to invent release content.
package releasenotes

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var pullRequestLine = regexp.MustCompile(`^\* .+https://github\.com/[^/]+/[^/]+/pull/([0-9]+)\s*$`)

// Change is one exact pull-request bullet emitted by GitHub.
type Change struct {
	Number int
	Title  string
	Line   string
}

// Document is GitHub's generated change list plus its mechanically generated footer.
type Document struct {
	Changes []Change
	Footer  string
}

// Parse accepts the body returned by GitHub's releases/generate-notes endpoint.
func Parse(body string) (Document, error) {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(body, "\r\n", "\n")))
	var doc Document
	inChanges := false
	seenHeading := false
	seen := make(map[int]struct{})
	var footer []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "## What's Changed" {
			if seenHeading {
				return Document{}, errors.New("release notes contain more than one What's Changed section")
			}
			seenHeading = true
			inChanges = true
			continue
		}
		if inChanges && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "**Full Changelog**")) {
			inChanges = false
		}
		if !inChanges {
			if seenHeading {
				footer = append(footer, line)
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := pullRequestLine.FindStringSubmatch(line)
		if len(matches) != 2 {
			return Document{}, fmt.Errorf("unrecognized change line %q", line)
		}
		number, err := strconv.Atoi(matches[1])
		if err != nil {
			return Document{}, fmt.Errorf("parse pull request number: %w", err)
		}
		if _, exists := seen[number]; exists {
			return Document{}, fmt.Errorf("pull request #%d appears more than once", number)
		}
		seen[number] = struct{}{}
		doc.Changes = append(doc.Changes, Change{Number: number, Title: titleFromLine(line), Line: line})
	}
	if err := scanner.Err(); err != nil {
		return Document{}, fmt.Errorf("scan release notes: %w", err)
	}
	if !seenHeading {
		return Document{}, errors.New("release notes do not contain a What's Changed section")
	}
	doc.Footer = strings.TrimSpace(strings.Join(footer, "\n"))
	return doc, nil
}

func titleFromLine(line string) string {
	trimmed := strings.TrimPrefix(line, "* ")
	if index := strings.LastIndex(trimmed, " by @"); index >= 0 {
		return strings.TrimSpace(trimmed[:index])
	}
	if index := strings.LastIndex(trimmed, " in https://github.com/"); index >= 0 {
		return strings.TrimSpace(trimmed[:index])
	}
	return trimmed
}

// Classification is the only shape accepted from the language model.
type Classification struct {
	NewFeatures   []int `json:"new_features"`
	Improvements  []int `json:"improvements"`
	BugFixes      []int `json:"bug_fixes"`
	SecurityFixes []int `json:"security_fixes"`
	Documentation []int `json:"documentation"`
	Dependencies  []int `json:"dependencies"`
	Maintenance   []int `json:"maintenance"`
}

var classificationFields = []string{
	"new_features",
	"improvements",
	"bug_fixes",
	"security_fixes",
	"documentation",
	"dependencies",
	"maintenance",
}

type category struct {
	Title   string
	Numbers []int
}

func (c Classification) categories() []category {
	return []category{
		{Title: "🆕 New Features", Numbers: c.NewFeatures},
		{Title: "✨ Improvements", Numbers: c.Improvements},
		{Title: "🐞 Bug Fixes", Numbers: c.BugFixes},
		{Title: "🔐 Security Fixes", Numbers: c.SecurityFixes},
		{Title: "📚 Documentation", Numbers: c.Documentation},
		{Title: "📦 Dependencies", Numbers: c.Dependencies},
		{Title: "🧰 Maintenance", Numbers: c.Maintenance},
	}
}

// DecodeClassification rejects extra fields before validating membership.
func DecodeClassification(data []byte, doc Document) (Classification, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Classification{}, fmt.Errorf("decode classification: %w", err)
	}
	if len(fields) != len(classificationFields) {
		return Classification{}, errors.New("classification must contain exactly the required category fields")
	}
	for _, name := range classificationFields {
		value, exists := fields[name]
		if !exists || string(value) == "null" {
			return Classification{}, fmt.Errorf("classification field %s must be an array", name)
		}
	}
	var classification Classification
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&classification); err != nil {
		return Classification{}, fmt.Errorf("decode classification: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Classification{}, errors.New("classification contains trailing JSON")
	}
	if err := Validate(doc, classification); err != nil {
		return Classification{}, err
	}
	return classification, nil
}

// Validate requires every real PR exactly once and rejects invented PR numbers.
func Validate(doc Document, classification Classification) error {
	want := make(map[int]struct{}, len(doc.Changes))
	for _, change := range doc.Changes {
		want[change.Number] = struct{}{}
	}
	got := make(map[int]string, len(doc.Changes))
	for _, group := range classification.categories() {
		for _, number := range group.Numbers {
			if _, exists := want[number]; !exists {
				return fmt.Errorf("classification invented pull request #%d", number)
			}
			if prior, exists := got[number]; exists {
				return fmt.Errorf("pull request #%d appears in both %s and %s", number, prior, group.Title)
			}
			got[number] = group.Title
		}
	}
	var missing []int
	for number := range want {
		if _, exists := got[number]; !exists {
			missing = append(missing, number)
		}
	}
	if len(missing) > 0 {
		sort.Ints(missing)
		return fmt.Errorf("classification omitted pull requests %v", missing)
	}
	return nil
}

// Render uses only GitHub's original bullets; the model controls section membership only.
func Render(doc Document, classification Classification) (string, error) {
	if err := Validate(doc, classification); err != nil {
		return "", err
	}
	byNumber := make(map[int]string, len(doc.Changes))
	for _, change := range doc.Changes {
		byNumber[change.Number] = change.Line
	}
	var out strings.Builder
	for _, group := range classification.categories() {
		if len(group.Numbers) == 0 {
			continue
		}
		fmt.Fprintf(&out, "## %s\n\n", group.Title)
		selected := make(map[int]struct{}, len(group.Numbers))
		for _, number := range group.Numbers {
			selected[number] = struct{}{}
		}
		for _, change := range doc.Changes {
			if _, exists := selected[change.Number]; exists {
				fmt.Fprintln(&out, byNumber[change.Number])
			}
		}
		out.WriteByte('\n')
	}
	if doc.Footer != "" {
		out.WriteString(doc.Footer)
		out.WriteByte('\n')
	}
	return out.String(), nil
}
