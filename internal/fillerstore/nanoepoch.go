package fillerstore

import "time"

// The safety ledger orders events inside one second, so it stores nanoseconds rather than the
// legacy store epoch. One INTEGER/BIGINT representation serves SQLite and Postgres.
func fillerNanoEpoch(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixNano()
}

func fromFillerNanoEpoch(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}
