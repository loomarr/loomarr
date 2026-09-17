package filler

// ReadinessAction is the highest-impact operator action for the filler system. The server owns
// this ordering so every client presents the same diagnosis instead of reverse-engineering it
// from counters.
type ReadinessAction string

const (
	ReadinessNone             ReadinessAction = "none"
	ReadinessEnableFetch      ReadinessAction = "enable_fetch"
	ReadinessFreeCatalog      ReadinessAction = "free_catalog_capacity"
	ReadinessFreeSpace        ReadinessAction = "free_disposable_space"
	ReadinessChooseFolder     ReadinessAction = "choose_another_folder"
	ReadinessChangeLimit      ReadinessAction = "change_storage_limit"
	ReadinessRetryAcquisition ReadinessAction = "retry_acquisition"
	ReadinessRetryWork        ReadinessAction = "retry_failed_work"
	ReadinessReviewIncoming   ReadinessAction = "review_incoming"
	ReadinessAddFiller        ReadinessAction = "add_filler"
	ReadinessImproveCoverage  ReadinessAction = "improve_channel_coverage"
)

// StorageStatus is the governor's server-owned capacity projection. It contains measurements and
// a typed pause reason, never a client-side reconstruction of filesystem arithmetic.
type StorageStatus struct {
	TotalBytes              int64
	FreeBytes               int64
	ManagedBytes            int64
	ReservedBytes           int64
	FilesystemReservedBytes int64
	SoftBudgetBytes         int64
	HardReserveBytes        int64
	AvailableBytes          int64
	Automatic               bool
	State                   string
	PausedBy                string
}

// ReadinessInput contains only authoritative domain projections. No raw storage status reaches
// this classifier: pipeline ownership, fetch limits, and channel matching have already crossed
// their respective single sources of truth.
type ReadinessInput struct {
	Fetch    FetchStatus
	Storage  StorageStatus
	Pipeline PipelineOverview
	Pool     PoolReport
	Runs     []AcquisitionRun
	Repairs  AcquisitionRepairSummary
}

// Readiness is the server-owned summary shown at the simple Filler entry point. The detailed
// views retain every count; this projection answers what matters now and where to go next.
type Readiness struct {
	Ready bool
	Next  ReadinessAction

	ChannelID string
	Count     int

	Fetch    FetchStatus
	Storage  StorageStatus
	Pipeline PipelineOverview
	Pool     PoolReport
	Runs     []AcquisitionRun
	Repairs  AcquisitionRepairSummary
}

// ProjectReadiness chooses one next action in impact order. Machine blockage precedes operator
// decisions, and an empty airable pool precedes quality improvements to a pool that already works.
func ProjectReadiness(in ReadinessInput) Readiness {
	out := Readiness{Fetch: in.Fetch, Storage: in.Storage, Pipeline: in.Pipeline, Pool: in.Pool, Runs: in.Runs, Repairs: in.Repairs}
	switch {
	case !in.Fetch.Enabled:
		out.Next = ReadinessEnableFetch
	case in.Fetch.StoppedBy == "catalog":
		out.Next, out.Count = ReadinessFreeCatalog, in.Fetch.CatalogClips
	case in.Storage.PausedBy == "host_reserve":
		out.Next = ReadinessFreeSpace
	case in.Storage.PausedBy == "library_limit":
		out.Next = ReadinessChangeLimit
	case in.Storage.PausedBy == "capacity_unavailable" || in.Storage.PausedBy == "estimate_unknown":
		out.Next = ReadinessChooseFolder
	case in.Repairs.Count > 0:
		out.Next, out.Count = ReadinessRetryAcquisition, in.Repairs.Count
	case len(in.Runs) > 0 && (in.Runs[0].Status == AcquisitionError || in.Runs[0].Artifacts.Repair > 0):
		out.Next, out.Count = ReadinessRetryAcquisition, in.Runs[0].Failed+in.Runs[0].Artifacts.Repair
	case in.Pipeline.Recoverable > 0:
		out.Next, out.Count = ReadinessRetryWork, in.Pipeline.Recoverable
	case in.Pipeline.NeedsDecision > 0:
		out.Next, out.Count = ReadinessReviewIncoming, in.Pipeline.NeedsDecision
	case in.Pool.Eligible == 0:
		out.Next = ReadinessAddFiller
	default:
		if weakest := in.Pool.Weakest(); weakest != nil && weakest.Report.Level != MatchExact {
			out.Next, out.ChannelID = ReadinessImproveCoverage, weakest.ChannelID
			out.Count = weakest.Report.Total
		} else {
			out.Ready, out.Next = true, ReadinessNone
		}
	}
	return out
}
