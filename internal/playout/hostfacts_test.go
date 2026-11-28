package playout

// nvencFacts is a measured NVENC host.
func nvencFacts() BudgetFacts {
	return BudgetFacts{
		Hardware: true, SessionLimit: 12, CPUAllowance: 1,
		Rungs: []int{1080, 1080, 720, 720},
		Costs: map[CostKey]ClassCost{
			{Class: ClassSDR, Height: 1080}:   {Speed: 19, CPUCores: 0.034},
			{Class: ClassSDR, Height: 720}:    {Speed: 30, CPUCores: 0.025},
			{Class: ClassHDR4K, Height: 1080}: {Speed: 2.4, CPUCores: 0.12},
			{Class: ClassHDR4K, Height: 720}:  {Speed: 3.6, CPUCores: 0.1},
		},
	}
}

// softwareHDRFacts is a CPU-only host whose boot probe measured 4K HDR at 3.6 cores and 0.3x at full
// quality (phase 0b's 4-CPU box), with an SDR cell beside it.
func softwareHDRFacts(allowance float64) BudgetFacts {
	return BudgetFacts{CPUAllowance: allowance, Rungs: []int{720}, Costs: map[CostKey]ClassCost{
		{Class: ClassSDR, Height: 720}:   {Speed: 3.1, CPUCores: 1.35},
		{Class: ClassHDR4K, Height: 720}: {Speed: 0.3, CPUCores: 3.6},
	}}
}
