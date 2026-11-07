package suggest

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/schedule"
)

// usPrompt is the system prompt rendered for the shipped definitions on the US calendar.
var usPrompt = systemPrompt(schedule.Calendar{})

// The system prompt is the only lever on model SELECTION — the pipeline grounds picks (drops
// fabricated ids) but never filters an in-library title on theme, so a real-but-off-format pick
// (a live-action dramedy in a "cartoons" channel) reaches the proposal unless the prompt tells
// the model to reject it. This pins the format/medium guidance so a prompt edit can't silently
// drop it; it does not (and cannot) assert model behaviour, only that the instruction is present.
func TestSystemPromptConstrainsFormatMedium(t *testing.T) {
	wants := []string{
		"FORMAT/MEDIUM is a hard qualifier",
		"cartoons", // the specific word that means "animation", not merely "kid-friendly tone"
		"animated",
		"live-action",
	}
	for _, w := range wants {
		if !strings.Contains(usPrompt, w) {
			t.Errorf("systemPrompt is missing the format/medium guidance %q — a live-action title "+
				"could then land in a cartoon channel (grounding is intact; selection is the model's, "+
				"and the prompt is what constrains it)", w)
		}
	}
}

func TestSystemPromptBoundsFinalSelectionToFitHostedCompletionBudget(t *testing.T) {
	if !strings.Contains(usPrompt, "Select at most 8 picks") {
		t.Fatal("systemPrompt must bound the final selection so grounded JSON fits max_tokens")
	}
}

func TestSystemPromptBalancesOwnedAnchorsWithOutsideLibraryDiscovery(t *testing.T) {
	for _, want := range []string{"owned titles as anchors", "one-third", "outside-library discoveries", "Never sacrifice relevance"} {
		if !strings.Contains(usPrompt, want) {
			t.Errorf("systemPrompt is missing discovery-balance guidance %q", want)
		}
	}
}

func TestSystemPromptSeparatesEmptyResultRecoveryFromFinalization(t *testing.T) {
	for _, want := range []string{"If a call returns no candidates", "A non-empty result ends retrieval"} {
		if !strings.Contains(usPrompt, want) {
			t.Errorf("systemPrompt is missing the tool-to-final transition %q", want)
		}
	}
}

func TestPromptRequiresCanonicalDateMeaningWithoutPolicyEra(t *testing.T) {
	for _, prompt := range []string{usPrompt, repairPrompt} {
		for _, want := range []string{"dateMeaning", "anchors", "axes"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("prompt is missing required date contract field %q", want)
			}
		}
	}
	if strings.Contains(usPrompt, `"era":{"from":<int>,"to":<int>}`) || strings.Contains(usPrompt, "era.from/era.to") {
		t.Fatal("systemPrompt must not ask the model to emit retired policy-era dates")
	}
}
