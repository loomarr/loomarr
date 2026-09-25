package api

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
)

func TestStartFailureDetailIsSpecificPerReason(t *testing.T) {
	seen := map[string]playout.StartReason{}
	for _, reason := range []playout.StartReason{
		playout.StartProgramSourceUnreachable, playout.StartProgramSourceFailed,
		playout.StartEncoderExited, playout.StartNoStream,
	} {
		detail := startFailureDetail(fmt.Errorf("wrapped: %w", &playout.StartError{Reason: reason, Err: errors.New("x")}))
		if detail == "" || strings.Contains(detail, "couldn't start this channel's in-app stream") {
			t.Errorf("%s fell back to the generic sentence: %q", reason, detail)
		}
		if other, dup := seen[detail]; dup {
			t.Errorf("%s and %s share one explanation", reason, other)
		}
		seen[detail] = reason
	}
	if got := startFailureDetail(errors.New("untyped")); !strings.Contains(got, "couldn't start") {
		t.Errorf("untyped failure = %q, want the generic sentence", got)
	}
}
