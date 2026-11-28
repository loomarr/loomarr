package playout

import (
	"github.com/loomarr/loomarr/internal/playout/admission"
	"github.com/loomarr/loomarr/internal/playout/media"
)

// nvencFacts is a measured NVENC host.
func nvencFacts() admission.BudgetFacts {
	return admission.BudgetFacts{
		Hardware: true, SessionLimit: 12, CPUAllowance: 1,
		Rungs: []int{1080, 1080, 720, 720},
		Costs: map[admission.CostKey]admission.ClassCost{
			{Class: media.ClassSDR, Height: 1080}:   {Speed: 19, CPUCores: 0.034},
			{Class: media.ClassSDR, Height: 720}:    {Speed: 30, CPUCores: 0.025},
			{Class: media.ClassHDR4K, Height: 1080}: {Speed: 2.4, CPUCores: 0.12},
			{Class: media.ClassHDR4K, Height: 720}:  {Speed: 3.6, CPUCores: 0.1},
		},
	}
}

// softwareHDRFacts is a CPU-only host whose boot probe measured 4K HDR at 3.6 cores and 0.3x at full
// quality (phase 0b's 4-CPU box), with an SDR cell beside it.
func softwareHDRFacts(allowance float64) admission.BudgetFacts {
	return admission.BudgetFacts{CPUAllowance: allowance, Rungs: []int{720}, Costs: map[admission.CostKey]admission.ClassCost{
		{Class: media.ClassSDR, Height: 720}:   {Speed: 3.1, CPUCores: 1.35},
		{Class: media.ClassHDR4K, Height: 720}: {Speed: 0.3, CPUCores: 3.6},
	}}
}
