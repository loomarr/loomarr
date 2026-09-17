//go:build eval

package main

import (
	"context"
	"strings"
	"testing"
)

func TestOpenRouterRetentionRequiresExplicitMatchingAuthorization(t *testing.T) {
	t.Parallel()

	for _, options := range []runOptions{
		{provider: "openrouter", maxChargeUSD: "1", allowProviderRetention: true},
		{provider: "openrouter", maxChargeUSD: "1", retentionAuthorization: "unexpected-on-zdr"},
	} {
		_, err := runReview(context.Background(), nil, options)
		if err == nil || !strings.Contains(err.Error(), "retention-authorization") {
			t.Fatalf("error = %v, want retention authorization rejection", err)
		}
	}
}
