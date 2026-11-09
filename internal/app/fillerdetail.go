package app

import (
	"github.com/loomarr/loomarr/internal/filler/capability"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
)

// mainSelection reads the install's active language-model selection, live.
func mainSelection(set resolved) func() llm.Selection {
	return func() llm.Selection { return resolveSelection(set) }
}

func activeFillerTextSelection(set resolved, recorder *metrics.Recorder) enrichment.TextSelection {
	return capability.TextSelection(resolveSelection(set), func(selection llm.Selection) llm.Provider {
		return buildProviderFor(selection, recorder)
	})
}

func activeFillerTranscriptCapability(set resolved) enrichment.CapabilitySelection {
	return capability.TranscriptCapability(set, resolveSelection(set))
}

func activeFillerVisionCapability(set resolved) enrichment.CapabilitySelection {
	return capability.VisionCapability(set, resolveSelection(set))
}
