package filler

import "testing"

// The two parsers, tested in-package because both are private. They are where a real backend's
// real answer lands, and both have a failure mode that silently disables or over-fires the gate.

// ⚠ `result.language` is the DETECTED language; `params.language` is what was REQUESTED (usually
// "auto"). Reading the wrong one gives you your own input back — a detector that always agrees
// with itself. The fixture carries both, spelled differently, so a swap fails loudly.
func TestParseWhisperLanguage_ReadsTheDetectedFieldNotTheRequestedOne(t *testing.T) {
	raw := []byte(`{
	  "params": {"model": "ggml-tiny.bin", "language": "auto", "translate": false},
	  "result": {"language": "es"},
	  "transcription": [{"offsets":{"from":0,"to":2000},"text":" Compre ahora."}]
	}`)

	got, err := parseWhisperLanguage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "es" {
		t.Errorf("language = %q, want es — reading params.language instead would give %q, which is "+
			"the request echoed back", got, "auto")
	}
}

// ⚠ **No transcribed text means NO SPEECH**, which always keeps the clip. Without this branch a
// silent visual spot inherits whatever the model guessed from noise, and a confident-sounding
// wrong answer is exactly what rejects a clip that should have been kept.
func TestParseWhisperLanguage_NoTranscribedTextIsNoneNotAGuess(t *testing.T) {
	// The shape whisper returns for music or silence: it still names a language, but transcribed
	// nothing.
	raw := []byte(`{"result":{"language":"en"},"transcription":[]}`)
	got, err := parseWhisperLanguage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != LangNone {
		t.Errorf("language = %q, want %q — a wordless clip must not inherit a guessed language",
			got, LangNone)
	}

	// Whitespace-only utterances are the same case, and are what whisper actually emits.
	raw = []byte(`{"result":{"language":"en"},"transcription":[{"offsets":{"from":0,"to":900},"text":"  "}]}`)
	if got, _ = parseWhisperLanguage(raw); got != LangNone {
		t.Errorf("language = %q for whitespace-only text, want %q", got, LangNone)
	}
}

// Unparseable output is UNDETERMINED, never a language. The gate then keeps the clip and retries.
func TestParseWhisperLanguage_BadOutputIsUndetermined(t *testing.T) {
	if got, err := parseWhisperLanguage([]byte("not json")); err == nil || got != LangUndetermined {
		t.Errorf("got (%q, %v), want (%q, an error)", got, err, LangUndetermined)
	}
	// Valid JSON, no detected language: still undetermined rather than "".
	got, err := parseWhisperLanguage([]byte(`{"result":{"language":"auto"},"transcription":[{"offsets":{"from":0,"to":1},"text":"hi"}]}`))
	if err != nil || got != LangUndetermined {
		t.Errorf("got (%q, %v), want (%q, nil)", got, err, LangUndetermined)
	}
}

// The hosted parser. ⚠ Deliberately STRICT about length: a model that ignores "one word" and
// replies with a sentence must land on undetermined (keep the clip), never on a language taken
// from the first token of prose.
func TestParseHostedLanguage(t *testing.T) {
	for _, tc := range []struct{ answer, want string }{
		// What a well-behaved model returns.
		{"en", "en"},
		{"es", "es"},
		{"EN", "en"},
		{" fr ", "fr"},
		{"en.", "en"},
		{`"de"`, "de"},

		// The licensed non-answer, and its neighbours.
		{"none", LangNone},
		{"silence", LangNone},
		{"music", LangNone},

		// ⚠ THE case strictness exists for. "The audio is in Spanish" starts with "the" — a
		// lenient parser taking the first word would reject an English clip on it.
		{"The audio is in Spanish", LangUndetermined},
		{"I cannot determine the language", LangUndetermined},
		{"", LangUndetermined},
		// A single word that is not a language code survives normalisation unrecognised, and a
		// non-two-letter result is not an answer.
		{"unknown", LangUndetermined},
	} {
		if got := parseHostedLanguage(tc.answer); got != tc.want {
			t.Errorf("parseHostedLanguage(%q) = %q, want %q", tc.answer, got, tc.want)
		}
	}
}

// A prose answer that HAPPENS to name the target language must still not be read as a code — the
// point is that we could not parse it, not that we found something.
func TestParseHostedLanguage_ProseIsNeverACode(t *testing.T) {
	for _, answer := range []string{"English", "english"} {
		// One word, and a name normalisation knows: this IS an answer.
		if got := parseHostedLanguage(answer); got != "en" {
			t.Errorf("parseHostedLanguage(%q) = %q, want en", answer, got)
		}
	}
	// But the moment it becomes a sentence it is not.
	if got := parseHostedLanguage("It is English"); got != LangUndetermined {
		t.Errorf("parseHostedLanguage(%q) = %q, want undetermined", "It is English", got)
	}
}
