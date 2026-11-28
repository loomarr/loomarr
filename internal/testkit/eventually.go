package testkit

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/waitfor"
)

// Eventually waits for cond to hold and fails the test if it does not within the bound; see
// waitfor.Eventually, which a package that testkit imports calls directly.
func Eventually(t testing.TB, within time.Duration, what string, cond func() bool) {
	t.Helper()
	waitfor.Eventually(t, within, what, cond)
}
