package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func TestPaidAssessmentRequiresExplicitCredentialAndCeilings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	code := runPaidAssessment(nil, &stdout, &stderr, paidTemporalCapabilities{
		getenv: func(string) string { return "" },
		run: func(context.Context, fillerreview.OpenRouterTemporalConfig) (fillerreview.OpenRouterTemporalResult, error) {
			called = true
			return fillerreview.OpenRouterTemporalResult{}, nil
		},
	})
	if code != 2 || called || stdout.Len() != 0 || !strings.Contains(stderr.String(), "max-spend-nanousd") {
		t.Fatalf("code=%d called=%t stdout=%q stderr=%q", code, called, stdout.String(), stderr.String())
	}
}

func TestRecoveryDispatchDoesNotReadCredential(t *testing.T) {
	if !recoveryRequested([]string{"--out", "result.json", "--recover-lock-sha256=abc"}) {
		t.Fatal("recovery flag must select the offline path")
	}
}
