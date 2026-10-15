//go:build !race

package schedule

// raceEnabled is true when the tests run under -race (race_on_test.go).
const raceEnabled = false
