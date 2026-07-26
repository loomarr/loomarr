package auth

import "golang.org/x/crypto/bcrypt"

// SetBcryptCostForTests lowers the stored-password cost so a test suite is not dominated by
// deliberate key-stretching. Returns a func restoring the previous value.
//
// ⚠ EXPORTED but test-only by contract, and the contract is enforced by a test rather than by
// hope: `TestBcryptCostIsDefaultInProduction` asserts the package's default is unchanged, so a
// call site that forgot to restore, or a stray init() lowering it, fails the suite rather than
// silently shipping weak hashes.
//
// It exists because `internal/api` is an EXTERNAL test package (`package api_test`) and cannot
// touch an unexported var. Its handler tests drive the real password service — creating users,
// resetting passwords — so they pay full bcrypt cost on every write. Measured: `internal/api`
// took 102s on CI, 4s locally, and `internal/api` + `internal/auth` together take 79s with
// `-race` versus 3s without. The stretching is the point in production and pure cost in a test
// that only needs "the hash verifies".
//
// Production must never call this. There is deliberately no config knob: a cost an operator
// could turn down is a way to weaken stored credentials by accident.
func SetBcryptCostForTests(cost int) func() {
	prev := bootstrapCost
	bootstrapCost = cost
	return func() { bootstrapCost = prev }
}

// DefaultBcryptCost is what production uses, exposed so a test can assert it has not drifted.
const DefaultBcryptCost = bcrypt.DefaultCost
