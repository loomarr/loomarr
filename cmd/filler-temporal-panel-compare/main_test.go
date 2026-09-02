package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompleteComparisonAuthority(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "human lock, two model locks") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
