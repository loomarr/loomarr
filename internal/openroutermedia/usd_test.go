package openroutermedia

import "testing"

func TestUSDToNanoCeilRoundsSubNanodollarSpendUp(t *testing.T) {
	got, err := USDToNanoCeil("0.0000000005")
	if err != nil || got != 1 {
		t.Fatalf("USDToNanoCeil = %d, %v", got, err)
	}
}
