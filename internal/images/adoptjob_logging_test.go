package images

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// An owner whose artwork keeps failing to adopt is re-listed every run. It must warn when the
// failure first appears or changes, not on every run.
func TestAdoptJobWarnFailed_RepeatsOnlyWhenTheFailureChanges(t *testing.T) {
	var buf bytes.Buffer
	j := NewAdoptJob(nil, nil, time.Now, slog.New(slog.NewTextHandler(&buf, nil)))

	for run := 0; run < 10; run++ {
		j.warnAdoptFailed("owner-1", errors.New("decode failed"), nil)
	}
	j.warnAdoptFailed("owner-1", errors.New("decode failed"), errors.New("anim missing"))

	if got := strings.Count(buf.String(), "level=WARN"); got != 2 {
		t.Fatalf("WARNs = %d, want 2 (first + changed)\n%s", got, buf.String())
	}
}
