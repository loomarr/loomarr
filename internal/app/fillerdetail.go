package app

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/filler/capability"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/research"
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

// fillerDetailRunner keeps one scheduler row while the modules retain different authority:
// enrichment writes verified axes; research writes only cited suggestions.
type fillerDetailRunner struct {
	enrichment fillerEnrichmentRunner
	research   *research.Runner
	retire     *fillerGeographyRetirement // runs last, once geography has had every pass
}

func (r fillerDetailRunner) Run(ctx context.Context) (enrichment.RunResult, error) {
	var out enrichment.RunResult
	var enrichmentErr, researchErr, retireErr error
	if r.enrichment != nil {
		out, enrichmentErr = r.enrichment.Run(ctx)
	}
	if r.research != nil {
		var researched research.RunResult
		researched, researchErr = r.research.Run(ctx)
		out.Considered += researched.Considered
		out.Updated += researched.Updated
		out.Failed += researched.Failed
	}
	if r.retire != nil {
		_, retireErr = r.retire.Run(ctx)
	}
	return out, errors.Join(enrichmentErr, researchErr, retireErr)
}
