package suggest

import (
	"strings"
	"testing"
)

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
		if !strings.Contains(systemPrompt, w) {
			t.Errorf("systemPrompt is missing the format/medium guidance %q — a live-action title "+
				"could then land in a cartoon channel (grounding is intact; selection is the model's, "+
				"and the prompt is what constrains it)", w)
		}
	}
}

func TestSystemPromptBoundsFinalSelectionToFitHostedCompletionBudget(t *testing.T) {
	if !strings.Contains(systemPrompt, "Select at most 8 picks") {
		t.Fatal("systemPrompt must bound the final selection so grounded JSON fits max_tokens")
	}
}

func TestSystemPromptBalancesOwnedAnchorsWithOutsideLibraryDiscovery(t *testing.T) {
	for _, want := range []string{"owned titles as anchors", "one-third", "outside-library discoveries", "Never sacrifice relevance"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt is missing discovery-balance guidance %q", want)
		}
	}
}

func TestSystemPromptSeparatesEmptyResultRecoveryFromFinalization(t *testing.T) {
	for _, want := range []string{"If a call returns no candidates", "A non-empty result ends retrieval"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt is missing the tool-to-final transition %q", want)
		}
	}
}
