//go:build race

package schedule_test

// raceEnabled is true when the tests run under -race. The CI lanes always do, where the detector
// costs the planner walks several times their plain run, so the sweeps walk a sample there (#2007).
const raceEnabled = true
