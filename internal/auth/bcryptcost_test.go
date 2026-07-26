package auth

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestMain lowers the bcrypt cost for THIS package's tests. bcrypt is deliberately CPU-bound
// and under `-race` that cost dominated the whole Go CI job (internal/api alone: 102s on CI,
// 4s locally). Nothing here asserts anything about the cost factor itself, so hashing at
// MinCost tests exactly the same behaviour, faster.
// productionCost captures the package default BEFORE TestMain lowers it — the only moment it
// is observable from inside the suite.
var productionCost int

func TestMain(m *testing.M) {
	productionCost = bootstrapCost
	restore := SetBcryptCostForTests(bcrypt.MinCost)
	code := m.Run()
	restore()
	os.Exit(code)
}

// ⚠ The guard that makes the exported test hook safe: production must hash at DefaultCost.
// Without it, a stray init() or a call site that forgot to restore would silently ship weak
// password hashes — a security regression with no visible symptom.
//
// It reads the value TestMain SAVED, not the live var: TestMain has already lowered the cost
// by the time any test body runs, so asserting on `bootstrapCost` here would only ever see
// MinCost. An earlier version called SetBcryptCostForTests(DefaultBcryptCost) and then
// asserted the var equalled DefaultCost — which is true by construction and passes even when
// the production default has been weakened to MinCost. Verified: that version stayed green
// with the source edited to MinCost; this one fails.
func TestBcryptCostIsDefaultInProduction(t *testing.T) {
	if productionCost != bcrypt.DefaultCost {
		t.Fatalf("production bcrypt cost = %d, want DefaultCost (%d) — a lowered default ships weak hashes",
			productionCost, bcrypt.DefaultCost)
	}
	if DefaultBcryptCost != bcrypt.DefaultCost {
		t.Errorf("DefaultBcryptCost = %d, want %d", DefaultBcryptCost, bcrypt.DefaultCost)
	}
}
