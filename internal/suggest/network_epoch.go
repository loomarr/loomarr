package suggest

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// These clauses identify the role of a dated reference, not a registry of
// networks, their eras, or their constituent shows. Catalog remains authority.
var networkStyleClause = regexp.MustCompile(`(?i)\b(?:like|recreate|recreating|resemble|resembling)\s+(?:(?:the|a|an)\s+)?((?:[\p{L}\p{N}&'’-]+\s+){1,6})(?:channel|network)\b(?:\s+(?:from|in|during)\s+(?:the\s+)?((?:19|20)[0-9]0s|[0-9]{2}s)\b)?`)

type networkReferenceSpan struct {
	dateStart, dateEnd int
}

func networkReferenceSpans(text string) []networkReferenceSpan {
	var spans []networkReferenceSpan
	for _, match := range networkStyleClause.FindAllStringSubmatchIndex(text, -1) {
		label := strings.Fields(strings.ToLower(strings.TrimSpace(text[match[2]:match[3]])))
		valid := true
		for _, word := range label {
			switch word {
			case "you", "me", "make", "build", "create", "want", "a", "an", "franchise":
				valid = false
			}
		}
		if valid {
			spans = append(spans, networkReferenceSpan{dateStart: match[4], dateEnd: match[5]})
		}
	}
	return spans
}

var independentDateHint = regexp.MustCompile(`(?i)\b(?:only|first|last|past|recent|latest|earliest|oldest|before|after|since|until|early|late|between|throughout|during)\b|\b[0-9]{2,4}s?\b`)
var runtimeRequestHint = regexp.MustCompile(`(?i)\b(?:runtimes?|minutes?|hours?)\b`)
var voteCountRequestHint = regexp.MustCompile(`(?i)\bvotes?\b`)
var voteAverageRequestHint = regexp.MustCompile(`(?i)\b(?:ratings?|rated|scores?|stars?)\b`)
var numericRequestHint = regexp.MustCompile(`[0-9]`)

func networkEpochHasNoPlaybackDates(intent Intent) bool {
	if networkProgrammingEpochEnd(intent) == 0 || intentRequiresDateAcknowledgement(intent) {
		return false
	}
	texts := append([]string{withoutNetworkEpochDates(intent.Description), withoutNetworkEpochDates(intent.RefineText)}, intent.MustInclude...)
	texts = append(texts, intent.MustExclude...)
	for _, text := range texts {
		if independentDateHint.MatchString(text) {
			return false
		}
	}
	return true
}

func networkScalarRequested(intent Intent, field string) bool {
	text := strings.Join(append([]string{withoutNetworkEpochDates(intent.Description), withoutNetworkEpochDates(intent.RefineText)}, intent.MustInclude...), " ")
	switch field {
	case "runtime_min", "runtime_max":
		return runtimeRequestHint.MatchString(text)
	case "vote_count_min":
		return voteCountRequestHint.MatchString(text) && numericRequestHint.MatchString(text)
	case "vote_average_min":
		return voteAverageRequestHint.MatchString(text) && numericRequestHint.MatchString(text)
	default:
		return true
	}
}

func networkStyleRequest(intent Intent) bool {
	return len(networkReferenceSpans(intent.Description)) > 0 || len(networkReferenceSpans(intent.RefineText)) > 0
}

func networkProgrammingEpochEnd(intent Intent) int {
	end := 0
	for _, text := range []string{intent.Description, intent.RefineText} {
		for _, span := range networkReferenceSpans(text) {
			if span.dateStart < 0 {
				continue
			}
			year, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(text[span.dateStart:span.dateEnd]), "s"))
			if err != nil {
				return 0
			}
			if year < 100 {
				if year <= 20 {
					year += 2000
				} else {
					year += 1900
				}
			}
			if end != 0 && end != year+9 {
				return 0
			}
			end = year + 9
		}
	}
	return end
}

func withoutNetworkEpochDates(text string) string {
	for _, span := range networkReferenceSpans(text) {
		if span.dateStart >= 0 {
			text = text[:span.dateStart] + strings.Repeat(" ", span.dateEnd-span.dateStart) + text[span.dateEnd:]
		}
	}
	return text
}

func networkEpochAnchor(intent Intent, anchor DateAnchor) bool {
	var text string
	switch anchor.Field {
	case DateAnchorDescription:
		text = intent.Description
	case DateAnchorRefineText:
		text = intent.RefineText
	default:
		return false
	}
	for _, span := range networkReferenceSpans(text) {
		if span.dateStart < 0 {
			continue
		}
		start := utf8.RuneCountInString(text[:span.dateStart])
		end := utf8.RuneCountInString(text[:span.dateEnd])
		if anchor.Start < end && anchor.End > start {
			return true
		}
	}
	return false
}
