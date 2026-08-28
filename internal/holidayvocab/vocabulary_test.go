package holidayvocab_test

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

func TestDefinitionsPinBuiltInIdentityAndAliasCoverage(t *testing.T) {
	want := []holidayvocab.Definition{
		{ID: "halloween", Label: "Halloween"},
		{ID: "thanksgiving", Label: "Thanksgiving"},
		{ID: "christmas", Label: "Christmas"},
		{ID: "newyear", Label: "New Year"},
		{ID: "valentines", Label: "Valentine's Day"},
	}
	if got := holidayvocab.Definitions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("built-in holiday vocabulary = %+v, want %+v", got, want)
	}
	for _, definition := range want {
		if aliases := holidayvocab.EvidenceAliases(definition.ID); len(aliases) == 0 {
			t.Fatalf("holiday %q has no scheduler evidence aliases", definition.ID)
		}
	}
}

func TestIntentAliasesCoverSchedulerKnownSantaAndNYE(t *testing.T) {
	for _, tt := range []struct {
		text string
		want string
	}{
		{text: "Santa specials", want: "christmas"},
		{text: "An NYE marathon", want: "newyear"},
	} {
		got := holidayvocab.MatchIntent(tt.text)
		if len(got) != 1 || got[0] != tt.want {
			t.Fatalf("MatchIntent(%q) = %v, want [%s]", tt.text, got, tt.want)
		}
	}
}
