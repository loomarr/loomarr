package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompleteStructureComparisonAuthority(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "at least two --assessment") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAssessmentPathsRejectsEmptyAndPreservesOrder(t *testing.T) {
	var paths assessmentPaths
	if err := paths.Set(""); err == nil {
		t.Fatal("empty assessment path was accepted")
	}
	if err := paths.Set("second.json"); err != nil {
		t.Fatal(err)
	}
	if err := paths.Set("first.json"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "second.json" || paths[1] != "first.json" {
		t.Fatalf("paths = %v", paths)
	}
}
