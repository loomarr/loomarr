package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func TestRunSuitabilityRequiresCredentialAndHardCeilings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	code := runSuitability(nil, &stdout, &stderr, suitabilityCapabilities{
		getenv: func(string) string { return "" },
		run: func(context.Context, fillerreview.TemporalSuitabilityConfig) (fillerreview.TemporalSuitabilityResult, error) {
			called = true
			return fillerreview.TemporalSuitabilityResult{}, nil
		},
	})
	if code != 2 || called || stdout.Len() != 0 || !strings.Contains(stderr.String(), "positive case/request/cost ceilings") {
		t.Fatalf("code=%d called=%t stdout=%q stderr=%q", code, called, stdout.String(), stderr.String())
	}
}

func TestStringListRejectsEmptyAlias(t *testing.T) {
	var values stringList
	if err := values.Set(" "); err == nil {
		t.Fatal("empty alias accepted")
	}
	if err := values.Set("evidence-01"); err != nil || values.String() != "evidence-01" {
		t.Fatalf("values=%v err=%v", values, err)
	}
}
