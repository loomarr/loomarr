package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompletePackageInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "draft path is required") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsUnknownMaterializationMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--draft", "draft", "--review-packet", "packet", "--alias-map", "map", "--evidence-packets", "evidence", "--corpus-root", "corpus", "--out", "out", "--materialize", "symlink"}
	if code := run(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "hardlink or copy") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
