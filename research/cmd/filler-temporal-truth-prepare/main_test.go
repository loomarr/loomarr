package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresEveryEvidenceInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "every evidence path") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRequiresCompleteOCRIdentity(t *testing.T) {
	args := []string{"--selection", "s", "--draft", "d", "--download-ledger", "l", "--media-root", "m", "--packets", "p", "--packet-root", "r", "--transcripts", "t", "--out", "o", "--generated-at", "2026-09-01T00:00:00Z", "--ocr-engine", "ocr"}
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "supplied together") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
