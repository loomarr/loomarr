package suggest

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// This recognizes the role of explicit editorial wording, never a historical
// roster or a guessed epoch boundary. Separate date expressions remain intact.
var editorialBlockDate = regexp.MustCompile(`(?i)\bas\s+(?:it|they)\s+(?:felt|was|were)\s+(?:in|during|from)\s+(?:the\s+)?((?:19|20)[0-9]0s|[0-9]{2}s)\b`)

func withoutEditorialBlockDates(intent Intent, text string) string {
	if namedBlockLabel(intent) == "" {
		return text
	}
	for _, match := range editorialBlockDate.FindAllStringSubmatchIndex(text, -1) {
		text = text[:match[2]] + strings.Repeat(" ", match[3]-match[2]) + text[match[3]:]
	}
	return text
}

func editorialBlockAnchor(intent Intent, anchor DateAnchor) bool {
	if namedBlockLabel(intent) == "" {
		return false
	}
	var text string
	switch anchor.Field {
	case "description":
		text = intent.Description
	case "refineText":
		text = intent.RefineText
	default:
		return false
	}
	for _, match := range editorialBlockDate.FindAllStringSubmatchIndex(text, -1) {
		start, end := utf8.RuneCountInString(text[:match[2]]), utf8.RuneCountInString(text[:match[3]])
		if anchor.Start < end && anchor.End > start {
			return true
		}
	}
	return false
}
