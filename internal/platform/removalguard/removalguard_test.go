package removalguard_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/removalguard"
)

func TestExceeds(t *testing.T) {
	for _, tc := range []struct {
		name             string
		removed, present int
		want             bool
	}{
		{"nothing removed", 0, 100, false},
		{"exactly the share", 10, 100, false},
		{"over the share", 11, 100, true},
		{"everything from a non-empty catalog", 3, 3, true},
		{"empty catalog has nothing to protect", 0, 0, false},
		{"exactly the cap", 500, 10_000, false},
		{"over the cap of a huge catalog", 501, 100_000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := removalguard.Exceeds(tc.removed, tc.present); got != tc.want {
				t.Fatalf("Exceeds(%d, %d) = %v, want %v", tc.removed, tc.present, got, tc.want)
			}
		})
	}
}
