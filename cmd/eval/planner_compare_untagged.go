//go:build !eval

package main

import "os"

func init() {
	register("planner-compare", plannerCompareSummary, func([]string) int {
		report(os.Stderr, "%s: this binary was built without -tags=eval; run it with go run -tags=eval ./cmd/eval\n", plannerCompareName)
		return 2
	})
}
