package main

import (
	"bytes"
	"testing"
)

func TestRunRequiresDistinctExplicitOutputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if code := run([]string{"--draft", "draft.json", "--batch-id", "a", "--packet-out", "same", "--map-out", "same"}, &stdout, &stderr); code != 2 {
		t.Fatalf("same-output exit = %d", code)
	}
}
