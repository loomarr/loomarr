package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresPaidReviewContract(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "OPENROUTER_API_KEY") || !strings.Contains(stderr.String(), "--snapshot") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
