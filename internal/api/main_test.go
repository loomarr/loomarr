package api_test

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/mantonx/loomarr/internal/auth"
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
func TestMain(m *testing.M) {
	restore := auth.SetBcryptCostForTests(bcrypt.MinCost)
	code := m.Run()
	restore()
	os.Exit(code)
}
