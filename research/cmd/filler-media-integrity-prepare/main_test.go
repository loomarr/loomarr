package main

import (
	"bytes"
	"testing"
)

func TestRunRequiresClosedOfflineInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() = %d, stderr = %s", code, stderr.String())
	}
}
