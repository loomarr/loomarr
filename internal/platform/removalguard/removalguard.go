// Package removalguard is the one policy for a scan that would remove too much at once.
//
// A walk or a listing that finds far less than the catalog holds usually means the folder was not
// readable (a share that is not mounted, a mount point that is empty), not that the household
// deleted most of its media. Both the library scanner (#2151, decision 33) and the filler catalog
// sync (#2673) hold such a removal instead of acting on it, and they hold it at the same size, so
// the size lives here rather than being restated by each.
package removalguard

const (
	// Share is the fraction of what a catalog holds that one pass may remove.
	Share = 0.10
	// AtLeast is the number of removals no catalog may exceed in one pass, however large it is.
	AtLeast = 500
)

// Exceeds reports whether removing removed of the present entries in one pass is more than the
// guard allows. A listing that drops everything while the catalog is non-empty always exceeds it;
// an empty catalog has nothing to protect, so only the absolute cap applies there.
func Exceeds(removed, present int) bool {
	return removed > AtLeast || (present > 0 && float64(removed) > Share*float64(present))
}
