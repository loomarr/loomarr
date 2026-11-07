package openroutermedia

import (
	"fmt"
	"math/big"
	"strings"
)

// USDToNanoCeil converts exact provider decimal text to an integer budget unit.
// Sub-nanodollar charges round up so a budget reservation never understates spend.
func USDToNanoCeil(amount string) (int64, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(amount))
	if !ok || r.Sign() < 0 {
		return 0, fmt.Errorf("invalid non-negative USD decimal %q", amount)
	}
	scaled := new(big.Rat).Mul(r, big.NewRat(1_000_000_000, 1))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(scaled.Num(), scaled.Denom(), rem)
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, fmt.Errorf("USD decimal %q exceeds nanodollar range", amount)
	}
	return q.Int64(), nil
}
