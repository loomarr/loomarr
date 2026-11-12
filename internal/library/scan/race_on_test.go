//go:build race

package scan

// raceEnabled is true when the tests run under -race. CI runs this package twice, with and without
// it (scripts/go-race-policy.sh RACE_AND_PLAIN): the scale guard runs in the plain run only.
const raceEnabled = true
