package app

import (
	"testing"

	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
)

// Recovery remains an explicit production capability even though ordinary pipeline adapters no
// longer possess publication authority.
func TestFillerServiceAdapter_ExposesPipelineRecovery(t *testing.T) {
	if _, ok := any(fillerServiceAdapter{}).(fillerapi.Rewinder); !ok {
		t.Fatal("production filler adapter does not expose pipeline recovery; retry and rewind return 501")
	}
}
