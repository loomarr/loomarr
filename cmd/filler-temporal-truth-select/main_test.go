package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresEveryBoundHistoryArtifact(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "all A/B/C") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
