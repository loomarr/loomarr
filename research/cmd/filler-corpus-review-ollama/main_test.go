package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompleteBlindReviewInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--package, --transcripts, --model, --model-digest, --reviewer-id, and --out are required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
