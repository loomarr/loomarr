package textmatch_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/textmatch"
)

func TestContainsPhraseMatchesWordsAcrossPunctuationWithoutSubstringLeakage(t *testing.T) {
	if !textmatch.ContainsPhrase("A start-to-finish marathon", "start to finish") {
		t.Fatal("phrase separated by punctuation did not match")
	}
	if textmatch.ContainsPhrase("Classical concerts", "classic") {
		t.Fatal("cue matched inside another word")
	}
}

func TestContainsPhraseMatchesCanonicalUnicodeEquivalents(t *testing.T) {
	if !textmatch.ContainsPhrase("Café Noël", "Café Noël") {
		t.Fatal("canonically equivalent composed and decomposed phrases did not match")
	}
}

func TestContainsPhraseUsesUnicodeCaseFolding(t *testing.T) {
	if !textmatch.ContainsPhrase("Die Straße", "STRASSE") {
		t.Fatal("non-ASCII case-fold equivalent phrase did not match")
	}
}

func TestContainsPhraseFoldsCompatibilityDecomposition(t *testing.T) {
	for _, text := range []string{"Ⓐ holiday", "ᴬ holiday"} {
		if !textmatch.ContainsPhrase(text, "a holiday") {
			t.Fatalf("compatibility decomposition in %q introduced an unmatched capital letter", text)
		}
	}
}
