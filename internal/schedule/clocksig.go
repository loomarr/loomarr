package schedule

import (
	"hash/fnv"
	"strconv"
	"time"
)

// ClockSignature summarises everything about `at` that ComputeDesiredAt reads OTHER than the
// rolling-window index (see WindowIndex): which rule pickRule selects, and which built-in holidays
// are active for the channel's seasonal selection. Two instants with equal signatures, the same
// inputs and the same window index arrange identically.
//
// It exists so a cache can key on what the clock actually CHANGES instead of on the wall-clock
// minute. `at` reaches the arrangement by exactly these routes — pickRule (rules), the seasonal
// pass and exclusive-episode pass (activeHolidays), and windowIndex — so this is the complete
// list. ⚠ When ComputeDesiredAt reads the clock a new way, it belongs here in the same change.
//
// The holiday set is hashed regardless of seasonal mode: over-splitting on a channel with
// seasonality off costs one extra cache entry per holiday window, and under-splitting would serve
// a wrong lineup, so the asymmetry decides it.
func ClockSignature(policy ChannelPolicy, at time.Time) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.Itoa(pickRuleIndex(policy.Rules, at))))
	for _, hol := range activeHolidays(at, policy.Seasonal.Holidays) {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(hol.id))
	}
	return h.Sum64()
}

// WindowIndex is the rolling-window index ComputeDesiredAt folds into its ordering seed for a
// window of the given length (windowIndex, exported for cache keying).
func WindowIndex(at time.Time, window time.Duration) int64 { return windowIndex(at, window) }
