//go:build race

package schedule

// raceEnabled is true when the tests run under -race. CI runs this package twice, with and without
// it (scripts/go-race-policy.sh RACE_AND_PLAIN, #2007): the plain run carries every assertion,
// and the race run only the tests that run in parallel (skipSerialUnderRace, sampleSweeps).
const raceEnabled = true
