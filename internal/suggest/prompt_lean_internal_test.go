package suggest

import (
	"strings"
	"testing"
)

// The output template is what the model imitates token by token, and generation is the slowest
// step of adding a channel (#1487). A per-pick name/mediaType or a restated dateMeaning is output
// the server re-derives from the surfaced candidate and the accepted tool call.
func TestSystemPromptFinalTemplateIsLean(t *testing.T) {
	i := strings.Index(systemPrompt, "When finished, reply with ONLY this JSON")
	if i < 0 {
		t.Fatal("final template marker missing")
	}
	template := systemPrompt[i:]
	for _, wasted := range []string{`"mediaType":"movie|series"`, `"name":"<string>"`, `"dateMeaning":{`} {
		if strings.Contains(template, wasted) {
			t.Errorf("final template still asks for %s", wasted)
		}
	}
	if !strings.Contains(systemPrompt, "no catalog_search call") {
		t.Error("the prompt must still say a final with no tool call states dateMeaning itself")
	}
}

func TestFinalizationNoteTellsModelToOmitAcceptedDateMeaning(t *testing.T) {
	meaning, err := validateIntentDateMeaning(Intent{Description: "action films"}, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := finalizationMessages(nil, &meaning)
	if err != nil {
		t.Fatal(err)
	}
	note := msgs[len(msgs)-1].Content
	if !strings.Contains(note, "omit dateMeaning") || strings.Contains(note, "Copy this accepted dateMeaning") {
		t.Errorf("note still asks the model to restate dateMeaning: %s", note)
	}
}
