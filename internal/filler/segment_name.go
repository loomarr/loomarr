package filler

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	SplitNameSourceAuthored = "source-authored"
	SplitNameModelProposed  = "model-proposed"
	SplitNameFallback       = "fallback"
	SplitNameOperatorEdited = "operator-edited"

	maxSegmentNameRunes         = 80
	maxSegmentNameWords         = 10
	maxSegmentNameEvidenceRunes = 160
)

var segmentNameDescriptors = map[string]struct{}{
	"ad": {}, "advert": {}, "advertisement": {}, "bumper": {}, "commercial": {},
	"promo": {}, "psa": {}, "spot": {}, "trailer": {},
}

// groundSegmentName accepts a proposed display name only when its identifying phrase is present
// in one exact-span signal. It is deliberately narrower than semantic similarity: rejecting a
// useful name leaves a readable fallback, while accepting an invented identity silently labels a
// different clip. Punctuation and case are presentation, so the comparison normalises both.
func groundSegmentName(candidate, evidence string) (string, string, bool) {
	name := strings.Join(strings.Fields(candidate), " ")
	if name == "" || len([]rune(name)) > maxSegmentNameRunes || len(strings.Fields(name)) > maxSegmentNameWords {
		return "", "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", "", false
		}
	}

	words := normalizedNameWords(name)
	for len(words) > 0 {
		if _, descriptor := segmentNameDescriptors[words[len(words)-1]]; !descriptor {
			break
		}
		words = words[:len(words)-1]
	}
	if len(words) == 0 || genericSegmentName(strings.Join(words, " ")) {
		return "", "", false
	}
	needle := strings.Join(words, " ")
	for _, line := range strings.Split(evidence, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || !strings.Contains(strings.Join(normalizedNameWords(line), " "), needle) {
			continue
		}
		return name, boundedRunes(line, maxSegmentNameEvidenceRunes), true
	}
	return "", "", false
}

func normalizedNameWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func genericSegmentName(value string) bool {
	if _, descriptor := segmentNameDescriptors[value]; descriptor {
		return true
	}
	switch value {
	case "clip", "segment", "unknown", "untitled", "none", "n/a", "na":
		return true
	default:
		return false
	}
}

func boundedRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

// fallbackSegmentName stays useful without pretending Loomarr identified the content. It keeps a
// readable source title when one exists and otherwise names only the clip's position.
func fallbackSegmentName(source string, position int) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(source)), filepath.Ext(source))
	opaque := opaqueSegmentSourceName(base)
	base = strings.NewReplacer("_", " ", "-", " ").Replace(base)
	base = strings.Join(strings.Fields(base), " ")
	if base == "" || opaque || len([]rune(base)) > 48 || genericSegmentName(strings.ToLower(base)) {
		base = "this recording"
	}
	return "Clip " + strconv.Itoa(position) + " from " + base
}

// opaqueSegmentSourceName catches the identifiers most likely to have become a filename during
// acquisition. Showing a hash or UUID as if it identified the recording is less useful than an
// honest generic fallback. A readable title containing ordinary non-hex letters remains eligible.
func opaqueSegmentSourceName(value string) bool {
	compact := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(value))
	if len(compact) < 16 {
		return false
	}
	for _, r := range compact {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func proposedSegmentName(candidate, evidence, source string, position int) (name, origin, nameEvidence string) {
	if grounded, excerpt, ok := groundSegmentName(candidate, evidence); ok {
		return grounded, SplitNameModelProposed, excerpt
	}
	return fallbackSegmentName(source, position), SplitNameFallback, ""
}
