package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestRunRequiresLockedInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--manifest") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestReadRunTranscriptsRequiresExactDeclaredInput(t *testing.T) {
	digest := strings.Repeat("a", 64)
	if _, err := readRunTranscripts(fillereval.RunIdentity{TranscriptSetSHA256: digest}, ""); err == nil || !strings.Contains(err.Error(), "--transcripts is missing") {
		t.Fatalf("missing transcript error = %v", err)
	}
	if _, err := readRunTranscripts(fillereval.RunIdentity{}, "unexpected.jsonl"); err == nil || !strings.Contains(err.Error(), "names no transcript set") {
		t.Fatalf("unexpected transcript error = %v", err)
	}
}
