// Package playoutprocessfixture supplies real subprocess behaviours for playout tests.
package playoutprocessfixture

import (
	"io"
	"os"
	"time"
)

// PreparedPrefix is the output emitted before a prepared child finishes or stalls.
const PreparedPrefix = "prepared-programme-prefix"

// RunPrepared writes a prefix, then exits successfully, fails, or closes stdout and
// stays alive until its supervisor stops it. Call only in a re-executed test process.
func RunPrepared(mode string) {
	if _, err := io.WriteString(os.Stdout, PreparedPrefix); err != nil {
		os.Exit(6)
	}
	switch mode {
	case "prepared-success":
		os.Exit(0)
	case "prepared-failure":
		os.Exit(7)
	case "prepared-stalled":
		_ = os.Stdout.Close()
		time.Sleep(24 * time.Hour)
	default:
		os.Exit(8)
	}
}
