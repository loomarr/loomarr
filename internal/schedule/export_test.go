package schedule

// SetBeforeBandSwaps turns arrange's before-band swaps (#1971) on or off and returns the restore.
// A test that calls it must not run in parallel with any other.
func SetBeforeBandSwaps(on bool) (restore func()) {
	was := beforeBandSwaps
	beforeBandSwaps = on
	return func() { beforeBandSwaps = was }
}
