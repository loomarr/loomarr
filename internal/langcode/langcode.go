// Package langcode maps the ways a language gets written down onto one code per language, so a
// channel's commercial language, a setting and a detector's answer compare equal (#2030 decision
// 12).
//
// The canonical codes are the speech detector's: ISO 639-1 where one exists, else ISO 639-3, with
// whisper's own spellings (`he`, `jw`, `no`). Those are what the store's `language` column holds,
// so every other spelling is mapped onto them rather than the reverse.
package langcode

import (
	"strings"

	"golang.org/x/text/language"
)

// bibliographic maps the ISO 639-2/B codes to their 639-2/T twins. x/text knows only the T forms,
// and the B forms are what older media metadata and operators raised on it write (`fre`, `ger`).
var bibliographic = map[string]string{
	"alb": "sqi", "arm": "hye", "baq": "eus", "bur": "mya", "chi": "zho", "cze": "ces",
	"dut": "nld", "fre": "fra", "geo": "kat", "ger": "deu", "gre": "ell", "ice": "isl",
	"mac": "mkd", "mao": "mri", "may": "msa", "per": "fas", "rum": "ron", "slo": "slk",
	"tib": "bod", "wel": "cym",
}

// detectorSpelling maps codes x/text prefers onto the detector's spelling of the same language.
// Filipino and Mandarin are standardised forms the detector reports as Tagalog and Chinese.
var detectorSpelling = map[string]string{
	"iw": "he", "in": "id", "ji": "yi", "jv": "jw", "nb": "no", "fil": "tl", "cmn": "zh",
}

// special codes name no one language (undetermined, multiple, no linguistic content, uncoded),
// and qaa–qtz are reserved for local use. A channel set to one would air only wordless clips.
var special = map[string]bool{"und": true, "mul": true, "zxx": true, "mis": true}

// names catches a model answering in prose rather than with a code. Deliberately short: a safety
// net for the common case, not a language database. An unrecognised name stays as it is and
// simply matches nothing, which is the honest outcome.
var names = map[string]string{
	"english": "en",
	"spanish": "es", "espanol": "es", "español": "es",
	"french": "fr", "francais": "fr", "français": "fr",
	"german": "de", "deutsch": "de",
}

// Normalize reduces a code to the detector's base tag: `en-US`, `EN`, `eng `, `English` all become
// `en`; `fre` and `fra` become `fr`; `heb` becomes `he`. A value it cannot place is returned
// lower-cased and trimmed, and "" stays "".
//
// ⚠ Comparing raw codes is the obvious implementation and it is wrong in a way that only shows up
// on real data: whisper answers `en`, a hosted model may answer `en-US` or `English`, and an
// operator may type `EN` or `eng`. Any of those mismatching the others empties a channel's breaks.
func Normalize(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if c == "" {
		return ""
	}
	// Strip a region or script subtag: en-US, zh-Hans, pt_BR.
	if i := strings.IndexAny(c, "-_"); i > 0 {
		c = c[:i]
	}
	if n, ok := names[c]; ok {
		return n
	}
	if t, ok := bibliographic[c]; ok {
		c = t
	}
	if len(c) == 2 || len(c) == 3 {
		if base, err := language.ParseBase(c); err == nil {
			c = base.String()
		}
	}
	if d, ok := detectorSpelling[c]; ok {
		return d
	}
	return c
}

// Valid reports whether code names one language Normalize can place: an ISO 639 code in any of its
// forms, or one of the names above.
func Valid(code string) bool {
	c := Normalize(code)
	if len(c) < 2 || len(c) > 3 || special[c] || (len(c) == 3 && c >= "qaa" && c <= "qtz") {
		return false
	}
	_, err := language.ParseBase(c)
	return err == nil
}
