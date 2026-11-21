package suggest

import "golang.org/x/sync/errgroup"

// maxCatalogFanOut bounds how many Catalog searches one grounding pass runs at once. The
// per-pass title caps (maxPickNameQueries, maxReferenceTitleQueries) already keep a pass small;
// this makes the concurrency bound a property of the fan-out itself, so raising a cap cannot
// quietly turn one suggestion into an unbounded burst against the library and TMDB.
const maxCatalogFanOut = 8

// forEachBounded runs work(0..n-1) with at most maxCatalogFanOut in flight and returns when all
// have finished. work records its own result and error by index: one failed search must not
// cancel its siblings, because callers report the first failure in input order.
func forEachBounded(n int, work func(index int)) {
	var group errgroup.Group
	group.SetLimit(maxCatalogFanOut)
	for index := range n {
		group.Go(func() error {
			work(index)
			return nil
		})
	}
	_ = group.Wait() // work never returns an error; failures travel in the caller's result slots
}
