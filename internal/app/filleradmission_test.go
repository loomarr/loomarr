package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/api"
)

// Recovery remains an explicit production capability even though ordinary pipeline adapters no
// longer possess publication authority.
func TestFillerServiceAdapter_ExposesPipelineRecovery(t *testing.T) {
	if _, ok := any(fillerServiceAdapter{}).(api.FillerRewinder); !ok {
		t.Fatal("production filler adapter does not expose pipeline recovery; retry and rewind return 501")
	}
}
