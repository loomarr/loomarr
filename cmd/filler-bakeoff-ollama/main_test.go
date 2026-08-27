package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresLockedInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--manifest") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}
