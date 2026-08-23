package api_test

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/loomarr/loomarr/internal/auth"
)

// TestMain lowers the stored-password bcrypt cost for this package's tests.
//
// These are HANDLER tests driving the real password service — creating local users,
// resetting passwords, changing your own — so every such test paid full key-stretching cost.
// Measured on CI: `internal/api` took 102s, versus 4s locally; `-race` multiplies it (the
// package pair internal/api+internal/auth was 79s with `-race`, 3s without).
//
// Nothing here asserts the cost factor — the tests care that a password verifies, that a
// member gets 403, that a session dies on disable. Hashing at MinCost tests all of that
// identically. `internal/auth` owns the guard that production still uses DefaultCost.
// ⚠ It also removes the migrated template (api_test.go) the run may have built. `os.Exit` does
// not run deferred functions, so this is the only place that teardown can happen — a `t.Cleanup`
// would fire after the FIRST test and delete the file the other 461 are copying.
func TestMain(m *testing.M) {
	restore := auth.SetBcryptCostForTests(bcrypt.MinCost)
	code := m.Run()
	restore()
	removeTemplate()
	os.Exit(code)
}
