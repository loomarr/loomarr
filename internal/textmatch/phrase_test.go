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

func TestContainsPhraseOutsidePreservesOccurrenceBoundaries(t *testing.T) {
	for _, tt := range []struct {
		text, phrase, excluded string
		want                   bool
	}{
		{"Santa visits Santa’s Little Helper", "santa", "Santa's Little Helper", true},
		{"Santa's Little Helper Santa's Little Helper", "santa", "Santa's Little Helper", false},
		{"Santa's Little Helpers", "santa", "Santa's Little Helper", true},
		{"A dog holiday", "a holiday", "dog", false},
		{"ᴬ holiday", "a holiday", "", true},
		{"Café Noël", "Café Noël", "", true},
		{"A holiday", "", "dog", false},
	} {
		if got := textmatch.ContainsPhraseOutside(tt.text, tt.phrase, tt.excluded); got != tt.want {
			t.Errorf("ContainsPhraseOutside(%q,%q,%q) = %v", tt.text, tt.phrase, tt.excluded, got)
		}
	}
}
