package playout

import "github.com/loomarr/loomarr/internal/playout/admission"

// The engine and its callers spell the admission ledger through these names. The types and their
// behaviour live in internal/playout/admission, which imports only the media vocabulary.
type (
	ResourceBudget    = admission.ResourceBudget
	BudgetFacts       = admission.BudgetFacts
	BudgetSnapshot    = admission.BudgetSnapshot
	BudgetUsage       = admission.BudgetUsage
	BudgetUse         = admission.BudgetUse
	ClassBudget       = admission.ClassBudget
	AdmitRequest      = admission.AdmitRequest
	Lease             = admission.Lease
	ClassCost         = admission.ClassCost
	CostKey           = admission.CostKey
	HostCPU           = admission.HostCPU
	MemoryGate        = admission.MemoryGate
	ToneCurveSource   = admission.ToneCurveSource
	RungCosts         = admission.RungCosts
	RungDecision      = admission.RungDecision
	RungMonitor       = admission.RungMonitor
	RungMonitorConfig = admission.RungMonitorConfig
	SpeedSample       = admission.SpeedSample
)

const (
	minStreamSpeed = admission.MinStreamSpeed
)

var (
	processCPUTime = admission.ProcessCPUTime
	peakRSSBytes   = admission.PeakRSSBytes
	heavyRungCosts = admission.HeavyRungCosts

	NewResourceBudget   = admission.NewResourceBudget
	NewRungMonitor      = admission.NewRungMonitor
	HDRKey              = admission.HDRKey
	ReadHostCPU         = admission.ReadHostCPU
	HostMemAvailable    = admission.HostMemAvailable
	EffectiveCapacity   = admission.EffectiveCapacity
	PlayoutCPUAllowance = admission.PlayoutCPUAllowance
	SoftwareRungs       = admission.SoftwareRungs
	ErrAtCapacity       = admission.ErrAtCapacity
	ErrYielded          = admission.ErrYielded
)
