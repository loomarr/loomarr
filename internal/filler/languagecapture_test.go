package filler

import (
	"os"
	"path/filepath"
	"testing"
)

// The parser against REAL whisper-cli output, not hand-written JSON.
//
// ⚠ **Captured from the vendored binary and model in the built image** (whisper.cpp v1.9.1 +
// `ggml-tiny.bin`), run with the exact arguments `WhisperLanguage.DetectLanguage` uses, over two
// genuine clips:
//
//   - `lang_es.json` — a Spanish Coca-Cola advert from archive.org
//     (`spanishrevolution-ElmejoranunciodeCocaCola-1LE-AHD7l80`, declared `language: spanish` by
//     the source), sampled 1s–11s exactly as `LanguageSpan(29814)` asks for.
//   - `lang_en.json` — a real catalog clip from the dev drop-folder, same span rule.
//
// Hand-written fixtures test my understanding of the format; these test the format. AGENTS.md's
// "fixtures are pinned truth" rule exists because a parser written against remembered field names
// is a parser that works until it meets the tool.
//
// The transcription array is TRIMMED to two utterances — the parser only asks "was anything said",
// so the rest is bulk. Everything the parser reads is preserved verbatim.
func TestParseWhisperLanguage_AgainstRealVendoredOutput(t *testing.T) {
	for _, tc := range []struct {
		file, want, why string
	}{
		{"lang_es.json", "es", "a genuine Spanish advert must be detected as Spanish, or the gate never rejects anything"},
		{"lang_en.json", "en", "a real catalog clip must be detected as English, or the gate deletes the whole catalog"},
	} {
		raw, err := os.ReadFile(filepath.Join("..", "testkit", "fixtures", "whisper", tc.file))
		if err != nil {
			t.Fatalf("read %s: %v", tc.file, err)
		}
		got, err := parseWhisperLanguage(raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if got != tc.want {
			t.Errorf("%s parsed as %q, want %q — %s", tc.file, got, tc.want, tc.why)
		}
	}
}

// ⚠ **The `.en` blocker, pinned as a fact rather than a comment.** `params.language` is `auto` in
// both captures while `result.language` carries the answer — so a parser reading the wrong field
// would return "auto" for every clip on earth and the gate would silently never fire.
//
// This is also the evidence that vendoring a SECOND model was necessary: the shipped
// `ggml-small.en.bin` is English-only and does not identify languages at all, so it would have
// answered `en` for the Spanish capture above.
func TestParseWhisperLanguage_TheCapturesDistinguishDetectedFromRequested(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "testkit", "fixtures", "whisper", "lang_es.json"))
	if err != nil {
		t.Fatal(err)
	}
	// If the parser ever reads `params`, this returns LangUndetermined ("auto" is not a language)
	// rather than "es" — a silent no-op gate.
	if got, _ := parseWhisperLanguage(raw); got != "es" {
		t.Errorf("parsed %q; the capture's params.language is %q, so reading the wrong field "+
			"disables the gate rather than failing loudly", got, "auto")
	}
}
