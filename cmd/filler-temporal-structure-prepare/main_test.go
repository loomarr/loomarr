package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRequiresExplicitConstructionAuthority(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, required := range []string{"authoring", "source root", "challenge id", "private seed", "fixed generation time", "output"} {
		if !strings.Contains(stderr.String(), required) {
			t.Fatalf("usage error omits %q: %s", required, stderr.String())
		}
	}
}
