package schedule

import "testing"

// RaceEnabled is raceEnabled for the external tests.
const RaceEnabled = raceEnabled

// skipSerialUnderRace skips a test that runs on one goroutine when the package runs under -race.
// This package starts no goroutines, so -race can only catch tests running at once over its
// shared memos (deal chains, season and hand-off runs), and a serial test gives it nothing to
// watch. The package's plain run, in the same CI lane, runs it (#2007).
func skipSerialUnderRace(t *testing.T) {
	t.Helper()
	if raceEnabled {
		t.Skip("serial: run by the plain run; -race runs only the parallel tests (#2007)")
	}
}

// SkipSerialUnderRace is skipSerialUnderRace for the external tests.
func SkipSerialUnderRace(t *testing.T) { skipSerialUnderRace(t) }

// SetBeforeBandSwaps turns arrange's before-band swaps (#1971) on or off and returns the restore.
// A test that calls it must not run in parallel with any other.
func SetBeforeBandSwaps(on bool) (restore func()) {
	was := beforeBandSwaps
	beforeBandSwaps = on
	return func() { beforeBandSwaps = was }
}
