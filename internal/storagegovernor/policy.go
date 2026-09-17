package storagegovernor

const (
	// GiB is the binary unit used by both settings and filesystem projections.
	GiB int64 = 1 << 30

	minimumHostReserve = 2 * GiB
	maximumHostReserve = 20 * GiB
	maximumAutoBudget  = 20 * GiB
)

// Domain names the managed owner of bytes. Domains share the hard host reserve
// but retain their own usage and soft-budget policy.
type Domain string

const (
	DomainFiller      Domain = "filler"
	DomainPrepared    Domain = "prepared"
	DomainDiagnostics Domain = "diagnostics"
)

// Policy is hot-applied to new reservations. AutomaticBudget selects the
// filesystem-derived allowance when SoftBudgetBytes is zero. With both unset,
// the domain has no governor-owned soft limit and only the host reserve applies.
type Policy struct {
	SoftBudgetBytes int64
	AutomaticBudget bool
}

// HardReserveBytes preserves ten percent of a filesystem, bounded for small
// appliances and large NAS volumes.
func HardReserveBytes(totalBytes int64) int64 {
	if totalBytes <= 0 {
		return 0
	}
	reserve := totalBytes / 10
	if reserve < minimumHostReserve {
		return minimumHostReserve
	}
	if reserve > maximumHostReserve {
		return maximumHostReserve
	}
	return reserve
}

// AutomaticBudgetBytes gives filler at most ten percent of its filesystem and
// caps that automatic allowance at twenty GiB.
func AutomaticBudgetBytes(totalBytes int64) int64 {
	if totalBytes <= 0 {
		return 0
	}
	budget := totalBytes / 10
	if budget > maximumAutoBudget {
		return maximumAutoBudget
	}
	return budget
}

func (p Policy) budget(totalBytes int64) (int64, bool) {
	if p.SoftBudgetBytes > 0 {
		return p.SoftBudgetBytes, true
	}
	if p.AutomaticBudget {
		return AutomaticBudgetBytes(totalBytes), true
	}
	return 0, false
}

func normalizeDomain(domain Domain) Domain {
	if domain == "" {
		return DomainFiller
	}
	return domain
}

func validDomain(domain Domain) bool {
	switch domain {
	case DomainFiller, DomainPrepared, DomainDiagnostics:
		return true
	default:
		return false
	}
}
