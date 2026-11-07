package langcode_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/langcode"
)

func TestNormalizeMapsEveryFormToTheDetectorsCode(t *testing.T) {
	for in, want := range map[string]string{
		"en": "en", "EN": "en", " eng ": "en", "en-US": "en", "pt_BR": "pt", "English": "en",
		"spa": "es", "fra": "fr", "fre": "fr", "deu": "de", "ger": "de", "zho": "zh", "chi": "zh",
		"nld": "nl", "dut": "nl", "jpn": "ja", "heb": "he", "he": "he", "iw": "he", "jav": "jw",
		"jw": "jw", "nob": "no", "nor": "no", "ind": "id", "yid": "yi", "haw": "haw", "yue": "yue",
		"wel": "cy", "cym": "cy", "fil": "tl", "tgl": "tl", "cmn": "zh",
		"": "", "none": "none", "klingon": "klingon",
	} {
		if got := langcode.Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidAcceptsLanguagesAndRejectsTheRest(t *testing.T) {
	for _, code := range []string{"en", "eng", "fre", "es-MX", "haw", "jw", "he", "Spanish"} {
		if !langcode.Valid(code) {
			t.Errorf("Valid(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"", "none", "e", "english please", "xq", "12", "und", "mul", "zxx", "mis", "qaa", "qtz"} {
		if langcode.Valid(code) {
			t.Errorf("Valid(%q) = true, want false", code)
		}
	}
}
