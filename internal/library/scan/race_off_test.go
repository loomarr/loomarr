//go:build !race

package scan

// raceEnabled is true when the tests run under -race (race_on_test.go).
const raceEnabled = false
