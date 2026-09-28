package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompleteAuthoritySet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, required := range []string{"inventory", "rights", "selection", "evidence", "human", "media quality", "suitability", "reference", "families", "transitions", "source root", "prior adjudication", "generation time", "output"} {
		if !strings.Contains(stderr.String(), required) {
			t.Fatalf("usage omits %q: %s", required, stderr.String())
		}
	}
}

func TestRepeatedPathsRejectsEmptyAndPreservesOrder(t *testing.T) {
	var paths repeatedPaths
	if err := paths.Set(""); err == nil {
		t.Fatal("empty path accepted")
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
