package playoutbench

import (
	"strings"
	"testing"
)

func TestFailureDiagnosticsRetainInitialCauseAndTerminalError(t *testing.T) {
	const cause = "Invalid output format nv12 for hwframe download."
	const terminal = "Could not open encoder before EOF"
	got := tail(cause + strings.Repeat("\nprogress", 1000) + "\n" + terminal)
	if !strings.HasPrefix(got, cause) || !strings.HasSuffix(got, terminal) {
		t.Fatalf("failure diagnostics lost the cause or terminal error: %q", got)
	}
	if len(got) > 4200 {
		t.Fatalf("failure diagnostics are unbounded: %d bytes", len(got))
	}
}
