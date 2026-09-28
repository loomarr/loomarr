package filler

import (
	"fmt"
	"strings"
	"unicode"
)

// NameSource records which evidence produced a clip's derived display name (#1452). It is written
// beside the name in the sidecar, so a derived name is recognisably Loomarr's and can be improved
// when better evidence arrives, while a name the source or a person gave never is.
type NameSource string

const (
	NameFromBrand      NameSource = "brand"
	NameFromTranscript NameSource = "transcript"
	NameFromScreenText NameSource = "on_screen_text"
	NameFromCategory   NameSource = "category"
	NameFromEra        NameSource = "era"
	NameUntitled       NameSource = "untitled"
)

// excerptWords bounds a quoted excerpt: long enough to identify a spot, short enough for a guide cell.
const excerptWords = 6

// GroundedName names a clip that has no source title from grounded evidence, most specific first:
// the grounded brand, then the opening words of its transcript, then the on-screen text a vision
// pass read, then category or era, and only then the neutral "Untitled <kind>".
//
// ⚠ Quotes, never paraphrase. A transcript or on-screen excerpt appears verbatim inside quotation
// marks, so the name states only what the clip itself says (§10 grounding). Summarising it would
// be the model inventing a title. No step needs the LLM or vision to be on: evidence that was
// never gathered simply leaves its rung empty.
func GroundedName(c Clip) (string, NameSource) {
	withEra := func(base string) string {
		if c.Era > 0 {
			return fmt.Sprintf("%s — %d", base, c.Era)
		}
		return base
	}
	if brand := strings.TrimSpace(c.Brand); brand != "" {
		return withEra(brand), NameFromBrand
	}
	if quote := transcriptExcerpt(c.Transcript); quote != "" {
		return withEra(quote), NameFromTranscript
	}
	if quote := screenTextExcerpt(c.VisibleText); quote != "" {
		return withEra(quote), NameFromScreenText
	}
	if category := strings.ReplaceAll(c.Category, "_", " "); category != "" {
		return withEra(strings.ToUpper(category[:1]) + category[1:] + " commercial"), NameFromCategory
	}
	kind := strings.TrimSpace(strings.ReplaceAll(string(c.Kind), "_", " "))
	if c.Era > 0 && kind != "" {
		return fmt.Sprintf("%d %s", c.Era, kind), NameFromEra
	}
	if kind == "" {
		kind = "filler clip"
	}
	return "Untitled " + kind, NameUntitled
}

// transcriptExcerpt quotes the opening of what the clip says: its first sentence, cut at a word
// boundary. Whisper's cue markers ("[Music]", "(applause)", "♪") are not speech and are dropped.
// A wordless clip, or an opening with fewer than two words, yields nothing.
func transcriptExcerpt(transcript string) string {
	if transcript == TranscriptNone {
		return ""
	}
	text := stripCues(transcript)
	if end := strings.IndexAny(text, ".!?"); end >= 0 {
		text = text[:end]
	}
	return quoteExcerpt(strings.Fields(text), true)
}

// screenTextExcerpt quotes the first line of on-screen text, as read (logos are often all caps).
func screenTextExcerpt(visible string) string {
	for _, line := range strings.Split(visible, "\n") {
		if words := strings.Fields(line); len(words) > 0 {
			return quoteExcerpt(words, false)
		}
	}
	return ""
}

func quoteExcerpt(words []string, sentenceCase bool) string {
	letters := 0
	for _, w := range words {
		for _, r := range w {
			if unicode.IsLetter(r) {
				letters++
			}
		}
	}
	if len(words) < 2 || letters < 4 {
		return ""
	}
	cut := len(words) > excerptWords
	if cut {
		words = words[:excerptWords]
	}
	text := strings.TrimRight(strings.Join(words, " "), ",;:-–—")
	if sentenceCase {
		r := []rune(text)
		r[0] = unicode.ToUpper(r[0])
		text = string(r)
	}
	if cut {
		text += "…"
	}
	return "“" + text + "”"
}

// stripCues removes bracketed and parenthesised cues and music notes from a transcript.
func stripCues(text string) string {
	var b strings.Builder
	depth := 0
	for _, r := range text {
		switch {
		case r == '[' || r == '(':
			depth++
		case (r == ']' || r == ')') && depth > 0:
			depth--
		case depth > 0, r == '♪', r == '♫':
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
