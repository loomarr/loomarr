package app

import "testing"

func TestEncodeMemoryGatePerEncodePrecedence(t *testing.T) {
	const mib = int64(1) << 20
	available := func() (int64, bool) { return 8 << 30, true }
	for _, tc := range []struct {
		name     string
		setting  int
		measured int64
		want     int64
	}{
		{"operator setting wins", 700, 1500 * mib, 700 * mib},
		{"probe measurement above the floor raises the cost", 0, 1500 * mib, 1500 * mib},
		// The synthetic trial under-reads real preparation encodes, so it can never lower the floor.
		{"probe measurement below the floor is ignored", 0, 254 * mib, defaultEncodeHostBytes},
		{"floor before any measurement", 0, 0, defaultEncodeHostBytes},
	} {
		gate := encodeMemoryGate(available, func() int { return 2048 },
			func() int { return tc.setting }, func() int64 { return tc.measured })
		if got := gate.PerEncode(); got != tc.want {
			t.Errorf("%s: PerEncode = %d, want %d", tc.name, got, tc.want)
		}
		if got := gate.Reserve(); got != 2048*mib {
			t.Errorf("%s: Reserve = %d, want %d", tc.name, got, 2048*mib)
		}
	}
}

func TestEncodeMemoryGateZeroReserveDisablesTheCheck(t *testing.T) {
	gate := encodeMemoryGate(func() (int64, bool) { return 1, true },
		func() int { return 0 }, func() int { return 0 }, func() int64 { return 0 })
	if _, known := gate.Available(); known {
		t.Fatal("a zero reserve must report host memory as unknown so the gate stays open")
	}
}
